package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNotificationDueCalendarDays(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	due := notificationDue("2026-03-07T12:00:00-05:00", "Datetime", 1, loc)
	if due.Format(time.RFC3339) != "2026-03-08T12:00:00-04:00" {
		t.Fatal(due)
	}
	due = notificationDue("2026-03-08", "Date", 1, loc)
	if due.Format(time.RFC3339) != "2026-03-09T00:00:00-04:00" {
		t.Fatal(due)
	}
}

func TestNotificationsCommitRollbackAndRecipients(t *testing.T) {
	e := setupWith(t, map[string]string{
		"notifications/person.notification.ts": `import {defineNotification} from "@ddcore/sdk";
export default defineNotification({name:"person",doctype:"Pessoa",event:"on_insert",
 recipients(doc) { return ["Administrator", "Administrator", "missing@example.com", "Guest"] },
 desk:{title(doc){return "Created " + doc.nome},message(doc){return "Hello"}}});`,
	})
	ctx := context.Background()
	own := e.Events.Subscribe("Administrator", nil)
	other := e.Events.Subscribe("another@example.com", nil)
	defer e.Events.Unsubscribe(own)
	defer e.Events.Unsubscribe(other)
	abort := errors.New("abort")
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		d, _ := c.NewDoc("Pessoa", Doc{"nome": "rollback"})
		if _, err := c.Insert(d, SaveOpts{}); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	select {
	case ev := <-own:
		t.Fatalf("rollback emitted %#v", ev)
	default:
	}
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		n, err := c.NotificationCount()
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatal(n)
		}
		d, _ := c.NewDoc("Pessoa", Doc{"nome": "committed"})
		_, err = c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for len(own) > 0 {
		if (<-own).Name == "notifications_changed" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing notification invalidation")
	}
	for len(other) > 0 {
		if (<-other).Name == "notifications_changed" {
			t.Fatal("recipient leak")
		}
	}
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		page, err := c.ListNotifications(20, 0, nil)
		if err != nil {
			return err
		}
		if page.Total != 1 || len(page.Data) != 1 || page.Data[0].Title != "Created committed" {
			t.Fatalf("%+v", page)
		}
		if _, err = c.SetNotificationRead(page.Data[0].Name, true); err != nil {
			return err
		}
		n, err := c.NotificationCount()
		if n != 0 {
			t.Fatal(n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
