package engine

import (
	"context"
	"encoding/json"

	"github.com/jrvidotti/ddcore/internal/db"
)

// Audit records that the current user did something sensitive to a target.
//
// Written on the caller's transaction: an action that rolls back did not
// happen, and a record claiming it did would be a false record. A refusal is
// the opposite case — see AuditDenied.
//
// This is the minimum the first operational service needed (a webhook replay).
// detail is for identifiers and states, never for a secret or a payload: every
// System Manager reads this table.
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

func (c *Ctx) writeAudit(q db.Querier, action, outcome, targetDoctype, targetName string, detail map[string]any) error {
	ip := ""
	if c.Request != nil {
		ip = db.Str(c.Request["ip"])
	}
	actor := c.User
	if actor == "" {
		actor = "System"
	}
	var detailJSON any
	if len(detail) > 0 {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		detailJSON = string(b)
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := q.Exec(ctx, `INSERT INTO tab_audit_event
		(name, owner, creation, modified, modified_by, docstatus, action, outcome, actor, target_doctype, target_name, ip, request_id, detail)
		VALUES ($1, $2, now(), now(), $2, 0, $3, $4, $2, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9)`,
		RandomToken(), actor, action, outcome, targetDoctype, targetName, ip, c.ReqID, detailJSON)
	return err
}
