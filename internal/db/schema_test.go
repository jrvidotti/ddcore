package db

import (
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
