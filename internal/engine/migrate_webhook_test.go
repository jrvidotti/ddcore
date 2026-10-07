package engine

import (
	"context"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// alterWebhookTable makes the next migrate ALTER tab_webhook, which takes the
// table's ACCESS EXCLUSIVE lock until the migration commits — what turning
// tenancy on does when it adds the tenant column.
func alterWebhookTable(t *testing.T, e *Engine) {
	t.Helper()
	wh, ok := e.Meta.Get("Webhook")
	if !ok {
		t.Fatal("no Webhook DocType")
	}
	wh.Fields = append(wh.Fields, &meta.Field{Fieldname: "nota_111", Fieldtype: "Data", Label: "Note"})
	wh.ResetFieldIndex()
}

// migrateWithin runs a migration that must finish: a migration that hangs
// fails here instead of holding the suite until its own timeout.
func migrateWithin(t *testing.T, e *Engine) *MigrateResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := e.Migrate(ctx, false)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return res
}

// TestMigrateAlteringWebhookWritesADocument is #111: a migration that alters
// tab_webhook and then writes a document — a new app role, here — read the
// subscriptions on another connection, which waited for the migration's lock
// while the migration waited for it. Nothing ever timed out.
func TestMigrateAlteringWebhookWritesADocument(t *testing.T) {
	e := setupWebhooks(t)
	hook := addWebhook(t, e, "http://127.0.0.1:1/hook", Doc{"webhook_doctype": "Role", "on_update": false})

	alterWebhookTable(t, e)
	e.Snap.Apps["demo"].Roles = append(e.Snap.Apps["demo"].Roles, "Auditor 111")
	migrateWithin(t, e)

	if len(sqlRows(t, e, `SELECT 1 FROM tab_role WHERE id = 'Auditor 111'`)) != 1 {
		t.Fatal("the new role was not created")
	}
	// A subscription committed before the migration still hears what the
	// migration writes, queued on the migration's own transaction.
	var got []string
	for _, d := range hookDeliveries(t, e) {
		if db.Str(d["reference_id"]) == "Auditor 111" {
			got = append(got, db.Str(d["webhook"]))
		}
	}
	if len(got) != 1 || got[0] != hook {
		t.Fatalf("want one delivery to %s for the new role, got %v", hook, got)
	}
}

// TestMigrateWebhookSubscriptionsStayInTheirTenant: inside a migration the
// subscriptions are read on its transaction, and a patch runs that
// transaction elevated — out of row-level security. A platform document a
// patch writes must still reach only the platform's subscriptions.
func TestMigrateWebhookSubscriptionsStayInTheirTenant(t *testing.T) {
	t.Setenv("DDCORE_SECRET_KEY", "webhook-test-master-key")
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, tenancyFiles, map[string]string{
		"patches/0001_pessoa_111.ts": `import { definePatch } from "@ddcore/sdk";
export default definePatch({
  execute(ctx) {
    ddcore.newDoc("Pessoa", { nome: "Da migração", cpf: "111" }).insert({ ignorePermissions: true });
  },
});`,
	})}}, Test: true, Tenancy: true})
	seedTenants(t, e)

	platform := addWebhook(t, e, "http://127.0.0.1:1/hook", nil)
	var other string
	inTenant(t, e, tenantA, func(c *Ctx) error {
		doc, err := c.NewDoc("Webhook", Doc{"url": "http://127.0.0.1:1/hook", "event_type": "Document",
			"webhook_doctype": "Pessoa", "on_insert": true, "secret": hookSecret, "max_attempts": 3})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err == nil {
			other = saved.ID()
		}
		return err
	})

	// the fresh install recorded the patch; make it pending again
	if _, err := e.DB.Sys.Exec(context.Background(), `DELETE FROM ddcore_patch WHERE app = 'demo'`); err != nil {
		t.Fatal(err)
	}
	alterWebhookTable(t, e)
	if res := migrateWithin(t, e); len(res.Patches) != 1 {
		t.Fatalf("the patch should have run, got %v", res.Patches)
	}

	rows, err := db.Select(context.Background(), e.DB.Sys,
		`SELECT d.tenant, d.webhook FROM tab_webhook_delivery d
		 JOIN tab_pessoa p ON p.tenant = d.tenant AND p.id = d.reference_id
		 WHERE p.cpf = '111'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || db.Str(rows[0]["tenant"]) != "" || db.Str(rows[0]["webhook"]) != platform {
		t.Fatalf("want one platform delivery to %s (and none to %s's %s), got %v", platform, tenantA, other, rows)
	}
}
