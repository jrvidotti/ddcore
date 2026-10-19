package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

// Insert and Save announce a write in a fixed order: for each event, its
// webhooks are queued before its notifications, so a notification sees the
// deliveries of its own event and of every earlier one, never of a later one;
// then the realtime doc_update carries the write's action. Each notification
// below records the deliveries it saw.
func TestAfterWriteEventOrder(t *testing.T) {
	t.Setenv("DDCORE_SECRET_KEY", "webhook-test-master-key")
	e := setupWith(t, map[string]string{"notifications/pedido.notification.ts": `import {defineNotification} from "@ddcore/sdk";
for (const event of ["on_insert","on_update","on_submit","on_cancel"]) defineNotification({name:event,doctype:"Pedido",event,recipients:()=>["Admin"],
 desk:{title:()=>event,message:(doc)=>ddcore.db.getList("Webhook Delivery",{filters:{reference_id:doc.id},fields:["event"]}).map((r)=>r.event).sort().join(",")}});`})
	rcv := newReceiver(t)
	addWebhook(t, e, rcv.URL, Doc{"webhook_doctype": "Pedido", "on_submit": true, "on_cancel": true})
	insertHookPessoa(t, e, "Order customer")
	events := e.Events.Subscribe("Admin", func(string, string) bool { return true })
	defer e.Events.Unsubscribe(events)

	ctx := context.Background()
	var drafted, submitted string
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		order, _ := c.NewDoc("Pedido", Doc{"cliente": "Order customer", "obs": "First"})
		order, err := c.Insert(order, SaveOpts{})
		if err != nil {
			return err
		}
		drafted = order.ID()
		order["obs"] = "Second"
		if order, err = c.Save(order, SaveOpts{}); err != nil {
			return err
		}
		if order, err = c.Submit(order); err != nil {
			return err
		}
		if _, err = c.Cancel(order); err != nil {
			return err
		}
		direct, _ := c.NewDoc("Pedido", Doc{"cliente": "Order customer", "docstatus": 1})
		direct, err = c.Insert(direct, SaveOpts{})
		submitted = direct.ID()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.Select(ctx, e.DB.Pool, `SELECT rule, reference_id, message FROM ddcore_notification WHERE reference_doctype = 'Pedido'`)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, r := range rows {
		seen[db.Str(r["reference_id"])+" "+db.Str(r["rule"])] = db.Str(r["message"])
	}
	want := map[string]string{
		drafted + " on_insert":   "doc.on_insert",
		drafted + " on_update":   "doc.on_insert,doc.on_update",
		drafted + " on_submit":   "doc.on_insert,doc.on_submit,doc.on_update",
		drafted + " on_cancel":   "doc.on_cancel,doc.on_insert,doc.on_submit,doc.on_update",
		submitted + " on_insert": "doc.on_insert",
		submitted + " on_submit": "doc.on_insert,doc.on_submit",
	}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("deliveries each notification saw:\n got %v\nwant %v", seen, want)
	}

	var actions []string
	for done := false; !done; {
		select {
		case ev := <-events:
			if p, ok := ev.Payload.(map[string]any); ok && ev.Name == "doc_update" && p["doctype"] == "Pedido" {
				actions = append(actions, fmt.Sprint(p["id"] == submitted, ":", p["action"]))
			}
		default:
			done = true
		}
	}
	if got := strings.Join(actions, ","); got != "false:insert,false:save,false:submit,false:cancel,true:insert" {
		t.Fatalf("doc_update actions = %s", got)
	}
}
