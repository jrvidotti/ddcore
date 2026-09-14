package engine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// What became of one delivery. Queued is written on the transaction that
// caused the event; every later value is written on the pool, because the
// outcome of an attempt must survive the rollback of the job that made it.
const (
	WebhookQueued   = "Queued"
	WebhookRetrying = "Retrying"
	WebhookSent     = "Sent"
	WebhookFailed   = "Failed"
)

// webhookJobMethod is the job target. Not whitelisted: nobody reaches it over
// HTTP.
const webhookJobMethod = "core.services.webhooks.send"

// webhookQueue only labels the jobs, so the per-queue metrics of PRD-04 can
// tell a receiver that is down from everything else going wrong.
const webhookQueue = "webhook"

// webhookDocEvents are the lifecycle events a subscription can pick, keyed by
// the Webhook field that turns each one on. The field name is also the event
// type a receiver sees, after "doc.".
var webhookDocEvents = []string{"on_insert", "on_update", "on_submit", "on_cancel", "on_trash"}

// webhookSaveEvent maps Save's action onto the event a receiver subscribes to.
// A change to an allowOnSubmit field is an update like any other, as far as
// anyone outside is concerned.
var webhookSaveEvent = map[string]string{
	"save": "on_update", "submit": "on_submit", "cancel": "on_cancel", "update_after_submit": "on_update",
}

// webhookUnwatchable are the DocTypes the framework writes as a consequence of
// doing its own bookkeeping. A subscription to Webhook Delivery would feed
// itself; the rest record events rather than being them, and a receiver that
// wants "an email went out" should be told so by the app that sent it.
var webhookUnwatchable = map[string]bool{
	"Webhook": true, "Webhook Delivery": true, "Audit Event": true, "Version": true,
	"Error Log": true, "Email Delivery": true,
}

// A custom event name is a dotted lowercase identifier, and "doc." belongs to
// the lifecycle events: letting an app emit doc.on_submit would let it forge
// the framework's own events.
var webhookEventName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(\.[a-z0-9][a-z0-9_-]*)*$`)

// Bounds on what a subscription may ask for. A timeout is how long a worker is
// held by somebody else's server; attempts times the exponential backoff is how
// long an event may keep arriving after the fact.
const (
	webhookMinTimeout      = 1
	webhookMaxTimeout      = 60
	webhookDefaultTimeout  = 10
	webhookMaxAttempts     = 10
	webhookDefaultAttempts = 6
	// How much of a receiver's answer is kept on a failed delivery: enough to
	// read an error message, not enough to store whatever page a proxy served.
	webhookResponseSnippet = 512
)

// webhookSub is one enabled subscription as the enqueue path needs it.
type webhookSub struct {
	Name        string
	EventType   string
	Doctype     string
	CustomEvent string
	Events      map[string]bool
	Timeout     int
	MaxAttempts int
}

// WebhookReference is the document a custom event is about.
type WebhookReference struct {
	Doctype string `json:"doctype"`
	Name    string `json:"name"`
}

// webhookSubs returns the enabled subscriptions, reading them once and then
// from memory: the lookup sits on every document write, and a query per write
// for a table that is usually empty is a cost every site would pay.
//
// Read on the pool, so the cache only ever holds committed rows. A Webhook
// saved in the same transaction as the document is therefore not yet in force
// for it; the cache is dropped after that transaction commits.
func (e *Engine) webhookSubs(ctx context.Context) ([]webhookSub, error) {
	e.webhookMu.Lock()
	defer e.webhookMu.Unlock()
	if e.webhooks != nil {
		return e.webhooks, nil
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT name, event_type, webhook_doctype, custom_event,
		on_insert, on_update, on_submit, on_cancel, on_trash, timeout, max_attempts
		FROM tab_webhook WHERE enabled`)
	if err != nil {
		// Before the first migrate there is no table and therefore no
		// subscription. Anything else is a real failure: a write that could not
		// find out whether it owes somebody an event must not pretend it owes
		// nothing, or the delivery is lost with no trace.
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "42P01" {
			return nil, nil
		}
		return nil, err
	}
	subs := make([]webhookSub, 0, len(rows))
	for _, r := range rows {
		s := webhookSub{
			Name: db.Str(r["name"]), EventType: db.Str(r["event_type"]),
			Doctype: db.Str(r["webhook_doctype"]), CustomEvent: db.Str(r["custom_event"]),
			Events:      map[string]bool{},
			Timeout:     clampInt(int(toFloat(r["timeout"])), webhookMinTimeout, webhookMaxTimeout, webhookDefaultTimeout),
			MaxAttempts: clampInt(int(toFloat(r["max_attempts"])), 1, webhookMaxAttempts, webhookDefaultAttempts),
		}
		for _, ev := range webhookDocEvents {
			if r[ev] == true {
				s.Events[ev] = true
			}
		}
		subs = append(subs, s)
	}
	e.webhooks = subs
	return subs, nil
}

// InvalidateWebhooks drops the cached subscriptions.
func (e *Engine) InvalidateWebhooks() {
	e.webhookMu.Lock()
	e.webhooks = nil
	e.webhookMu.Unlock()
}

// queueDocWebhooks turns one lifecycle event into a delivery per matching
// subscription, on the caller's transaction.
//
// Called from Insert, Save and Delete after their hooks have run, so a hook
// that throws has already rolled the event back with the write. The document
// is the one about to be committed, which is what the receiver is told about.
func (c *Ctx) queueDocWebhooks(doctype string, doc Doc, event string) error {
	if doctype == "Webhook" {
		// A subscription changed. Drop the cache only once it is committed, so a
		// concurrent write cannot re-read the old rows into it afterwards.
		c.AfterCommit(c.E.InvalidateWebhooks)
		return nil
	}
	if c.E.Cfg.Webhooks.Off || webhookUnwatchable[doctype] || c.E.DB == nil {
		return nil
	}
	subs, err := c.E.webhookSubs(c.Ctx)
	if err != nil {
		return err
	}
	var matched []webhookSub
	for _, s := range subs {
		if s.EventType == "Document" && s.Doctype == doctype && s.Events[event] {
			matched = append(matched, s)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	data := map[string]any{"doctype": doctype, "name": doc.Name(), "doc": c.webhookDoc(doctype, doc)}
	ref := &WebhookReference{Doctype: doctype, Name: doc.Name()}
	for _, s := range matched {
		if _, err := c.queueWebhook(s, "doc."+event, data, ref, ""); err != nil {
			return err
		}
	}
	return nil
}

// webhookDoc is the document as a receiver may see it: a copy, so redaction
// never touches what the caller still holds, with every Password and Vault
// field removed the same way an API read removes them.
func (c *Ctx) webhookDoc(doctype string, doc Doc) Doc {
	b, err := json.Marshal(doc)
	if err != nil {
		return Doc{}
	}
	cp := Doc{}
	if err := json.Unmarshal(b, &cp); err != nil {
		return Doc{}
	}
	for k := range cp {
		if strings.HasPrefix(k, "__") {
			delete(cp, k)
		}
	}
	return c.RedactDoc(doctype, cp)
}

// EmitWebhook is ddcore.webhooks.emit: an app's own event, delivered to every
// subscription that names it. Returns the delivery names, empty when nobody
// subscribed or webhooks are off.
//
// key makes the emit idempotent per subscription. It is stored as
// "<key>:<webhook>" in a unique column, so a second emit with the same key is
// refused by the database rather than delivered twice.
func (c *Ctx) EmitWebhook(event string, data any, ref *WebhookReference, key string) ([]string, error) {
	if !webhookEventName.MatchString(event) || strings.HasPrefix(event, "doc.") {
		return nil, cerr.Validation("{0} is not a valid webhook event name", event).WithTitleKey("Invalid event")
	}
	if c.E.Cfg.Webhooks.Off {
		return []string{}, nil
	}
	subs, err := c.E.webhookSubs(c.Ctx)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, s := range subs {
		if s.EventType != "Custom" || s.CustomEvent != event {
			continue
		}
		k := ""
		if key != "" {
			k = key + ":" + s.Name
		}
		name, err := c.queueWebhook(s, event, data, ref, k)
		if err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, nil
}

// queueWebhook writes one delivery and its job on the caller's transaction —
// the shape QueueMail has, for the same reason: a write that rolls back has
// announced nothing.
func (c *Ctx) queueWebhook(s webhookSub, event string, data any, ref *WebhookReference, key string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"type":      event,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"data":      data,
	})
	if err != nil {
		return "", cerr.Validation("webhook payload for {0} is not JSON: {1}", event, err.Error())
	}
	values := Doc{
		"webhook":  s.Name,
		"event":    event,
		"status":   WebhookQueued,
		"attempts": 0,
		"payload":  string(body),
		"key":      key,
	}
	if ref != nil && ref.Doctype != "" {
		values["reference_doctype"] = ref.Doctype
		values["reference_name"] = ref.Name
	}
	doc, err := c.NewDoc("Webhook Delivery", values)
	if err != nil {
		return "", err
	}
	saved, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
	if err != nil {
		return "", err
	}
	name := saved.Name()
	if err := c.enqueueWebhookJob(name, s.Timeout, s.MaxAttempts); err != nil {
		return "", err
	}
	return name, nil
}

func (c *Ctx) enqueueWebhookJob(delivery string, timeout, maxAttempts int) error {
	id, err := c.Enqueue(webhookJobMethod, map[string]any{"delivery": delivery}, map[string]any{
		"queue": webhookQueue, "maxAttempts": maxAttempts, "backoff": BackoffExponential,
		// The HTTP timeout bounds the request; the job's own timeout only has to
		// outlast it, with room for reading the delivery and recording the result.
		"timeout": timeout + 15,
	})
	if err != nil {
		return err
	}
	_, err = c.Q().Exec(c.Ctx, `UPDATE tab_webhook_delivery SET job = $2 WHERE name = $1`, delivery, id)
	return err
}

// errWebhookRetry is what a delivery returns when the job should run again:
// the job machinery reschedules on any error, and the status on the record has
// already been written.
type errWebhookRetry struct{ msg string }

func (e errWebhookRetry) Error() string { return e.msg }

// DeliverWebhook makes one attempt at one delivery.
//
// A receiver that answers 2xx has it. A network error, a timeout, 408, 429 or
// any 5xx is worth trying again, and returns an error so the job is
// rescheduled with its exponential backoff. Anything else — a 4xx, a redirect —
// is the receiver saying no, and trying again would only say it again.
//
// A timeout is retried even though the receiver may already have acted on the
// request. That is the difference from mail's Uncertain: every attempt carries
// the same webhook-id, so a receiver can drop what it has seen, and the
// Standard Webhooks contract asks it to.
func (e *Engine) DeliverWebhook(c *Ctx, delivery string) error {
	rows, err := db.Select(c.Ctx, e.DB.Pool,
		`SELECT webhook, payload::text AS payload, status, job FROM tab_webhook_delivery WHERE name = $1`, delivery)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		// Pruned or deleted by an administrator while the job waited.
		return nil
	}
	row := rows[0]
	if db.Str(row["status"]) == WebhookSent {
		return nil
	}

	// The attempt is counted on the pool: it happened whether or not anything
	// after it commits.
	var attempts int
	if err := e.DB.Pool.QueryRow(c.Ctx,
		`UPDATE tab_webhook_delivery SET attempts = attempts + 1, modified = now() WHERE name = $1 RETURNING attempts`,
		delivery).Scan(&attempts); err != nil {
		return err
	}
	last := true
	if job := int64(toFloat(row["job"])); job > 0 {
		var jobAttempts, maxAttempts int
		if err := e.DB.Pool.QueryRow(c.Ctx, `SELECT attempts, max_attempts FROM ddcore_job WHERE id = $1`, job).
			Scan(&jobAttempts, &maxAttempts); err == nil {
			last = jobAttempts >= maxAttempts
		}
	}

	hooks, err := db.Select(c.Ctx, e.DB.Pool,
		`SELECT name, url, enabled, timeout FROM tab_webhook WHERE name = $1`, db.Str(row["webhook"]))
	if err != nil {
		return err
	}
	if len(hooks) == 0 || hooks[0]["enabled"] != true {
		e.recordWebhook(c.Ctx, delivery, WebhookFailed, 0, "webhook "+db.Str(row["webhook"])+" is disabled or was deleted")
		return nil
	}
	hook := hooks[0]

	secret, err := e.webhookSecret(c, db.Str(hook["name"]))
	if err != nil {
		// A key that cannot be read will not become readable by waiting.
		e.recordWebhook(c.Ctx, delivery, WebhookFailed, 0, err.Error())
		return nil
	}

	body := []byte(db.Str(row["payload"]))
	timeout := clampInt(int(toFloat(hook["timeout"])), webhookMinTimeout, webhookMaxTimeout, webhookDefaultTimeout)
	status, snippet, sendErr := postWebhook(c.Ctx, db.Str(hook["url"]), delivery, secret, body, time.Duration(timeout)*time.Second)

	retry := false
	var errText string
	switch {
	case sendErr != nil:
		retry, errText = true, sendErr.Error()
	case status >= 200 && status < 300:
		e.recordWebhook(c.Ctx, delivery, WebhookSent, status, "")
		return nil
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500:
		retry, errText = true, fmt.Sprintf("receiver answered %d: %s", status, snippet)
	default:
		errText = fmt.Sprintf("receiver answered %d: %s", status, snippet)
	}

	if retry && !last {
		e.recordWebhook(c.Ctx, delivery, WebhookRetrying, status, errText)
		return errWebhookRetry{msg: "webhook delivery " + delivery + ": " + errText}
	}
	e.recordWebhook(c.Ctx, delivery, WebhookFailed, status, errText)
	if retry {
		// Out of attempts: fail the job too, so the failure is counted where
		// PRD-04's thresholds look and an Error Log row points at it.
		return errWebhookRetry{msg: "webhook delivery " + delivery + " gave up: " + errText}
	}
	return nil
}

// webhookSecret reads a subscription's signing key without writing a Vault
// Audit Log row per attempt; see vaultRead.
func (e *Engine) webhookSecret(c *Ctx, webhook string) (string, error) {
	key, err := MasterKey()
	if err != nil {
		return "", err
	}
	d, err := c.St.DocType("Webhook")
	if err != nil {
		return "", err
	}
	f := d.Field("secret")
	if f == nil {
		return "", cerr.Internal("Webhook has no secret field")
	}
	secret, ok, err := vaultRead(c.Ctx, e.DB.Pool, key, c.DeriveVaultKey(d, f, Doc{"name": webhook}))
	if err != nil {
		return "", err
	}
	if !ok || secret == "" {
		return "", cerr.Validation("Webhook {0} has no signing secret", webhook)
	}
	return secret, nil
}

// postWebhook sends one signed request. Redirects are not followed: a signed
// body re-posted to wherever a 302 points is a body sent to an address nobody
// configured.
func postWebhook(ctx context.Context, target, id, secret string, body []byte, timeout time.Duration) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	ts := time.Now().Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ddcore-webhooks/1")
	req.Header.Set("webhook-id", id)
	req.Header.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("webhook-signature", SignWebhook(secret, id, ts, body))
	client := &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, webhookResponseSnippet))
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, strings.TrimSpace(string(snippet)), nil
}

// SignWebhook is the Standard Webhooks signature: HMAC-SHA256 over
// "<id>.<timestamp>.<body>", base64, versioned "v1,". A secret written as
// whsec_<base64> is that spec's key format and is decoded; anything else is
// used as raw bytes, so a key pasted from a receiver that invents its own
// still works.
func SignWebhook(secret, id string, ts int64, body []byte) string {
	key := []byte(secret)
	if rest, ok := strings.CutPrefix(secret, "whsec_"); ok {
		if k, err := base64.StdEncoding.DecodeString(rest); err == nil {
			key = k
		}
	}
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%d.", id, ts)
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// recordWebhook writes on the pool, never on the job's transaction: a job
// about to fail is about to roll back, and the record of why must not go with
// it.
func (e *Engine) recordWebhook(ctx context.Context, delivery, status string, code int, errText string) {
	var sentAt any
	if status == WebhookSent {
		sentAt = time.Now()
	}
	if len(errText) > 1000 {
		errText = errText[:1000]
	}
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE tab_webhook_delivery SET status = $2, response_status = NULLIF($3, 0),
		error = NULLIF($4, ''), sent_at = COALESCE($5, sent_at), modified = now() WHERE name = $1`,
		delivery, status, code, errText, sentAt); err != nil {
		e.Log.Warn("could not record a webhook outcome", "delivery", delivery, "err", err)
	}
}

// ReplayWebhook sends a finished delivery again: the same webhook-id and the
// same body, with a fresh set of attempts.
//
// Only a System Manager may, and every replay — allowed or refused — leaves an
// Audit Event, because it sends data to a third party on a person's say-so.
// A delivery still on its way is refused rather than doubled.
func (c *Ctx) ReplayWebhook(delivery string) error {
	if !c.IgnorePermissions() && !c.HasRole("System Manager") {
		c.AuditDenied("webhook.replay", "Webhook Delivery", delivery, nil)
		return cerr.Permission("Only a System Manager may replay a webhook delivery")
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT d.status, d.webhook, w.timeout, w.max_attempts
		FROM tab_webhook_delivery d LEFT JOIN tab_webhook w ON w.name = d.webhook
		WHERE d.name = $1 FOR UPDATE OF d`, delivery)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return cerr.NotFound("Webhook Delivery {0} not found", delivery)
	}
	r := rows[0]
	status := db.Str(r["status"])
	if status != WebhookSent && status != WebhookFailed {
		return cerr.Validation("Webhook Delivery {0} is still on its way ({1})", delivery, status).
			WithTitleKey("Delivery in progress")
	}
	if r["timeout"] == nil && r["max_attempts"] == nil {
		return cerr.Validation("The webhook of delivery {0} was deleted", delivery)
	}
	if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_webhook_delivery SET status = $2, attempts = 0, error = NULL,
		response_status = NULL, modified = now() WHERE name = $1`, delivery, WebhookQueued); err != nil {
		return err
	}
	if err := c.enqueueWebhookJob(delivery,
		clampInt(int(toFloat(r["timeout"])), webhookMinTimeout, webhookMaxTimeout, webhookDefaultTimeout),
		clampInt(int(toFloat(r["max_attempts"])), 1, webhookMaxAttempts, webhookDefaultAttempts)); err != nil {
		return err
	}
	return c.Audit("webhook.replay", "Webhook Delivery", delivery, map[string]any{
		"webhook": db.Str(r["webhook"]), "previous_status": status,
	})
}

// ValidateWebhook is the Webhook controller's validate, in Go so the rules
// that decide where signed data may go are tested with the delivery path.
func (c *Ctx) ValidateWebhook(doc Doc) error {
	raw := strings.TrimSpace(doc.Str("url"))
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return cerr.Validation("{0} is not an http or https address", raw).WithTitleKey("Invalid URL")
	}
	// A signed payload sent in the clear can be read by anyone on the path.
	// Plain http is for a receiver on this machine, or for development.
	if u.Scheme == "http" && !c.E.Cfg.Dev && !isLoopbackHost(u.Hostname()) {
		return cerr.Validation("A webhook outside development must use https").WithTitleKey("Invalid URL")
	}
	switch doc.Str("event_type") {
	case "Document":
		dt := doc.Str("webhook_doctype")
		d, err := c.St.DocType(dt)
		if err != nil || d == nil {
			return cerr.Validation("DocType {0} does not exist", dt)
		}
		if d.IsChild || webhookUnwatchable[d.Name] {
			return cerr.Validation("{0} cannot be watched by a webhook", dt)
		}
		any := false
		for _, ev := range webhookDocEvents {
			if doc[ev] == true || doc[ev] == float64(1) || doc[ev] == int64(1) {
				any = true
			}
		}
		if !any {
			return cerr.Validation("Choose at least one document event")
		}
	case "Custom":
		ev := doc.Str("custom_event")
		if !webhookEventName.MatchString(ev) || strings.HasPrefix(ev, "doc.") {
			return cerr.Validation("{0} is not a valid webhook event name", ev).WithTitleKey("Invalid event")
		}
	}
	if t := int(toFloat(doc["timeout"])); t < webhookMinTimeout || t > webhookMaxTimeout {
		return cerr.Validation("Timeout must be between {0} and {1} seconds", webhookMinTimeout, webhookMaxTimeout)
	}
	if n := int(toFloat(doc["max_attempts"])); n < 1 || n > webhookMaxAttempts {
		return cerr.Validation("Max attempts must be between {0} and {1}", 1, webhookMaxAttempts)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// SweepWebhookDeliveries removes finished deliveries older than the site's
// retention. A delivery on its way is never removed, whatever its age.
func (e *Engine) SweepWebhookDeliveries(ctx context.Context) (int, error) {
	days := e.Cfg.Ops.WebhookDeliveryRetentionDays()
	if days <= 0 {
		return 0, nil
	}
	tag, err := e.DB.Pool.Exec(ctx, `DELETE FROM tab_webhook_delivery
		WHERE status IN ('Sent', 'Failed') AND modified < now() - make_interval(days => $1)`, days)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// WebhookStatus is what doctor reports.
type WebhookStatus struct {
	Off        bool `json:"off"`
	Enabled    int  `json:"enabled"`
	Retrying   int  `json:"retrying"`
	FailedLast int  `json:"failed24h"`
}

func (e *Engine) WebhookStatus(ctx context.Context) (WebhookStatus, error) {
	s := WebhookStatus{Off: e.Cfg.Webhooks.Off}
	err := e.DB.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM tab_webhook WHERE enabled),
		(SELECT count(*) FROM tab_webhook_delivery WHERE status = 'Retrying'),
		(SELECT count(*) FROM tab_webhook_delivery WHERE status = 'Failed' AND modified > now() - interval '24 hours')`).
		Scan(&s.Enabled, &s.Retrying, &s.FailedLast)
	return s, err
}

func clampInt(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
