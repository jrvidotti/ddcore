package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// Notifications are internal records, deliberately absent from DocType metadata.
// Their only public interface derives the recipient from the authenticated ctx.
type Notification struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Message          string    `json:"message"`
	Creation         time.Time `json:"creation"`
	Read             bool      `json:"read"`
	ReferenceDoctype string    `json:"reference_doctype"`
	ReferenceID      string    `json:"reference_id"`
}
type NotificationPage struct {
	Data  []Notification `json:"data"`
	Total int            `json:"total"`
}

// withNotificationUser shares the transaction and VM, but never its privileged
// flags, roles or document cache. Rebinding avoids acquiring a second pool slot
// while every concurrent writer already holds one.
func (c *Ctx) withNotificationUser(user string, fn func(*Ctx) error) error {
	rt, err := c.RT()
	if err != nil {
		return err
	}
	child := c.E.NewCtx(c.Ctx, user)
	child.St, child.Tx, child.rt = c.St, c.Tx, rt
	child.Lang = c.RecipientLang([]string{user})
	// Shares are read from this transaction for the same reason as the roles
	// below: a share written a moment ago is not in the process cache yet.
	child.shares, err = child.loadShares(user)
	if err != nil {
		return err
	}
	child.sharesLoaded, child.sharesDirty = true, true
	// Authorization must observe role revocation even within this transaction,
	// before the ordinary role cache's after-commit invalidation.
	if user != "Admin" {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT role FROM tab_has_role WHERE parent=$1 AND parenttype='User'`, user)
		if err != nil {
			return err
		}
		child.roles = []string{"All"}
		for _, row := range rows {
			child.roles = append(child.roles, db.Str(row["role"]))
		}
	}
	old := rt.Ctx
	rt.Ctx = child
	rt.SetLang(child.Lang)
	defer func() { rt.Ctx = old; rt.SetLang(c.Lang) }()
	return fn(child)
}

func (c *Ctx) notificationAccess(user, doctype, name string) (bool, error) {
	if user == "" || user == "Guest" {
		return false, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT enabled FROM tab_user WHERE id=$1`, user)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 || rows[0]["enabled"] != true {
		return false, nil
	}
	allowed := false
	err = c.withNotificationUser(user, func(reader *Ctx) error {
		if _, ok := reader.St.Meta.Get(doctype); !ok {
			return nil
		}
		doc, err := reader.GetDoc(doctype, name)
		if err != nil {
			ce := cerr.From(err)
			if ce.Status == 403 || ce.Status == 404 {
				return nil
			}
			return err
		}
		ok, err := reader.HasPermission(doctype, "read", doc)
		if err != nil || !ok {
			return err
		}
		// GetDoc checks the document hook; GetList also applies permissionQuery.
		list, err := reader.GetList(doctype, ListArgs{Filters: map[string]any{"id": name}, Fields: []string{"id"}, Limit: 1})
		if err != nil {
			if cerr.From(err).Status == 403 {
				return nil
			}
			return err
		}
		allowed = len(list) == 1
		return nil
	})
	return allowed, err
}

func (c *Ctx) notificationsChanged(user string) {
	c.AfterCommit(func() {
		c.E.Events.Publish(Event{Name: "notifications_changed", User: user, Payload: map[string]any{}})
	})
}

func (c *Ctx) queueDocNotifications(doctype string, doc, before Doc, event string) error {
	if c.E.DB == nil {
		return nil
	}
	identity := "event:" + RandomToken()
	for _, rule := range c.St.Notifications {
		if rule.Doctype == doctype && rule.Event == event {
			if err := c.queueNotification(rule, doc, before, identity, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// queueNotification evaluates a rule and writes one occurrence per authorized
// recipient. claim, when given, runs once the condition holds and before any
// recipient is written; returning false means another sweep already owns this
// occurrence.
func (c *Ctx) queueNotification(rule js.Notification, doc, before Doc, identity string, claim func() (bool, error)) error {
	rt, err := c.RT()
	if err != nil {
		return err
	}
	evaluation, err := rt.EvaluateNotification(rule.Name, doc.JSON(), before.JSON())
	if err != nil {
		return err
	}
	if !evaluation.Matches {
		return nil
	}
	if claim != nil {
		ok, err := claim()
		if err != nil || !ok {
			return err
		}
	}
	seen := map[string]bool{}
	for _, user := range evaluation.Recipients {
		if seen[user] {
			continue
		}
		seen[user] = true
		ok, err := c.notificationAccess(user, rule.Doctype, doc.ID())
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		// Claim before rendering or queuing. ON CONFLICT waits for concurrent claims
		// and leaves the transaction usable when another scanner already won.
		name := RandomToken()
		tag, err := c.Q().Exec(c.Ctx, `INSERT INTO ddcore_notification
   (id,rule,recipient,reference_doctype,reference_id,identity,desk)
   VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, name, rule.Name, user, rule.Doctype, doc.ID(), identity, rule.Desk)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		var content *js.NotificationContent
		err = c.withNotificationUser(user, func(reader *Ctx) error {
			// the rule decided on the whole document; what it writes to a
			// recipient is only what that recipient may read (SEC-02)
			var err error
			content, err = rt.RenderNotification(rule.Name,
				reader.RedactDoc(rule.Doctype, doc.Clone()).JSON(), reader.RedactDoc(rule.Doctype, before.Clone()).JSON())
			return err
		})
		if err != nil {
			return err
		}
		if _, err = c.Q().Exec(c.Ctx, `UPDATE ddcore_notification SET title=$2,message=$3 WHERE id=$1`, name, content.Title, content.Message); err != nil {
			return err
		}
		if rule.Email != nil {
			// User identifiers are resolved to their account's email, never interpreted
			// as arbitrary external addresses supplied by a rule.
			users, err := db.Select(c.Ctx, c.Q(), `SELECT email FROM tab_user WHERE id=$1`, user)
			if err != nil {
				return err
			}
			if len(users) > 0 && db.Str(users[0]["email"]) != "" {
				var args map[string]any
				if len(content.EmailArgs) > 0 {
					if err = json.Unmarshal(content.EmailArgs, &args); err != nil {
						return err
					}
				}
				payload, _ := json.Marshal(map[string]any{"template": rule.Email.Template, "to": []string{db.Str(users[0]["email"])}, "args": args,
					"lang": c.RecipientLang([]string{user}), "reference": MailReference{Doctype: rule.Doctype, ID: doc.ID()}, "key": "notification:" + name})
				result, err := rt.CallFunction("core.services.mail.queue", payload)
				if err != nil {
					return err
				}
				var queued struct {
					Delivery string `json:"delivery"`
				}
				if err = json.Unmarshal(result, &queued); err != nil {
					return err
				}
				if _, err = c.Q().Exec(c.Ctx, `UPDATE ddcore_notification SET email_delivery=$2 WHERE id=$1`, name, queued.Delivery); err != nil {
					return err
				}
			}
		}
		if rule.Desk {
			c.notificationsChanged(user)
		}
	}
	return nil
}

// NotifyUser records a persistent notification for user (e.g. on assignment).
func (c *Ctx) NotifyUser(user, refDoctype, refName, title, message string) error {
	return c.notifyUserAs("assignment", user, refDoctype, refName, title, message)
}

// notifyUserAs records a direct notification under rule ("assignment",
// "share"), for a recipient who can read the document.
func (c *Ctx) notifyUserAs(rule, user, refDoctype, refName, title, message string) error {
	if user == "" || user == "Guest" || user == c.User {
		return nil
	}
	ok, err := c.notificationAccess(user, refDoctype, refName)
	if err != nil || !ok {
		return err
	}
	name := RandomToken()
	identity := fmt.Sprintf("%s:%s:%s:%s", rule, refDoctype, refName, name)
	tag, err := c.Q().Exec(c.Ctx, `INSERT INTO ddcore_notification
   (id,rule,recipient,reference_doctype,reference_id,identity,desk,title,message)
   VALUES($1,$2,$3,$4,$5,$6,true,$7,$8) ON CONFLICT DO NOTHING`,
		name, rule, user, refDoctype, refName, identity, title, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		c.notificationsChanged(user)
	}
	return nil
}

func notificationFromRow(r map[string]any) Notification {
	created, _ := r["creation"].(time.Time)
	return Notification{ID: db.Str(r["id"]), Title: db.Str(r["title"]), Message: db.Str(r["message"]), Creation: created, Read: r["read"] == true,
		ReferenceDoctype: db.Str(r["reference_doctype"]), ReferenceID: db.Str(r["reference_id"])}
}

// Walk in bounded database batches and paginate only after permission checks:
// hidden rows cannot inflate counts or leave holes in a page.
func (c *Ctx) ListNotifications(limit, offset int, read *bool) (NotificationPage, error) {
	out := NotificationPage{Data: []Notification{}}
	if c.User == "Guest" || c.User == "" {
		return out, cerr.Auth("Sign in to continue")
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	// Unread first, newest first within each group; the keyset cursor walks the same order.
	var cursorTime any
	cursorID, cursorRead := "", false
	for {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT id,title,message,creation,read,reference_doctype,reference_id
   FROM ddcore_notification WHERE recipient=$1 AND desk AND ($2::boolean IS NULL OR read=$2)
   AND ($3::timestamptz IS NULL OR read>$5 OR (read=$5 AND (creation,id)<($3,$4)))
   ORDER BY read,creation DESC,id DESC LIMIT 100`, c.User, read, cursorTime, cursorID, cursorRead)
		if err != nil {
			return out, err
		}
		for _, r := range rows {
			n := notificationFromRow(r)
			ok, err := c.notificationAccess(c.User, n.ReferenceDoctype, n.ReferenceID)
			if err != nil {
				return out, err
			}
			if !ok {
				continue
			}
			if out.Total >= offset && len(out.Data) < limit {
				out.Data = append(out.Data, n)
			}
			out.Total++
		}
		if len(rows) < 100 {
			break
		}
		last := rows[len(rows)-1]
		cursorTime = last["creation"]
		cursorID = db.Str(last["id"])
		cursorRead = last["read"] == true
	}
	return out, nil
}

func (c *Ctx) NotificationCount() (int, error) {
	unread := false
	p, err := c.ListNotifications(1, 0, &unread)
	return p.Total, err
}

func (c *Ctx) SetNotificationRead(name string, read bool) (Notification, error) {
	var zero Notification
	if c.User == "Guest" || c.User == "" {
		return zero, cerr.Auth("Sign in to continue")
	}
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT * FROM ddcore_notification WHERE id=$1 AND recipient=$2 AND desk FOR UPDATE`, name, c.User)
	if err != nil {
		return zero, err
	}
	if len(rows) == 0 {
		return zero, cerr.NotFound("Notification not found")
	}
	n := notificationFromRow(rows[0])
	ok, err := c.notificationAccess(c.User, n.ReferenceDoctype, n.ReferenceID)
	if err != nil {
		return zero, err
	}
	if !ok {
		return zero, cerr.NotFound("Notification not found")
	}
	if _, err = c.Q().Exec(c.Ctx, `UPDATE ddcore_notification SET read=$2 WHERE id=$1`, name, read); err != nil {
		return zero, err
	}
	n.Read = read
	c.notificationsChanged(c.User)
	return n, nil
}

// notificationMailAllowed is checked before rendering and again before the
// transport call. A revoked or deleted reference must never leave on a retry.
func (c *Ctx) notificationMailAllowed(delivery string) (bool, error) {
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT recipient,reference_doctype,reference_id FROM ddcore_notification WHERE email_delivery=$1`, delivery)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return true, nil
	}
	r := rows[0]
	return c.notificationAccess(db.Str(r["recipient"]), db.Str(r["reference_doctype"]), db.Str(r["reference_id"]))
}

func notificationDue(value any, fieldtype string, days int, loc *time.Location) time.Time {
	var t time.Time
	if fieldtype == "Date" {
		s := db.Str(value)
		if v, ok := value.(time.Time); ok {
			s = v.Format("2006-01-02")
		}
		t, _ = time.ParseInLocation("2006-01-02", s, loc)
	} else {
		t = parseTime(value, loc).In(loc)
	}
	if t.IsZero() {
		return t
	}
	return t.AddDate(0, 0, days)
}

const notificationSweepMethod = "core.notifications.sweep"

// notificationCutoff bounds the sweep's SQL to rows that may already be due.
// A Date is exact; a Datetime gets a few hours of slack for a daylight-saving
// day, and notificationDue decides precisely.
func notificationCutoff(fieldtype string, days int, now time.Time, loc *time.Location) any {
	local := now.In(loc)
	if fieldtype == "Date" {
		return local.AddDate(0, 0, -days).Format("2006-01-02")
	}
	return local.AddDate(0, 0, -days).Add(3 * time.Hour)
}

// SweepNotifications has no high-water date: a newly enabled rule and a restart
// both recover still-valid historical dates. ddcore_notification_due records
// each (rule, document, due date) whose condition held, so a matched date is
// evaluated once rather than on every sweep, while one whose condition does not
// hold yet is evaluated again. A changed due date is a new occurrence. A
// separate transaction per batch bounds locks; a row lock makes evaluating a
// changed due date coherent.
func (e *Engine) SweepNotifications(ctx context.Context, now time.Time) error {
	st := e.Current()
	loc := e.Location()
	for _, rule := range st.Notifications {
		if rule.Date == nil {
			continue
		}
		d, err := st.DocType(rule.Doctype)
		if err != nil {
			return err
		}
		field := d.Field(rule.Date.Field)
		cutoff := notificationCutoff(field.Fieldtype, rule.Date.Days, now, loc)
		cursor := ""
		for {
			count := 0
			c := e.NewCtx(ctx, "Admin")
			c.St = st
			err := c.Run(func(c *Ctx) error {
				rows, err := db.Select(ctx, c.Q(), fmt.Sprintf(`SELECT id, %[2]s AS due FROM %[1]s
   WHERE id > $1 AND %[2]s IS NOT NULL AND %[2]s <= $2 ORDER BY id LIMIT 100`,
					db.Ident(d.TableName()), db.Ident(field.Fieldname)), cursor, cutoff)
				if err != nil {
					return err
				}
				count = len(rows)
				if count == 0 {
					return nil
				}
				names := make([]string, len(rows))
				for i, row := range rows {
					names[i] = db.Str(row["id"])
				}
				cursor = names[len(names)-1]
				marks, err := db.Select(ctx, c.Q(), `SELECT reference_id, due FROM ddcore_notification_due
   WHERE rule = $1 AND reference_doctype = $2 AND reference_id = ANY($3)`, rule.Name, d.Name, names)
				if err != nil {
					return err
				}
				done := map[string]bool{}
				for _, m := range marks {
					if t, ok := m["due"].(time.Time); ok {
						done[db.Str(m["reference_id"])+"\x00"+t.UTC().Format(time.RFC3339Nano)] = true
					}
				}
				for _, row := range rows {
					name := db.Str(row["id"])
					due := notificationDue(row["due"], field.Fieldtype, rule.Date.Days, loc)
					if due.IsZero() || due.After(now) || done[name+"\x00"+due.UTC().Format(time.RFC3339Nano)] {
						continue
					}
					doc, err := c.getDocForUpdate(d.Name, name)
					if err != nil {
						if cerr.From(err).Status == 404 {
							continue
						}
						return err
					}
					// The row may have changed since the batch was read.
					due = notificationDue(doc[field.Fieldname], field.Fieldtype, rule.Date.Days, loc)
					if due.IsZero() || due.After(now) {
						continue
					}
					claim := func() (bool, error) {
						tag, err := c.Q().Exec(c.Ctx, `INSERT INTO ddcore_notification_due (rule, reference_doctype, reference_id, due)
   VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, rule.Name, d.Name, name, due)
						return err == nil && tag.RowsAffected() == 1, err
					}
					if err = c.queueNotification(rule, doc, nil, "date:"+due.UTC().Format(time.RFC3339Nano), claim); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			if count < 100 {
				break
			}
		}
	}
	return nil
}
