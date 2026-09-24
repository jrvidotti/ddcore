package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

// legacyRefs are the core fields that held another document's key under a
// `*_name` column before 0.17, as (table, column now, column then).
var legacyRefs = [][3]string{
	{"tab_comment", "reference_id", "reference_name"},
	{"tab_to_do", "reference_id", "reference_name"},
	{"tab_email_delivery", "reference_id", "reference_name"},
	{"tab_webhook_delivery", "reference_id", "reference_name"},
	{"tab_document_share", "share_id", "share_name"},
	{"tab_file", "attached_to_id", "attached_to_name"},
	{"tab_audit_event", "target_id", "target_name"},
	{"tab_version", "doc_id", "docname"},
}

// A Single, and a DocType that declares a data field called `name` — legal
// from 0.17 on, and the case the sweep must not mistake for the old key.
var pkRenameFixtures = map[string]string{
	"doctypes/settings/settings.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Settings", isSingle: true, fields: [
 {fieldname: "enabled", fieldtype: "Check", label: "Enabled"}
], permissions: [{role: "All", read: true, write: true}]});`,
	"doctypes/contato/contato.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Contato", fields: [
 {fieldname: "name", fieldtype: "Data", label: "Name"}
], permissions: [{role: "System Manager", read: true, write: true, create: true}]});`,
}

// degradeToLegacyPK turns a database this binary migrated into one a 0.16
// binary would have left: the key called `name` on every DocType table and in
// ddcore_notification, and the reference fields — with their indexes, which a
// column rename does not carry — under their `*_name` columns.
func degradeToLegacyPK(t *testing.T, e *Engine) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string) {
		t.Helper()
		if _, err := e.DB.Pool.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// Tables that already have a `name` column are 0.17 DocTypes declaring it
	// as data: a 0.16 database could not have had them, so they stay as they are.
	for _, r := range sqlRows(t, e, `
		SELECT t.table_name FROM information_schema.tables t
		WHERE t.table_schema = current_schema() AND t.table_type = 'BASE TABLE' AND t.table_name LIKE 'tab\_%'
		  AND EXISTS (SELECT 1 FROM information_schema.columns c WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name AND c.column_name = 'id')
		  AND NOT EXISTS (SELECT 1 FROM information_schema.columns c WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name AND c.column_name = 'name')`) {
		exec(`ALTER TABLE ` + db.Ident(db.Str(r["table_name"])) + ` RENAME COLUMN id TO name`)
	}
	for _, ref := range legacyRefs {
		exec(fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, ref[0], ref[1], ref[2]))
		if indexDef(t, e, ref[0]+"_"+ref[1]) != "" {
			exec(fmt.Sprintf(`ALTER INDEX %s RENAME TO %s`, ref[0]+"_"+ref[1], ref[0]+"_"+ref[2]))
		}
	}
	exec(`ALTER TABLE ddcore_notification RENAME COLUMN id TO name`)
	exec(`ALTER TABLE ddcore_notification RENAME COLUMN reference_id TO reference_name`)
	exec(`ALTER TABLE ddcore_notification_due RENAME COLUMN reference_id TO reference_name`)
}

func TestMigrateMovesTheDocumentKeyFromNameToID(t *testing.T) {
	e := setupWith(t, pkRenameFixtures)
	ctx := context.Background()
	pessoa := insertPessoa(t, e, Doc{"nome": "Ana", "tipo": "PF"})
	var pedido string
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		saved, err := c.Insert(Doc{"doctype": "Pedido", "cliente": pessoa, "itens": []any{
			map[string]any{"descricao": "Cadeira", "qtd": 1, "preco": 10},
		}}, SaveOpts{})
		if err != nil {
			return err
		}
		pedido = saved.ID()
		if _, err := c.Insert(Doc{"doctype": "Comment", "reference_doctype": "Pessoa", "reference_id": pessoa,
			"content": "hello", "comment_type": "Comment"}, SaveOpts{}); err != nil {
			return err
		}
		_, err = c.Insert(Doc{"doctype": "Contato", "name": "Bia"}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	degradeToLegacyPK(t, e)
	migrar(t, e, false, "0.16 -> 0.17")

	// The key moved, with its rows.
	if columnType(t, e, "tab_pessoa", "name") != "" || columnType(t, e, "tab_pessoa", "id") != "text" {
		t.Fatal("tab_pessoa still keyed by name")
	}
	if rows := sqlRows(t, e, `SELECT 1 FROM tab_pessoa WHERE id = $1`, pessoa); len(rows) != 1 {
		t.Fatal("the document did not survive the rename")
	}
	// The primary key kept its name and followed the column: no index rename,
	// and the duplicate error still recognises it by its `_pkey` suffix.
	pk := sqlRows(t, e, `SELECT conname, pg_get_constraintdef(oid) AS def FROM pg_constraint
		WHERE conrelid = 'tab_pessoa'::regclass AND contype = 'p'`)
	if len(pk) != 1 || db.Str(pk[0]["conname"]) != "tab_pessoa_pkey" || db.Str(pk[0]["def"]) != "PRIMARY KEY (id)" {
		t.Fatalf("primary key = %v", pk)
	}
	// The Single's identity check followed the column too, and still holds.
	chk := sqlRows(t, e, `SELECT pg_get_constraintdef(oid) AS def FROM pg_constraint
		WHERE conrelid = 'tab_settings'::regclass AND conname = 'ddcore_single_identity'`)
	if len(chk) != 1 || !strings.Contains(db.Str(chk[0]["def"]), "id = 'singleton'") {
		t.Fatalf("single identity check = %v", chk)
	}
	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO tab_settings (id) VALUES ('other')`); err == nil {
		t.Fatal("the Single accepted a second row")
	}
	// A child stays attached through `parent`, which did not move.
	if rows := sqlRows(t, e, `SELECT 1 FROM tab_item_pedido WHERE parent = $1`, pedido); len(rows) != 1 {
		t.Fatal("the child row lost its parent")
	}
	// The reference fields were renamed by their declarations, indexes included.
	for _, ref := range legacyRefs {
		if columnType(t, e, ref[0], ref[2]) != "" || columnType(t, e, ref[0], ref[1]) != "text" {
			t.Errorf("%s: %s was not renamed to %s", ref[0], ref[2], ref[1])
		}
		if indexDef(t, e, ref[0]+"_"+ref[2]) != "" {
			t.Errorf("%s: the index on %s was left behind", ref[0], ref[2])
		}
	}
	if indexDef(t, e, "tab_comment_reference_id") == "" {
		t.Error("tab_comment_reference_id index missing")
	}
	if rows := sqlRows(t, e, `SELECT 1 FROM tab_comment WHERE reference_id = $1`, pessoa); len(rows) != 1 {
		t.Error("the comment lost its reference")
	}
	if def := indexDef(t, e, "tab_document_share_uk_user_document"); !strings.Contains(def, "share_id") {
		t.Errorf("document share key = %q", def)
	}
	// The framework's own tables.
	if columnType(t, e, "ddcore_notification", "id") != "text" || columnType(t, e, "ddcore_notification", "reference_id") != "text" ||
		columnType(t, e, "ddcore_notification_due", "reference_id") != "text" {
		t.Error("ddcore_notification not moved to id")
	}
	if def := indexDef(t, e, "ddcore_notification_inbox_unread_first"); !strings.Contains(def, "id DESC") {
		t.Errorf("notification inbox index = %q", def)
	}
	// What genuinely is a name stays one.
	if columnType(t, e, "ddcore_vault", "name") != "text" || columnType(t, e, "ddcore_patch", "name") != "text" {
		t.Error("a ddcore_* name column was renamed")
	}
	if columnType(t, e, "ddcore_series", "id") != "" {
		t.Error("ddcore_series gained an id")
	}
	// A DocType declaring a `name` field keeps it as data, beside its key.
	c := sqlRows(t, e, `SELECT id, name FROM tab_contato`)
	if len(c) != 1 || db.Str(c[0]["name"]) != "Bia" || db.Str(c[0]["id"]) == "" || db.Str(c[0]["id"]) == "Bia" {
		t.Fatalf("contato = %v", c)
	}

	// Once done, it is done.
	migrar(t, e, false, "again")
}

// With prune, the sweep must have run before Plan looks for columns to drop,
// or every table would refuse on a `name` column that still holds data.
func TestMigratePruneMovesTheDocumentKey(t *testing.T) {
	e := setupWith(t, pkRenameFixtures)
	insertPessoa(t, e, Doc{"nome": "Ana", "tipo": "PF"})
	degradeToLegacyPK(t, e)
	// The dry run still shows the reference fields' renames, but nothing that
	// adds a key column or drops the old one.
	plan, err := e.Plan(context.Background(), true)
	if err != nil {
		t.Fatalf("a dry run on a 0.16 database: %v", err)
	}
	for _, st := range plan {
		if strings.Contains(st.SQL, `ADD COLUMN "id"`) || strings.Contains(st.SQL, `DROP COLUMN "name"`) {
			t.Fatalf("the dry run touches the key: %s", st.SQL)
		}
	}
	migrar(t, e, true, "prune")
	if columnType(t, e, "tab_pessoa", "id") != "text" {
		t.Fatal("tab_pessoa not moved")
	}
}

// On a database that already says `id`, the sweep finds nothing.
func TestRenameLegacyPKLeavesANewDatabaseAlone(t *testing.T) {
	e := setupWith(t, pkRenameFixtures)
	n, err := db.RenameLegacyPK(context.Background(), e.DB.Pool)
	if err != nil || n != 0 {
		t.Fatalf("moved %d tables, err %v", n, err)
	}
}
