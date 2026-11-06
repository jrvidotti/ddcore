package engine

import (
	"context"
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
	legacy := meta.TruncatedIndexName(longTable, longSearch)
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
