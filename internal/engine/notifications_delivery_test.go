package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/mail"
)

func notificationFixture() map[string]string {
	return map[string]string{
		"doctypes/reminder/reminder.doctype.ts": `import {defineDoctype} from "@ddcore/sdk";
 export default defineDoctype({name:"Reminder",allowRename:true,naming:{field:"title"},fields:[
 {fieldname:"title",fieldtype:"Data",label:"Title"},{fieldname:"due",fieldtype:"Date",label:"Due"},
 {fieldname:"valid",fieldtype:"Check",label:"Valid",default:true},{fieldname:"reader",fieldtype:"Data",label:"Reader"}],
 permissions:[{role:"All",read:true},{role:"System Manager",write:true,create:true,delete:true}]});`,
		"doctypes/reminder/reminder.controller.ts": `import {defineController} from "@ddcore/sdk";
 export default defineController("Reminder",{hasPermission(doc,ptype,user){return !doc || doc.valid},permissionQuery(user){return {reader:user}}});`,
		"notifications/due.notification.ts": `import {defineNotification,_} from "@ddcore/sdk";
 export default defineNotification({name:"due",doctype:"Reminder",date:{field:"due",days:0},condition:doc=>doc.valid,
 recipients:doc=>[doc.reader,"disabled@example.com","missing@example.com"],
 desk:{title:doc=>_("Reminder for {0}",[doc.title]),message:doc=>"Plain <text>"},
 email:{template:"reminder",args:doc=>({title:doc.title})}});`,
		"mail/reminder.mail.ts": `import {defineMailTemplate,_} from "@ddcore/sdk";
 export default defineMailTemplate({name:"reminder",subject:doc=>_("Reminder for {0}",[doc.title]),body:(doc,b)=>[b.p(doc.title)]});`,
		"translations/pt-BR.csv": "Reminder for {0},Lembrete para {0},\n",
	}
}

func seedNotificationUsers(t *testing.T, e *Engine) {
	t.Helper()
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, u := range []struct {
			name, lang string
			enabled    bool
		}{{"reader@example.com", "pt-BR", true}, {"other@example.com", "en", true}, {"disabled@example.com", "en", false}} {
			d, _ := c.NewDoc("User", Doc{"email": u.name, "full_name": u.name, "language": u.lang, "enabled": u.enabled})
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func insertReminder(t *testing.T, e *Engine, name, due string, valid bool) {
	t.Helper()
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Reminder", Doc{"title": name, "due": due, "reader": "reader@example.com", "valid": valid})
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNotificationsDateRecoveryDedupRenameAndRevocation(t *testing.T) {
	e := setupWith(t, notificationFixture())
	ctx := context.Background()
	seedNotificationUsers(t, e)
	insertReminder(t, e, "Historical", "2020-01-01", true)
	insertReminder(t, e, "No longer valid", "2020-01-01", false)
	insertReminder(t, e, "Future", "2099-01-01", true)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- e.SweepNotifications(ctx, now) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	if err := e.SweepNotifications(ctx, now); err != nil {
		t.Fatal(err)
	}
	rows := deliveries(t, e)
	if len(rows) != 1 || rows[0]["subject"] != "Lembrete para Historical" {
		t.Fatalf("%+v", rows)
	}
	err := e.Run(ctx, "reader@example.com", func(c *Ctx) error {
		p, err := c.ListNotifications(20, 0, nil)
		if err != nil {
			return err
		}
		if p.Total != 1 || p.Data[0].Title != "Lembrete para Historical" {
			t.Fatalf("%+v", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "Admin", func(c *Ctx) error { _, err := c.Rename("Reminder", "Historical", "Renamed"); return err })
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SweepNotifications(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(deliveries(t, e)) != 1 {
		t.Fatal("rename duplicated date")
	}
	err = e.Run(ctx, "Admin", func(c *Ctx) error { return c.SetValue("Reminder", "Renamed", Doc{"due": "2021-01-01"}) })
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SweepNotifications(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(deliveries(t, e)) != 2 {
		t.Fatal("changed due date not delivered")
	}
	// Revocation by permissionQuery hides rows and prevents transport, even when
	// the worker's own context is Admin with ignorePermissions enabled.
	err = e.Run(ctx, "Admin", func(c *Ctx) error { return c.SetValue("Reminder", "Renamed", Doc{"reader": "other@example.com"}) })
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "reader@example.com", func(c *Ctx) error {
		n, err := c.NotificationCount()
		if n != 0 {
			t.Fatal(n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range deliveries(t, e) {
		if _, err := e.RunJob(ctx, "Admin", mailJobMethod, map[string]any{"delivery": row["name"]}); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range deliveries(t, e) {
		if row["status"] != MailFailed || fmt.Sprint(row["attempts"]) != "0" {
			t.Fatalf("revoked mail attempted: %+v", row)
		}
	}
}

func TestNotificationsEmailRollbackAndRetry(t *testing.T) {
	files := notificationFixture()
	files["notifications/due.notification.ts"] = `import {defineNotification} from "@ddcore/sdk";
 export default defineNotification({name:"insert",doctype:"Reminder",event:"on_insert",recipients:doc=>[doc.reader],
 desk:{title:doc=>doc.title,message:()=>"Created"},email:{template:"reminder",args:doc=>({title:doc.title})}});`
	e := setupWith(t, files)
	seedNotificationUsers(t, e)
	ctx := context.Background()
	abort := errors.New("abort")
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Reminder", Doc{"title": "Rollback", "reader": "reader@example.com", "valid": true})
		if _, err := c.Insert(d, SaveOpts{}); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if len(deliveries(t, e)) != 0 {
		t.Fatal("rolled back mail survived")
	}
	var jobs int
	if err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM ddcore_job WHERE method=$1`, mailJobMethod).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("jobs %d: %v", jobs, err)
	}
	insertReminder(t, e, "Retry", "2020-01-01", true)
	sender := &notificationRetrySender{}
	e.mailOnce.Do(func() { e.mailer = sender })
	rows := deliveries(t, e)
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	args := map[string]any{"delivery": db.Str(rows[0]["name"])}
	if _, err := e.RunJob(ctx, "Admin", mailJobMethod, args); err == nil {
		t.Fatal("expected transport failure")
	}
	if _, err := e.RunJob(ctx, "Admin", mailJobMethod, args); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RunJob(ctx, "Admin", mailJobMethod, args); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 2 {
		t.Fatalf("duplicate successful transport: %d", sender.calls)
	}
	if len(deliveries(t, e)) != 1 || deliveries(t, e)[0]["status"] != MailSent {
		t.Fatal(deliveries(t, e))
	}
}

// notificationRetrySender fails its first transport call and accepts the rest.
type notificationRetrySender struct{ calls int }

func (s *notificationRetrySender) Send(ctx context.Context, m mail.Message) error {
	s.calls++
	if s.calls == 1 {
		return errors.New("temporary transport failure")
	}
	return nil
}

func (s *notificationRetrySender) Delivers() bool { return true }

func TestNotificationsSweepMarksOnlyMatchedDates(t *testing.T) {
	e := setupWith(t, notificationFixture())
	ctx := context.Background()
	seedNotificationUsers(t, e)
	insertReminder(t, e, "Matched", "2020-01-01", true)
	insertReminder(t, e, "Pending", "2020-01-01", false)
	insertReminder(t, e, "Future", "2099-01-01", true)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	marks := func() []map[string]any {
		t.Helper()
		rows, err := db.Select(ctx, e.DB.Pool, `SELECT reference_name FROM ddcore_notification_due ORDER BY reference_name`)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if err := e.SweepNotifications(ctx, now); err != nil {
		t.Fatal(err)
	}
	if m := marks(); len(m) != 1 || m[0]["reference_name"] != "Matched" {
		t.Fatalf("marks after first sweep: %+v", m)
	}
	// A condition that starts to hold is still picked up on a later sweep.
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.SetValue("Reminder", "Pending", Doc{"valid": true})
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.SweepNotifications(ctx, now); err != nil {
		t.Fatal(err)
	}
	if m := marks(); len(m) != 2 {
		t.Fatalf("marks after condition change: %+v", m)
	}
	if n := len(deliveries(t, e)); n != 2 {
		t.Fatalf("deliveries = %d, want 2", n)
	}
	// Deleting the document removes its mark with it.
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.Delete("Reminder", "Matched", false, false)
	}); err != nil {
		t.Fatal(err)
	}
	if m := marks(); len(m) != 1 || m[0]["reference_name"] != "Pending" {
		t.Fatalf("marks after delete: %+v", m)
	}
}
