package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
)

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(password|secret|token|key|hash|credential|auth|ciphertext|nonce)`)

// SanitizeAuditDetail redacts sensitive keys and truncates oversized strings.
func SanitizeAuditDetail(detail map[string]any) map[string]any {
	if detail == nil {
		return nil
	}
	out := make(map[string]any, len(detail))
	for k, v := range detail {
		if sensitiveKeyPattern.MatchString(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = sanitizeAuditValue(v)
	}
	return out
}

func sanitizeAuditValue(v any) any {
	switch val := v.(type) {
	case string:
		if len(val) > 500 {
			return val[:500] + "…"
		}
		return val
	case map[string]any:
		return SanitizeAuditDetail(val)
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = sanitizeAuditValue(item)
		}
		return res
	default:
		return v
	}
}

// Audit records that the current user did something sensitive to a target.
//
// Written on the caller's transaction: an action that rolls back did not
// happen, and a record claiming it did would be a false record. A refusal is
// the opposite case — see AuditDenied.
//
// detail is for identifiers and states, never for a secret or a payload: every
// System Manager reads this table. Sensitive keys are redacted automatically.
func (c *Ctx) Audit(action, targetDoctype, targetName string, detail map[string]any) error {
	return c.writeAudit(c.Q(), action, "Allowed", targetDoctype, targetName, detail)
}

// AuditDenied records a refused attempt. It is written on the pool, because
// the refusal is about to roll back the caller's transaction and the attempt
// is precisely what an investigation needs to find.
func (c *Ctx) AuditDenied(action, targetDoctype, targetName string, detail map[string]any) {
	if err := c.writeAudit(c.E.DB.Pool, action, "Denied", targetDoctype, targetName, detail); err != nil {
		c.E.Log.Warn("could not record a refused action", "action", action, "err", err)
	}
}

// RecordAudit records an audit event directly on the engine's DB pool.
func (e *Engine) RecordAudit(ctx context.Context, actor, action, outcome, targetDoctype, targetName string, detail map[string]any) error {
	return e.RecordAuditOn(ctx, e.DB.Pool, actor, action, outcome, targetDoctype, targetName, "", "", detail)
}

// RecordAuditOn records an audit event on the provided querier (tx or pool).
func (e *Engine) RecordAuditOn(ctx context.Context, q db.Querier, actor, action, outcome, targetDoctype, targetName, ip, reqID string, detail map[string]any) error {
	if actor == "" {
		actor = "System"
	}
	var detailJSON any
	if len(detail) > 0 {
		sanitized := SanitizeAuditDetail(detail)
		b, err := json.Marshal(sanitized)
		if err != nil {
			return err
		}
		detailJSON = string(b)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := q.Exec(ctx, `INSERT INTO tab_audit_event
		(name, owner, creation, modified, modified_by, docstatus, action, outcome, actor, target_doctype, target_name, ip, request_id, detail)
		VALUES ($1, $2, now(), now(), $2, 0, $3, $4, $2, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9)`,
		RandomToken(), actor, action, outcome, targetDoctype, targetName, ip, reqID, detailJSON)
	return err
}

func (c *Ctx) writeAudit(q db.Querier, action, outcome, targetDoctype, targetName string, detail map[string]any) error {
	ip := ""
	if c.Request != nil {
		ip = db.Str(c.Request["ip"])
	}
	actor := c.User
	return c.E.RecordAuditOn(c.Ctx, q, actor, action, outcome, targetDoctype, targetName, ip, c.ReqID, detail)
}

// AuditFilter narrows an audit listing.
type AuditFilter struct {
	Action        string
	Actor         string
	TargetDocType string
	TargetName    string
	Outcome       string
	Since         *time.Time
	Until         *time.Time
	Limit         int
	Start         int
}

func (f AuditFilter) where() (string, []any) {
	var clauses []string
	var args []any
	add := func(sql string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(sql, len(args)))
	}
	if f.Action != "" {
		add("action = $%d", f.Action)
	}
	if f.Actor != "" {
		add("actor = $%d", f.Actor)
	}
	if f.TargetDocType != "" {
		add("target_doctype = $%d", f.TargetDocType)
	}
	if f.TargetName != "" {
		add("target_name = $%d", f.TargetName)
	}
	if f.Outcome != "" {
		add("outcome = $%d", f.Outcome)
	}
	if f.Since != nil {
		add("creation >= $%d", *f.Since)
	}
	if f.Until != nil {
		add("creation <= $%d", *f.Until)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

const auditColumns = `name, owner, creation, modified, modified_by, docstatus, action, outcome, actor, target_doctype, target_name, ip, request_id, detail`

// ListAuditEvents reads audit events, newest first.
func (e *Engine) ListAuditEvents(ctx context.Context, f AuditFilter) ([]map[string]any, error) {
	where, args := f.where()
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, f.Start)
	q := fmt.Sprintf(`SELECT %s FROM tab_audit_event%s ORDER BY creation DESC, name DESC LIMIT $%d OFFSET $%d`,
		auditColumns, where, len(args)-1, len(args))
	return db.Select(ctx, e.DB.Pool, q, args...)
}

// CountAuditEvents counts audit events matching the filter.
func (e *Engine) CountAuditEvents(ctx context.Context, f AuditFilter) (int64, error) {
	where, args := f.where()
	var n int64
	err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM tab_audit_event`+where, args...).Scan(&n)
	return n, err
}

// PurgeAuditEvents deletes audit events older than days. DryRun returns the count without deleting.
func (e *Engine) PurgeAuditEvents(ctx context.Context, days int, dryRun bool) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	const where = `creation < now() - make_interval(days => $1)`
	if dryRun {
		var n int
		err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM tab_audit_event WHERE `+where, days).Scan(&n)
		return n, err
	}
	tag, err := e.DB.Pool.Exec(ctx, `DELETE FROM tab_audit_event WHERE `+where, days)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// SweepAuditEvents is the scheduled retention pass for audit events.
// If ops.auditRetentionDays <= 0, it does nothing and keeps events forever.
func (e *Engine) SweepAuditEvents(ctx context.Context) (int, error) {
	days := e.Cfg.Ops.AuditRetentionDays()
	if days <= 0 {
		return 0, nil
	}
	return e.PurgeAuditEvents(ctx, days, false)
}
