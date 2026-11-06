package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// #144 — an index name past Postgres's 63 bytes. Postgres truncated it, the
// planner looked for the full name, missed, and every migrate after the first
// failed with "relation … already exists".

const (
	longTable = "tab_long_produto_estoque_referencia"
	// tab_long_produto_estoque_referencia_codigo_produto_estoque_referencia: 69 bytes
	longSearch = "codigo_produto_estoque_referencia"
	// tab_long_produto_estoque_referencia_codigo_unico_estoque_referencia: 67 bytes
	longUnique = "codigo_unico_estoque_referencia"
)

func setupLongIndexes(t *testing.T) *Engine {
	t.Helper()
	return setupWith(t, map[string]string{
		"doctypes/long_produto_estoque_referencia/long_produto_estoque_referencia.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Long Produto Estoque Referencia", fields: [
  { fieldname: "` + longSearch + `", fieldtype: "Data", label: "Code", searchIndex: true },
  { fieldname: "` + longUnique + `", fieldtype: "Data", label: "Unique code", unique: true } ] });`,
	})
}

func TestLongIndexNameMigratesTwice(t *testing.T) {
	e := setupLongIndexes(t)
	// setupWith migrated once; migrar migrates again and insists on an empty
	// plan after it — the run that used to fail.
	migrar(t, e, false, "second migrate")

	for _, f := range []string{longSearch, longUnique} {
		name := meta.IndexName(longTable, f)
		if len(name) != meta.MaxIdentifier {
			t.Fatalf("%s: index name %q is %d bytes, want %d", f, name, len(name), meta.MaxIdentifier)
		}
		if def := indexDef(t, e, name); !strings.Contains(def, f) {
			t.Fatalf("%s: no index under %q: %q", f, name, def)
		}
	}
	if def := indexDef(t, e, meta.IndexName(longTable, longUnique)); !strings.Contains(def, "UNIQUE") {
		t.Fatalf("the unique field's index is not unique: %q", def)
	}
}

// A site migrated before the fix holds the index under the name Postgres cut
// it to. The next migrate takes it over by renaming, rather than building a
// second copy of it beside the first.
func TestTruncatedIndexIsAdopted(t *testing.T) {
	e := setupLongIndexes(t)
	ctx := context.Background()
	hashed := meta.IndexName(longTable, longSearch)
	full := longTable + "_" + longSearch
	legacy := meta.LegacyIndexNames(longTable, longSearch)[1]
	// What a migrate before #144 left: the full name, which Postgres truncates.
	if _, err := e.DB.Pool.Exec(ctx, "DROP INDEX "+db.Ident(hashed)+"; CREATE INDEX "+full+" ON "+longTable+" ("+longSearch+")"); err != nil {
		t.Fatal(err)
	}
	if indexDef(t, e, legacy) == "" {
		t.Fatalf("Postgres did not keep the index under %q", legacy)
	}

	res := migrar(t, e, false, "adopt the truncated index")

	var renamed bool
	for _, st := range res.DDL {
		switch st.Kind {
		case db.KindRenameIndex:
			renamed = renamed || st.OldName == legacy
		case db.KindCreateIndex, db.KindDropIndex:
			t.Fatalf("the index was rebuilt instead of adopted: %q", st.SQL)
		}
	}
	if !renamed {
		t.Fatalf("no rename of %q in the plan: %v", legacy, db.SQL(res.DDL))
	}
	if indexDef(t, e, legacy) != "" || indexDef(t, e, hashed) == "" {
		t.Fatalf("the index is not under %q alone", hashed)
	}
}

func TestRenameFieldWithLongIndexName(t *testing.T) {
	e := setupLongIndexes(t)
	d, _ := e.Current().Meta.Get("Long Produto Estoque Referencia")
	f := d.Field(longSearch)
	f.Fieldname = "codigo_produto_estoque_referencia_novo"
	f.RenamedFrom = meta.Names{longSearch}
	d.ResetFieldIndex()

	res := migrar(t, e, false, "rename a field with a long index")

	for _, st := range res.DDL {
		if st.Kind == db.KindDropIndex || st.Kind == db.KindCreateIndex {
			t.Fatalf("the index was rebuilt instead of renamed: %q", st.SQL)
		}
	}
	if def := indexDef(t, e, meta.IndexName(longTable, f.Fieldname)); !strings.Contains(def, f.Fieldname) {
		t.Fatalf("the index did not follow the column: %q", def)
	}
}

func TestRenameDocTypeWithLongIndexNames(t *testing.T) {
	e := setupLongIndexes(t)
	reg := e.Current().Meta
	d, _ := reg.Get("Long Produto Estoque Referencia")
	d.Name = "Long Produto Estoque Referencia Nova"
	d.RenamedFrom = meta.Names{"Long Produto Estoque Referencia"}
	delete(reg.DocTypes, "Long Produto Estoque Referencia")
	reg.DocTypes[d.Name] = d

	res := migrar(t, e, false, "rename a DocType with long indexes")

	for _, st := range res.DDL {
		if st.Kind == db.KindDropIndex || st.Kind == db.KindCreateIndex {
			t.Fatalf("an index was rebuilt instead of renamed: %q", st.SQL)
		}
	}
	nt := d.TableName()
	for _, f := range []string{longSearch, longUnique, "modified"} {
		if indexDef(t, e, meta.IndexName(nt, f)) == "" {
			t.Fatalf("no index %q after the rename", meta.IndexName(nt, f))
		}
	}
}

// The race path names the index a unique field hit; with a long fieldname that
// name carries a hash, and the message still has to name the field.
func TestADuplicateOnALongUniqueFieldNamesTheField(t *testing.T) {
	e := setupLongIndexes(t)
	constraint := meta.IndexName(longTable, longUnique)
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: constraint,
		Message: `duplicate key value violates unique constraint "` + constraint + `"`}
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.St.DocType("Long Produto Estoque Referencia")
		got := cerr.From(c.duplicateErr(d, Doc{"id": "X", longUnique: "X1"}, pgErr))
		if got.Key != `{0} "{1}" already exists` {
			t.Fatalf("key = %q, want the field-specific sentence", got.Key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// #149 — the index of Product.model was named tab_product_model, the table of
// Product Model: Postgres keeps both in one namespace, and migrate failed with
// 42P07 whichever of the two came second.
func TestALinkIndexDoesNotTakeAnotherDocTypesTable(t *testing.T) {
	e := setupWith(t, map[string]string{
		"doctypes/product_model/product_model.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Product Model", idGeneration: { hash: true }, fields: [
  { fieldname: "model_name", fieldtype: "Data", label: "Model name", reqd: true } ] });`,
		"doctypes/product/product.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Product", idGeneration: { hash: true }, fields: [
  { fieldname: "model", fieldtype: "Link", label: "Model", options: "Product Model" } ] });`,
	})
	migrar(t, e, false, "second migrate")
	if def := indexDef(t, e, "tab_product__model"); !strings.Contains(def, "tab_product ") {
		t.Fatalf("Product.model has no index of its own: %q", def)
	}
	if rows := sqlRows(t, e, `SELECT 1 FROM pg_tables WHERE tablename = 'tab_product_model'`); len(rows) != 1 {
		t.Fatal("the Product Model table is missing")
	}
}

// A site migrated before #149 holds every index under "<table>_<suffix>". The
// next migrate renames each one to the current name — which leaves the rows
// and the index alone — rather than building it a second time.
func TestIndexesNamedByTheOldSchemeAreRenamed(t *testing.T) {
	e := setupWithKey(t)
	ctx := context.Background()
	rows := sqlRows(t, e, `SELECT indexname FROM pg_indexes WHERE schemaname = current_schema()
		AND tablename LIKE 'tab\_%' AND indexname LIKE '%\_\_%'`)
	if len(rows) == 0 {
		t.Fatal("no index to rename back")
	}
	renamed := map[string]bool{}
	for _, r := range rows {
		name := db.Str(r["indexname"])
		if len(name) == meta.MaxIdentifier {
			continue // a hashed name: TestTruncatedIndexIsAdopted's case
		}
		old := strings.Replace(name, "__", "_", 1)
		if _, err := e.DB.Pool.Exec(ctx, "ALTER INDEX "+db.Ident(name)+" RENAME TO "+db.Ident(old)); err != nil {
			t.Fatal(err)
		}
		renamed[old] = true
	}
	if !renamed["tab_pessoa_uk_tipo_codigo"] || !renamed["tab_pessoa_modified"] {
		t.Fatalf("the fixture lost the indexes it is about: %v", renamed)
	}

	res := migrar(t, e, false, "rename the old names")

	for _, st := range res.DDL {
		switch {
		case st.Kind == db.KindRenameIndex:
			delete(renamed, st.OldName)
		case st.Kind == db.KindCreateIndex || st.Kind == db.KindDropIndex:
			t.Fatalf("an index was rebuilt instead of renamed: %q", st.SQL)
		}
	}
	if len(renamed) > 0 {
		t.Fatalf("left under the old name: %v", renamed)
	}
	if indexDef(t, e, "tab_pessoa__uk_tipo_codigo") == "" {
		t.Fatal("the key is not under its current name")
	}
	// The primary key is Postgres's to name, and keeps its name.
	if indexDef(t, e, "tab_pessoa_pkey") == "" {
		t.Fatal("the primary key was renamed")
	}
}

// The upgrade itself can meet #149: an index the old scheme named
// tab_product_model, and a Product Model DocType arriving in the same release.
// The rename has to run before the CREATE TABLE that needs the name.
func TestAnOldIndexNameMakesWayForANewTable(t *testing.T) {
	e := setupWith(t, map[string]string{
		"doctypes/product/product.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Product", idGeneration: { hash: true }, fields: [
  { fieldname: "model", fieldtype: "Data", label: "Model", searchIndex: true } ] });`,
	})
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `ALTER INDEX tab_product__model RENAME TO tab_product_model`); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(e.Current().App("demo").Dir, "doctypes/product_model")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "product_model.doctype.ts"), []byte(`import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Product Model", idGeneration: { hash: true }, fields: [
  { fieldname: "model_name", fieldtype: "Data", label: "Model name" } ] });`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}

	migrar(t, e, false, "rename the index, then create the table")

	if indexDef(t, e, "tab_product__model") == "" {
		t.Fatal("Product.model's index is not under its current name")
	}
	if rows := sqlRows(t, e, `SELECT 1 FROM pg_tables WHERE tablename = 'tab_product_model'`); len(rows) != 1 {
		t.Fatal("the Product Model table is missing")
	}
}
