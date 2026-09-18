package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

func TestNotificationsLifecycleAndPreviousDocument(t *testing.T) {
	files := map[string]string{}
	for _, event := range []string{"on_insert", "on_update", "on_submit", "on_cancel"} {
		files["notifications/"+event+".notification.ts"] = `import {defineNotification} from "@ddcore/sdk";
export default defineNotification({name:"` + event + `",doctype:"Pedido",event:"` + event + `",
 condition(doc,before) { return "` + event + `" !== "on_update" || doc.obs !== before.obs },
 recipients(){return ["Admin"]},desk:{title(){return "` + event + `"},message(doc,before){return JSON.stringify({current:doc.docstatus,previous:before ? before.docstatus : null,obs:doc.obs,old:before ? before.obs : null})}}});`
	}
	e := setupWith(t, files)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		customer, _ := c.NewDoc("Pessoa", Doc{"nome": "Lifecycle customer"})
		if _, err := c.Insert(customer, SaveOpts{}); err != nil {
			return err
		}
		order, _ := c.NewDoc("Pedido", Doc{"cliente": "Lifecycle customer", "obs": "First"})
		order, err := c.Insert(order, SaveOpts{})
		if err != nil {
			return err
		}
		order["obs"] = "Second"
		order, err = c.Save(order, SaveOpts{})
		if err != nil {
			return err
		}
		// Unchanged saves remain events but the rule's previous-document condition suppresses them.
		order, err = c.Save(order, SaveOpts{})
		if err != nil {
			return err
		}
		order, err = c.Submit(order)
		if err != nil {
			return err
		}
		if _, err = c.Cancel(order); err != nil {
			return err
		}
		submitted, _ := c.NewDoc("Pedido", Doc{"cliente": "Lifecycle customer", "obs": "Direct", "docstatus": 1})
		_, err = c.Insert(submitted, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Select(context.Background(), e.DB.Pool, `SELECT rule,message FROM ddcore_notification ORDER BY creation,name`)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, row := range rows {
		event := db.Str(row["rule"])
		counts[event]++
		message := db.Str(row["message"])
		switch event {
		case "on_update":
			if message != `{"current":0,"previous":0,"obs":"Second","old":"First"}` {
				t.Fatal(message)
			}
		case "on_cancel":
			if message != `{"current":2,"previous":1,"obs":"Second","old":"Second"}` {
				t.Fatal(message)
			}
		case "on_submit":
			if !strings.Contains(message, `"current":1`) || (!strings.Contains(message, `"previous":0`) && !strings.Contains(message, `"previous":null`)) {
				t.Fatal(message)
			}
		case "on_insert":
			if !strings.Contains(message, `"previous":null`) {
				t.Fatal(message)
			}
		}
	}
	if fmt.Sprint(counts) != fmt.Sprint(map[string]int{"on_insert": 2, "on_update": 1, "on_submit": 2, "on_cancel": 1}) {
		t.Fatalf("events: %v", counts)
	}
}

func TestNotificationsDBSetRenameDeleteDoNotTrigger(t *testing.T) {
	e := setupWith(t, map[string]string{"notifications/person.notification.ts": `import {defineNotification} from "@ddcore/sdk";
for (const event of ["on_insert","on_update"]) defineNotification({name:event,doctype:"Pessoa",event,recipients:()=>["Admin"],desk:{title:()=>event,message:()=>"Message"}});`})
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		person, _ := c.NewDoc("Pessoa", Doc{"nome": "Original"})
		if _, err := c.Insert(person, SaveOpts{}); err != nil {
			return err
		}
		if _, err := c.DBSet("Pessoa", "Original", Doc{"email": "person@example.com"}, true); err != nil {
			return err
		}
		if _, err := c.Rename("Pessoa", "Original", "Renamed"); err != nil {
			return err
		}
		page, err := c.ListNotifications(20, 0, nil)
		if err != nil {
			return err
		}
		if page.Total != 1 || page.Data[0].ReferenceName != "Renamed" {
			t.Fatalf("rename reference: %+v", page)
		}
		return c.Delete("Pessoa", "Renamed", false, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT rule,reference_name FROM ddcore_notification`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || db.Str(rows[0]["rule"]) != "on_insert" || db.Str(rows[0]["reference_name"]) != "Renamed" {
		t.Fatalf("unexpected occurrences: %+v", rows)
	}
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		page, err := c.ListNotifications(20, 0, nil)
		if page.Total != 0 {
			t.Fatal("deleted document remains visible")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationsEvaluationErrorRollsBackSourceMailAndEvents(t *testing.T) {
	files := map[string]string{
		"mail/update.mail.ts": `import {defineMailTemplate} from "@ddcore/sdk";export default defineMailTemplate({name:"notice",subject:()=>"Notice",body:(args,b)=>[b.p("Message")]});`,
		"notifications/person.notification.ts": `import {defineNotification} from "@ddcore/sdk";
defineNotification({name:"insert",doctype:"Pessoa",event:"on_insert",recipients:()=>["Admin"],desk:{title:()=>"Created",message:()=>"Message"},email:{template:"notice",args:()=>({})}});
defineNotification({name:"update",doctype:"Pessoa",event:"on_update",condition(){throw new Error("notification evaluation failed")},recipients:()=>["Admin"],desk:{title:()=>"Changed",message:()=>"Message"}});`,
	}
	e := setupWith(t, files)
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.DBSet("User", "Admin", Doc{"email": "admin@example.com"}, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	events := e.Events.Subscribe("Admin", nil)
	defer e.Events.Unsubscribe(events)
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		person, _ := c.NewDoc("Pessoa", Doc{"nome": "Rollback evaluation"})
		person, err := c.Insert(person, SaveOpts{})
		if err != nil {
			return err
		}
		// Verify the first operation really staged every effect before the second fails.
		for _, table := range []string{"ddcore_notification", "tab_email_delivery", "ddcore_job"} {
			var count int
			if err := c.Q().QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				t.Fatalf("%s staged %d rows", table, count)
			}
		}
		person["email"] = "changed@example.com"
		_, err = c.Save(person, SaveOpts{})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "notification evaluation failed") {
		t.Fatalf("wrong failure: %v", err)
	}
	for _, table := range []string{"tab_pessoa", "ddcore_notification", "tab_email_delivery", "ddcore_job"} {
		var count int
		if err := e.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback left %d rows in %s", count, table)
		}
	}
	select {
	case event := <-events:
		t.Fatalf("rollback published %+v", event)
	default:
	}
}
