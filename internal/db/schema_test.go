package db

import (
	"sort"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// A type change migrate makes on its own must hold every value the column had.
// The table is explicit rather than computed: there are nine column types, the
// answers are not derivable from anything, and a wrong entry loses data quietly.
func TestSafeWidening(t *testing.T) {
	safe := [][2]string{
		{"bigint", "text"}, {"numeric(21,9)", "text"}, {"boolean", "text"},
		{"date", "text"}, {"timestamptz", "text"}, {"jsonb", "text"},
		{"bigint", "double precision"}, {"bigint", "numeric(21,9)"},
		{"double precision", "numeric(21,9)"},
	}
	for _, p := range safe {
		if !safeWidening[p] {
			t.Errorf("%s → %s should need no declaration", p[0], p[1])
		}
	}
	destructive := [][2]string{
		// one unparseable row aborts the whole ALTER, with no partial route
		{"text", "bigint"}, {"text", "numeric(21,9)"}, {"text", "date"}, {"text", "jsonb"},
		// loses exactness, and Currency is the precision-correct type (DAT-06)
		{"numeric(21,9)", "double precision"},
		// drops the time, in whatever timezone the connection happens to have
		{"timestamptz", "date"}, {"date", "timestamptz"},
		// Postgres has no cast at all for these
		{"numeric(21,9)", "boolean"}, {"date", "time"},
	}
	for _, p := range destructive {
		if safeWidening[p] {
			t.Errorf("%s → %s must be declared", p[0], p[1])
		}
	}
}

// fieldIndex is the seam that lets a renamed column be compared against its own
// old definition. Written in terms of the current name it must agree with
// wantedIndexes, or the two would disagree about an index that did not change.
func TestFieldIndexAgreesWithWantedIndexes(t *testing.T) {
	d := &meta.DocType{Name: "Thing", Fields: []*meta.Field{
		{Fieldname: "code", Fieldtype: "Data", Unique: true},
		{Fieldname: "owner_ref", Fieldtype: "Link", Options: "User"},
		{Fieldname: "tag", Fieldtype: "Data", SearchIndex: true},
		{Fieldname: "plain", Fieldtype: "Data"},
	}}
	wanted := wantedIndexes(d)
	for _, f := range d.Fields {
		got, ok := fieldIndex(f, f.Fieldname)
		w, inWanted := wanted["tab_thing_"+f.Fieldname]
		if ok != inWanted {
			t.Fatalf("%s: fieldIndex says %v, wantedIndexes says %v", f.Fieldname, ok, inWanted)
		}
		if ok && (got.cols != w.cols || got.unique != w.unique || got.predicate != w.predicate) {
			t.Fatalf("%s: %+v vs %+v", f.Fieldname, got, w)
		}
	}
	// Written in terms of the old name, it matches what Postgres still reports
	// for an index that was only renamed.
	old, _ := fieldIndex(d.Fields[0], "codigo")
	if !strings.Contains(old.cols, "codigo") || !strings.Contains(old.predicate, "codigo") {
		t.Fatalf("the old-name form should name the old column: %+v", old)
	}
}

func TestReportSeparatesTheDestructive(t *testing.T) {
	out := Report([]Statement{
		{SQL: `ALTER TABLE "tab_a" RENAME COLUMN "x" TO "y";`, Kind: KindRenameColumn, Table: "tab_a", Column: "y", OldName: "x"},
		{SQL: `ALTER TABLE "tab_a" DROP COLUMN "z";`, Kind: KindDropColumn, Table: "tab_a", Column: "z", Destructive: true},
	})
	if !strings.Contains(out, "expand") || !strings.Contains(out, "contract") {
		t.Fatalf("both phases should be headed:\n%s", out)
	}
	if strings.Index(out, "rename") > strings.Index(out, "drop ") {
		t.Fatalf("the drop must come last:\n%s", out)
	}
	if got := Report(nil); !strings.Contains(got, "nothing to do") {
		t.Fatalf("empty plan: %q", got)
	}
}

// DAT-05 — a compound key is one partial unique index. The predicate is the
// part worth pinning: it has to name every component, and it has to stay
// column-type aware, because `<> ”` on a numeric or date column is the very
// thing that broke migration in B14.
func TestUniqueKeyIndex(t *testing.T) {
	d := &meta.DocType{Name: "Invoice", UniqueKeys: []meta.UniqueKey{
		{Name: "customer_period", Fields: []string{"customer", "period", "total"}},
	}, Fields: []*meta.Field{
		{Fieldname: "customer", Fieldtype: "Link", Options: "Customer"},
		{Fieldname: "period", Fieldtype: "Date"},
		{Fieldname: "total", Fieldtype: "Currency"},
	}}
	idx := wantedIndexes(d)
	got, ok := idx["tab_invoice_uk_customer_period"]
	if !ok {
		t.Fatalf("the key produced no index: %v", idx)
	}
	if !got.unique {
		t.Fatal("a business key that is not unique enforces nothing")
	}
	if want := `"customer", "period", "total"`; got.cols != want {
		t.Fatalf("cols = %q, want %q", got.cols, want)
	}
	want := `"customer" IS NOT NULL AND "customer" <> '' AND "period" IS NOT NULL AND "total" IS NOT NULL`
	if got.predicate != want {
		t.Fatalf("predicate = %q, want %q", got.predicate, want)
	}
	// The whole point of the predicate: Postgres has to report it back the way
	// it was written, or every migrate would rebuild the index.
	if !strings.Contains(got.ddl(), "CREATE UNIQUE INDEX") {
		t.Fatalf("ddl = %q", got.ddl())
	}
}

// The index name is the key's name, not the field list: that is what lets a
// field be reordered or renamed without the index being dropped and rebuilt.
func TestUniqueKeyIndexIsNamedAfterTheKey(t *testing.T) {
	d := &meta.DocType{Name: "Invoice", UniqueKeys: []meta.UniqueKey{
		{Name: "business_key", Fields: []string{"b", "a"}},
	}, Fields: []*meta.Field{
		{Fieldname: "a", Fieldtype: "Data"},
		{Fieldname: "b", Fieldtype: "Data"},
	}}
	idx := wantedIndexes(d)
	if _, ok := idx["tab_invoice_uk_business_key"]; !ok {
		t.Fatalf("wanted tab_invoice_uk_business_key, got %v", keysOf(idx))
	}
	// Declaration order is the index's column order, and nothing sorts it.
	if got := idx["tab_invoice_uk_business_key"].cols; got != `"b", "a"` {
		t.Fatalf("cols = %q, want the declared order", got)
	}
}

// uniqueKeyIndex renders against whichever column name it is given, which is
// how a renamed component is compared against the definition the catalog still
// holds instead of costing a rebuild.
func TestUniqueKeyIndexRendersAgainstTheOldColumn(t *testing.T) {
	d := &meta.DocType{Name: "Invoice", Fields: []*meta.Field{
		{Fieldname: "customer", Fieldtype: "Data"},
		{Fieldname: "invoice_no", Fieldtype: "Data"},
	}}
	k := meta.UniqueKey{Name: "k", Fields: []string{"customer", "invoice_no"}}
	got, ok := uniqueKeyIndex(d, k, func(fn string) string {
		if fn == "invoice_no" {
			return "numero"
		}
		return fn
	})
	if !ok {
		t.Fatal("the key produced no index")
	}
	if want := `"customer", "numero"`; got.cols != want {
		t.Fatalf("cols = %q, want %q", got.cols, want)
	}
	if !strings.Contains(got.predicate, `"numero" IS NOT NULL`) {
		t.Fatalf("the predicate has to follow the column too: %q", got.predicate)
	}
}

func keysOf(idx map[string]index) []string {
	out := make([]string, 0, len(idx))
	for k := range idx {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Idempotence rests entirely on this: what we write and what Postgres reads
// back have to normalise to the same string. Postgres parenthesises each
// conjunct and spells a text literal `”::text`, and normalizeSQL strips both —
// but it does not reorder, so a predicate built in any other order than the
// declared one would be rebuilt on every single migrate. Cheaper to pin here
// than to discover as a migration that never settles.
func TestSameIndexAcceptsPostgresRenderingOfACompositePredicate(t *testing.T) {
	d := &meta.DocType{Name: "Pessoa", UniqueKeys: []meta.UniqueKey{
		{Name: "doc", Fields: []string{"tipo", "documento", "limite"}},
	}, Fields: []*meta.Field{
		{Fieldname: "tipo", Fieldtype: "Select", Options: []any{"PF", "PJ"}},
		{Fieldname: "documento", Fieldtype: "Data"},
		{Fieldname: "limite", Fieldtype: "Currency"},
	}}
	want := wantedIndexes(d)["tab_pessoa_uk_doc"]
	// Copied from the shape pg_indexes reports, parens and cast included.
	indexdef := "CREATE UNIQUE INDEX tab_pessoa_uk_doc ON public.tab_pessoa USING btree " +
		"(tipo, documento, limite) WHERE ((tipo IS NOT NULL) AND (tipo <> ''::text) AND " +
		"(documento IS NOT NULL) AND (documento <> ''::text) AND (limite IS NOT NULL))"
	if !sameIndex(indexdef, want) {
		t.Fatalf("migrate would rebuild this index forever:\n  postgres: %s\n  wanted:   %s WHERE %s",
			indexdef, want.cols, want.predicate)
	}
	// And the negative: a key that really did change must not compare equal.
	changed := want
	changed.cols = `"tipo", "documento"`
	if sameIndex(indexdef, changed) {
		t.Fatal("a changed column list compared equal, so the index would never be rebuilt")
	}
}

func TestCreateTableOmitsVaultField(t *testing.T) {
	d := &meta.DocType{
		Name: "Conta",
		Fields: []*meta.Field{
			{Fieldname: "titulo", Fieldtype: "Data"},
			{Fieldname: "token", Fieldtype: "Vault"},
		},
	}
	sql := createTable(d)
	if strings.Contains(sql, "token") {
		t.Fatalf("CreateTableSQL should not contain Vault field:\n%s", sql)
	}
	if !strings.Contains(sql, "titulo") {
		t.Fatalf("CreateTableSQL should contain Data field:\n%s", sql)
	}
}

// meta.StdColumns names the standard columns and stdColumns types them. Nothing
// else ties the two: were one changed alone, Plan would add the other's column
// to every table without a word.
func TestStdColumnsAgreeWithMeta(t *testing.T) {
	cols := stdColumns(&meta.DocType{})
	if len(cols) != len(meta.StdColumns) {
		t.Fatalf("%d typed columns, %d named", len(cols), len(meta.StdColumns))
	}
	for i, c := range cols {
		if c.name != meta.StdColumns[i] {
			t.Errorf("column %d: typed %q, named %q", i, c.name, meta.StdColumns[i])
		}
	}
}
