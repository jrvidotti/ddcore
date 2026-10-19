package engine

import (
	"github.com/jrvidotti/ddcore/internal/meta"
)

// queueDelivery writes a framework-owned delivery record and its job on the
// caller's transaction, then stamps the record with the job. args["delivery"]
// is filled with the new record's id.
func (c *Ctx) queueDelivery(doctype string, values Doc, method string, args, opts map[string]any) (string, error) {
	doc, err := c.NewDoc(doctype, values)
	if err != nil {
		return "", err
	}
	// Raised, not just SaveOpts.IgnorePermissions: the user whose write caused
	// the event may have access scopes, and those close the delivery doctypes
	// (and the check that a Link target exists) to them.
	var saved Doc
	if err := c.WithIgnorePermissions(func() error {
		var err error
		saved, err = c.Insert(doc, SaveOpts{IgnorePermissions: true})
		return err
	}); err != nil {
		return "", err
	}
	name := saved.ID()
	args["delivery"] = name
	if err := c.enqueueDelivery(doctype, name, method, args, opts); err != nil {
		return "", err
	}
	return name, nil
}

func (c *Ctx) enqueueDelivery(doctype, id, method string, args, opts map[string]any) error {
	job, err := c.Enqueue(method, args, opts)
	if err != nil {
		return err
	}
	_, err = c.Q().Exec(c.Ctx, `UPDATE tab_`+meta.Snake(doctype)+` SET job = $2 WHERE id = $1`, id, job)
	return err
}

// recordDelivery writes an outcome on the pool and logs a failure to write it.
// "Sent" is the success status of both mail and webhook deliveries and stamps sent_at.
// extraSet is appended to the SET list and its args bind from $5.
func (e *Engine) recordDelivery(c *Ctx, table, id, status, errText, extraSet string, extraArgs ...any) {
	if _, err := c.space().Exec(c.Ctx, `UPDATE `+table+` SET status = $2, error = NULLIF(left($3, 1000), ''),
		sent_at = CASE WHEN $2 = $4 THEN now() ELSE sent_at END, modified = now()`+extraSet+` WHERE id = $1`,
		append([]any{id, status, errText, MailSent}, extraArgs...)...); err != nil {
		e.Log.Warn("could not record a delivery outcome", "table", table, "delivery", id, "err", err)
	}
}
