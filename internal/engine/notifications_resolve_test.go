package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
)

// ticketFixture is a chat-queue shaped DocType: an event rule that tells two
// users a ticket is waiting, and resolves once it no longer is (#140).
func ticketFixture() map[string]string {
	return map[string]string{
		"doctypes/ticket/ticket.doctype.ts": `import {defineDoctype} from "@ddcore/sdk";
 export default defineDoctype({name:"Ticket",idGeneration:{field:"title"},fields:[
 {fieldname:"title",fieldtype:"Data",label:"Title"},{fieldname:"status",fieldtype:"Data",label:"Status"},
 {fieldname:"due",fieldtype:"Date",label:"Due"}],
 permissions:[{role:"All",read:true},{role:"System Manager",write:true,create:true,delete:true}]});`,
		"doctypes/ticket/ticket.controller.ts": `import {defineController} from "@ddcore/sdk";
 export default defineController("Ticket",{hasPermission(doc,ptype,user){return user !== "outsider@example.com"}});`,
		"notifications/waiting.notification.ts": `import {defineNotification} from "@ddcore/sdk";
 export default defineNotification({name:"ticket.waiting",doctype:"Ticket",event:"on_update",
 condition:(doc,before)=>doc.status === "Waiting" && (!before || before.status !== "Waiting"),
 resolveWhen:doc=>doc.status !== "Waiting",
 recipients:()=>["other@example.com","reader@example.com"],
 desk:{title:doc=>"Waiting " + doc.title,message:()=>"In the queue"},
 email:{template:"ticket",args:doc=>({title:doc.title})}});`,
		"notifications/overdue.notification.ts": `import {defineNotification} from "@ddcore/sdk";
 export default defineNotification({name:"ticket.overdue",doctype:"Ticket",date:{field:"due",days:0},
 resolveWhen:doc=>doc.status === "Done",
 recipients:()=>["reader@example.com"],
 desk:{title:doc=>"Overdue " + doc.title,message:()=>"Past due"}});`,
		"mail/ticket.mail.ts": `import {defineMailTemplate} from "@ddcore/sdk";
 export default defineMailTemplate({name:"ticket",subject:doc=>"Ticket " + doc.title,body:(doc,b)=>[b.p(doc.title)]});`,
	}
}

func setupTickets(t *testing.T, extra map[string]string) *Engine {
	t.Helper()
	files := ticketFixture()
	for k, v := range extra {
		files[k] = v
	}
	e := setupWith(t, files)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, u := range []string{"reader@example.com", "other@example.com", "outsider@example.com"} {
			d, _ := c.NewDoc("User", Doc{"email": u, "full_name": u})
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		d, _ := c.NewDoc("Ticket", Doc{"title": "T1", "status": "Open", "due": "2020-01-01"})
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func setTicketStatus(t *testing.T, e *Engine, status string) error {
	t.Helper()
	return e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, err := c.GetDoc("Ticket", "T1")
		if err != nil {
			return err
		}
		d["status"] = status
		_, err = c.Save(d, SaveOpts{})
		return err
	})
}

// ticketNotifications lists rule's occurrences on T1, oldest first.
func ticketNotifications(t *testing.T, e *Engine, rule string) []map[string]any {
	t.Helper()
	rows, err := db.Select(context.Background(), e.DB.Pool, `SELECT recipient, read, resolved, email_delivery FROM ddcore_notification
		WHERE rule = $1 AND reference_doctype = 'Ticket' AND reference_id = 'T1' ORDER BY creation, id`, rule)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// open counts the occurrences neither read nor resolved, per recipient.
func openByRecipient(rows []map[string]any) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		if r["read"] != true && r["resolved"] == nil {
			out[db.Str(r["recipient"])]++
		}
	}
	return out
}

func drainNotificationsChanged(ch <-chan Event) bool {
	found := false
	for len(ch) > 0 {
		if (<-ch).Name == "notifications_changed" {
			found = true
		}
	}
	return found
}

// The chat queue: taking the ticket resolves everyone's occurrence and tells
// each inbox; going back to the queue writes one fresh occurrence, not a
// second one on top.
func TestNotificationsResolveWhenForEveryRecipient(t *testing.T) {
	e := setupTickets(t, nil)
	if err := setTicketStatus(t, e, "Waiting"); err != nil {
		t.Fatal(err)
	}
	rows := ticketNotifications(t, e, "ticket.waiting")
	if open := openByRecipient(rows); len(rows) != 2 || open["other@example.com"] != 1 || open["reader@example.com"] != 1 {
		t.Fatalf("after Waiting = %v", rows)
	}
	other := e.Events.Subscribe("other@example.com", nil)
	reader := e.Events.Subscribe("reader@example.com", nil)
	defer e.Events.Unsubscribe(other)
	defer e.Events.Unsubscribe(reader)

	if err := setTicketStatus(t, e, "Taken"); err != nil {
		t.Fatal(err)
	}
	for _, r := range ticketNotifications(t, e, "ticket.waiting") {
		if r["read"] != true || r["resolved"] == nil {
			t.Fatalf("after Taken = %v", r)
		}
	}
	if !drainNotificationsChanged(other) || !drainNotificationsChanged(reader) {
		t.Fatal("a recipient was not told its inbox changed")
	}
	err := e.Run(context.Background(), "reader@example.com", func(c *Ctx) error {
		n, err := c.NotificationCount()
		if n != 0 {
			t.Fatalf("unread after resolve = %d", n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// a save that keeps it resolved resolves nothing new and tells nobody
	if err := setTicketStatus(t, e, "Closed"); err != nil {
		t.Fatal(err)
	}
	if drainNotificationsChanged(reader) {
		t.Fatal("nothing left to resolve, yet the inbox was invalidated")
	}

	if err := setTicketStatus(t, e, "Waiting"); err != nil {
		t.Fatal(err)
	}
	rows = ticketNotifications(t, e, "ticket.waiting")
	if open := openByRecipient(rows); len(rows) != 4 || open["other@example.com"] != 1 || open["reader@example.com"] != 1 {
		t.Fatalf("back in the queue = %v", rows)
	}
}

// resolveWhen runs on every write event, so a date rule resolves on a save.
func TestNotificationsResolveWhenOnADateRule(t *testing.T) {
	e := setupTickets(t, nil)
	if err := e.SweepNotifications(context.Background(), time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if rows := ticketNotifications(t, e, "ticket.overdue"); len(openByRecipient(rows)) != 1 {
		t.Fatalf("after sweep = %v", rows)
	}
	if err := setTicketStatus(t, e, "Done"); err != nil {
		t.Fatal(err)
	}
	for _, r := range ticketNotifications(t, e, "ticket.overdue") {
		if r["read"] != true || r["resolved"] == nil {
			t.Fatalf("after Done = %v", r)
		}
	}
}

// A non-boolean resolveWhen throws and rolls the save back with it.
func TestNotificationsResolveWhenMustReturnBoolean(t *testing.T) {
	e := setupTickets(t, map[string]string{
		"notifications/broken.notification.ts": `import {defineNotification} from "@ddcore/sdk";
 export default defineNotification({name:"ticket.broken",doctype:"Ticket",event:"on_insert",
 resolveWhen:doc=>doc.status === "Broken" ? "yes" : false,
 recipients:()=>["reader@example.com"],desk:{title:()=>"t",message:()=>"m"}});`,
	})
	if err := setTicketStatus(t, e, "Waiting"); err != nil {
		t.Fatal(err)
	}
	err := setTicketStatus(t, e, "Broken")
	if err == nil || !strings.Contains(err.Error(), "resolveWhen must return a boolean") {
		t.Fatalf("save = %v", err)
	}
	if open := openByRecipient(ticketNotifications(t, e, "ticket.waiting")); open["reader@example.com"] != 1 {
		t.Fatal("the failed save resolved the occurrence anyway")
	}
}

// A resolved occurrence's queued email does not leave.
func TestNotificationsResolvedEmailIsWithdrawn(t *testing.T) {
	e := setupTickets(t, nil)
	if err := setTicketStatus(t, e, "Waiting"); err != nil {
		t.Fatal(err)
	}
	if err := setTicketStatus(t, e, "Taken"); err != nil {
		t.Fatal(err)
	}
	rows := deliveries(t, e)
	if len(rows) == 0 {
		t.Fatal("no email was queued")
	}
	for _, row := range rows {
		if _, err := e.RunJob(context.Background(), "Admin", mailJobMethod, map[string]any{"delivery": row["id"]}); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range deliveries(t, e) {
		if row["status"] != MailFailed || db.Str(row["error"]) != "Notification resolved" || fmt.Sprint(row["attempts"]) != "0" {
			t.Fatalf("resolved mail = %+v", row)
		}
	}
}

// ddcore.notifications.resolve: the caller's read access unless it opts out,
// a known rule, and the core's own rules too.
func TestNotificationsResolveFromServerCode(t *testing.T) {
	e := setupTickets(t, nil)
	ctx := context.Background()
	if err := setTicketStatus(t, e, "Waiting"); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.NotifyUser("reader@example.com", "Ticket", "T1", "Assigned", "T1 is yours")
	}); err != nil {
		t.Fatal(err)
	}
	resolve := func(user, args string) (string, error) {
		var out string
		err := e.Run(ctx, user, func(c *Ctx) error {
			rt, err := c.RT()
			if err != nil {
				return err
			}
			v, err := rt.Eval(`String(ddcore.notifications.resolve(` + args + `).resolved)`)
			if err != nil {
				return err
			}
			out = v.String()
			return nil
		})
		return out, err
	}
	waiting := `{rule: "ticket.waiting", doctype: "Ticket", id: "T1"}`
	if _, err := resolve("outsider@example.com", waiting); err == nil {
		t.Fatal("a user who cannot read the ticket resolved its notifications")
	}
	if open := openByRecipient(ticketNotifications(t, e, "ticket.waiting")); len(open) != 2 {
		t.Fatalf("refused resolve changed rows: %v", open)
	}
	if _, err := resolve("Admin", `{rule: "ticket.nope", doctype: "Ticket", id: "T1"}`); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unknown rule = %v", err)
	}
	if n, err := resolve("outsider@example.com", waiting+`, {ignorePermissions: true}`); err != nil || n != "2" {
		t.Fatalf("resolve with ignorePermissions = %s, %v", n, err)
	}
	if open := openByRecipient(ticketNotifications(t, e, "ticket.waiting")); len(open) != 0 {
		t.Fatalf("still open = %v", open)
	}
	if n, err := resolve("reader@example.com", `{rule: "assignment", doctype: "Ticket", id: "T1"}`); err != nil || n != "1" {
		t.Fatalf("resolve assignment = %s, %v", n, err)
	}
	if open := openByRecipient(ticketNotifications(t, e, "assignment")); len(open) != 0 {
		t.Fatalf("assignment still open = %v", open)
	}
}
