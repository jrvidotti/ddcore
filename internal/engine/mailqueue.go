package engine

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/mail"
)

// What became of one message. Queued is written on the sender's transaction;
// every later value is written outside it, because the outcome of a delivery
// must survive the rollback of the job that discovered it.
const (
	MailQueued    = "Queued"
	MailSent      = "Sent"
	MailFailed    = "Failed"
	MailUncertain = "Uncertain"
)

// The framework's own two messages. Both are declared sensitive in
// core/mail/: their argument is a single-use link, and a link that can set
// somebody's password has no business sitting in a table.
const (
	MailTemplateInvite = "core.invite"
	MailTemplateReset  = "core.reset"
)

// mailJobMethod is the job target. Not whitelisted, so nobody reaches it over
// HTTP; it exists to be named here and by the scheduler.
const mailJobMethod = "core.services.mail.send"

// MailReference is the document a message is about.
type MailReference struct {
	Doctype string `json:"doctype"`
	Name    string `json:"name"`
}

// MailRequest is one call to ddcore.sendMail, after the prelude has resolved
// the template and rendered the subject.
type MailRequest struct {
	Template string   `json:"template"`
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	Lang     string   `json:"lang"`
	// Args is stored on the delivery record; JobArgs is not. A sensitive
	// template fills the second and leaves the first empty, which is what keeps
	// a live recovery link out of a table every System Manager can read.
	Args      map[string]any `json:"args"`
	JobArgs   map[string]any `json:"jobArgs"`
	Attach    []string       `json:"attach"`
	Reference *MailReference `json:"reference"`
	Key       string         `json:"key"`
}

// RecipientLang is the language a message will be written in: the first
// recipient who is a user of this site and has chosen one, else the site's.
//
// Read on the caller's transaction rather than the pool, so that inviting a
// user and mailing them in the same request sees the language just written.
func (c *Ctx) RecipientLang(to []string) string {
	for _, addr := range to {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT language FROM tab_user WHERE name = $1`, normalizeEmail(addr))
		if err == nil && len(rows) > 0 {
			if l := db.Str(rows[0]["language"]); l != "" {
				return l
			}
		}
	}
	return c.E.Cfg.Lang
}

// QueueMail records one message and hands it to a worker.
//
// Both writes land on the caller's transaction: a request that rolls back has
// neither queued a job nor left a delivery record behind, and a recovery mail
// for a password change that never happened is worse than no mail at all.
func (c *Ctx) QueueMail(r MailRequest) (map[string]any, error) {
	if _, ok := c.St.Snap.MailTemplates[r.Template]; !ok {
		return nil, cerr.NotFound("Mail template {0} does not exist", r.Template)
	}
	to := make([]string, 0, len(r.To))
	for _, addr := range r.To {
		addr = normalizeEmail(addr)
		if addr == "" {
			continue
		}
		if !validEmail(addr) {
			return nil, cerr.Validation("{0} is not a valid email address", addr).WithTitleKey("Invalid email")
		}
		to = append(to, addr)
	}
	if len(to) == 0 {
		return nil, cerr.Validation("A message needs a recipient").WithTitleKey("No recipient")
	}

	attachments, err := c.authorizeAttachments(r.Attach)
	if err != nil {
		return nil, err
	}

	values := Doc{
		"to":       strings.Join(to, ", "),
		"subject":  r.Subject,
		"status":   MailQueued,
		"template": r.Template,
		"lang":     r.Lang,
		"attempts": 0,
		"key":      r.Key,
	}
	if len(r.Args) > 0 {
		values["args"] = r.Args
	}
	if len(attachments) > 0 {
		values["attachments"] = attachments
	}
	if r.Reference != nil && r.Reference.Doctype != "" {
		values["reference_doctype"] = r.Reference.Doctype
		values["reference_name"] = r.Reference.Name
	}

	doc, err := c.NewDoc("Email Delivery", values)
	if err != nil {
		return nil, err
	}
	// The framework writes this record; no role has create on it.
	saved, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
	if err != nil {
		return nil, err
	}
	name := saved.Name()

	jobArgs := map[string]any{"delivery": name}
	if len(r.JobArgs) > 0 {
		jobArgs["args"] = r.JobArgs
	}
	id, err := c.Enqueue(mailJobMethod, jobArgs, nil)
	if err != nil {
		return nil, err
	}
	if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_email_delivery SET job = $2 WHERE name = $1`, name, id); err != nil {
		return nil, err
	}
	return map[string]any{"delivery": name}, nil
}

// SendTemplate queues one message from Go.
//
// It goes through the same JavaScript entry point an app uses rather than a
// private path in Go, because a template's subject and body *are* JavaScript
// functions. The framework's own two messages therefore exercise exactly the
// road every app travels, which is the only way that road stays honest.
func (c *Ctx) SendTemplate(template, to string, args map[string]any) error {
	rt, err := c.RT()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"template": template, "to": []string{to}, "args": args,
	})
	if err != nil {
		return err
	}
	_, err = rt.CallFunction("core.services.mail.queue", payload)
	return err
}

// authorizeAttachments resolves File references and decides, once and here,
// whether the caller may send them.
//
// The check happens now rather than at delivery because now is when there is
// someone to refuse: an unauthorized attachment fails the app's own
// transaction instead of failing alone in a worker half an hour later. The
// worker reads the bytes with permissions ignored, on the strength of this.
func (c *Ctx) authorizeAttachments(refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	limit := c.E.Cfg.Mail.MaxAttachment
	var total int64
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		row, err := c.findFile(ref)
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, cerr.NotFound("File {0} does not exist", ref)
		}
		if !c.CanReadFile(row) {
			return nil, cerr.Permission("No permission for the file {0}", ref)
		}
		total += int64(toFloat(row["file_size"]))
		if limit > 0 && total > limit {
			return nil, cerr.Validation("Attachments total more than the {0} bytes this site allows on one message", limit).
				WithTitleKey("Attachments are too large")
		}
		out = append(out, db.Str(row["name"]))
	}
	return out, nil
}

// mailFileFields is everything the sender needs about an attachment, plus
// whatever CanReadFile has to see.
var mailFileFields = append([]string{"name", "file_name", "file_url", "file_size", "content_type"}, FilePermFields...)

// findFile accepts what an app is likely to be holding: a File document name,
// or the file_url that an Attach field stores. The lookup ignores File's own
// permissions on purpose — the row has to be in hand before CanReadFile can
// have an opinion about it.
func (c *Ctx) findFile(ref string) (map[string]any, error) {
	var row map[string]any
	err := c.WithIgnorePermissions(func() error {
		var e error
		if row, e = c.GetValues("File", ref, mailFileFields); e != nil || row != nil {
			return e
		}
		row, e = c.GetValues("File", map[string]any{"file_url": ref}, mailFileFields)
		return e
	})
	return row, err
}

// LoadMail is the start of one delivery attempt: it hands the worker what the
// template needs and counts the attempt.
//
// The count is written on the pool, outside the job's transaction, for the same
// reason ddcore_job counts its own attempts there: the attempt happened whether
// or not the work that followed it committed.
func (e *Engine) LoadMail(c *Ctx, delivery string) (map[string]any, error) {
	allowed, err := c.notificationMailAllowed(delivery)
	if err != nil {
		return nil, err
	}
	if !allowed {
		e.recordMail(c, delivery, MailFailed, "Notification access revoked")
		return map[string]any{"skip": true}, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(),
		`SELECT name, "to", template, lang, args, attachments, status FROM tab_email_delivery WHERE name = $1`, delivery)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cerr.NotFound("Email Delivery {0} does not exist", delivery)
	}
	r := rows[0]
	if r["status"] == MailSent || r["status"] == MailUncertain {
		return map[string]any{"skip": true}, nil
	}
	if _, err := e.DB.Pool.Exec(c.Ctx, `UPDATE tab_email_delivery SET attempts = attempts + 1 WHERE name = $1`, delivery); err != nil {
		e.Log.Warn("could not count a delivery attempt", "delivery", delivery, "err", err)
	}

	return map[string]any{
		"to":       splitRecipients(db.Str(r["to"])),
		"template": db.Str(r["template"]),
		"lang":     db.Str(r["lang"]),
		"args":     asMap(r["args"]),
	}, nil
}

// DeliverMail renders the message and puts it on the wire.
//
// When the transport is an app's own function it cannot send from here — only
// the runtime can call into an app — so it hands the composed message back for
// the caller to dispatch, and records nothing. Every other transport is sent
// and recorded here.
func (e *Engine) DeliverMail(c *Ctx, delivery, subject string, blocks []mail.Block) (map[string]any, error) {
	allowed, err := c.notificationMailAllowed(delivery)
	if err != nil {
		return nil, err
	}
	if !allowed {
		e.recordMail(c, delivery, MailFailed, "Notification access revoked")
		return map[string]any{}, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT "to", attachments FROM tab_email_delivery WHERE name = $1`, delivery)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cerr.NotFound("Email Delivery {0} does not exist", delivery)
	}
	text, html := mail.Render(blocks)
	msg := mail.Message{
		To:      splitRecipients(db.Str(rows[0]["to"])),
		Subject: subject,
		Text:    text,
		HTML:    html,
	}
	if msg.Attachments, err = e.readAttachments(c, rows[0]["attachments"]); err != nil {
		e.recordMail(c, delivery, MailFailed, err.Error())
		return nil, err
	}

	if e.Cfg.Mail.Transport == config.MailMethod {
		return map[string]any{"method": e.Cfg.Mail.Method, "message": methodPayload(msg)}, nil
	}

	if err := e.Mailer().Send(c.Ctx, msg); err != nil {
		// An uncertain outcome is not retried: the relay may already have the
		// message, and trying again is precisely how it arrives twice. Record
		// the doubt and let a person resolve it.
		if errors.Is(err, mail.ErrUncertain) {
			e.recordMail(c, delivery, MailUncertain, err.Error())
			return map[string]any{}, nil
		}
		e.recordMail(c, delivery, MailFailed, err.Error())
		return nil, err
	}
	e.recordMail(c, delivery, MailSent, "")
	return map[string]any{}, nil
}

// readAttachments turns stored File names into bytes. Permissions were settled
// when the message was queued; a file deleted since is a delivery failure with
// a name in it, not a silent send without the invoice.
func (e *Engine) readAttachments(c *Ctx, stored any) ([]mail.Attachment, error) {
	names := asStrings(stored)
	if len(names) == 0 {
		return nil, nil
	}
	out := make([]mail.Attachment, 0, len(names))
	for _, n := range names {
		var row map[string]any
		err := c.WithIgnorePermissions(func() error {
			var e error
			row, e = c.GetValues("File", n, mailFileFields)
			return e
		})
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, fmt.Errorf("attachment %s no longer exists", n)
		}
		content, err := c.ReadAttachment(db.Str(row["file_url"]))
		if err != nil {
			return nil, fmt.Errorf("attachment %s: %w", n, err)
		}
		out = append(out, mail.Attachment{
			Filename:    db.Str(row["file_name"]),
			ContentType: db.Str(row["content_type"]),
			Content:     content,
		})
	}
	return out, nil
}

// RecordMail is how the worker reports an outcome it produced itself, which is
// only the app-transport case.
func (e *Engine) RecordMail(c *Ctx, delivery, status, errText string) error {
	switch status {
	case MailSent, MailFailed, MailUncertain, MailQueued:
	default:
		return cerr.Validation("{0} is not a delivery status", status)
	}
	e.recordMail(c, delivery, status, errText)
	return nil
}

// recordMail writes on the pool, never on the caller's transaction. The job
// that failed is about to roll back, and the record of why it failed must not
// roll back with it.
func (e *Engine) recordMail(c *Ctx, delivery, status, errText string) {
	var sentAt any
	if status == MailSent {
		sentAt = time.Now().In(e.Location())
	}
	e.DB.Pool.Exec(c.Ctx,
		`UPDATE tab_email_delivery SET status = $2, error = $3, sent_at = COALESCE($4, sent_at), modified = now() WHERE name = $1`,
		delivery, status, errText, sentAt)
}

// methodPayload is what an app's own transport function receives. Bytes are
// base64 so the value survives JSON on the way across the bridge.
func methodPayload(m mail.Message) map[string]any {
	atts := make([]map[string]any, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		atts = append(atts, map[string]any{
			"filename": a.Filename, "contentType": a.ContentType, "content": base64.StdEncoding.EncodeToString(a.Content),
		})
	}
	return map[string]any{
		"to": m.To, "subject": m.Subject, "text": m.Text, "html": m.HTML, "attachments": atts,
	}
}

func splitRecipients(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// asMap reads back a JSON column, which arrives as text or as an already
// decoded map depending on the driver's mood about jsonb.
func asMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case string:
		out := map[string]any{}
		json.Unmarshal([]byte(t), &out)
		return out
	case []byte:
		out := map[string]any{}
		json.Unmarshal(t, &out)
		return out
	}
	return map[string]any{}
}

func asStrings(v any) []string {
	var raw []any
	switch t := v.(type) {
	case []any:
		raw = t
	case string:
		json.Unmarshal([]byte(t), &raw)
	case []byte:
		json.Unmarshal(t, &raw)
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s := db.Str(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}
