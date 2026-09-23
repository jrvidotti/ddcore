package i18nx

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

func texts(s *Set) []string {
	var out []string
	for _, k := range s.Keys() {
		out = append(out, k.Text)
	}
	return out
}

func collect(t *testing.T, src, name string, svelte bool) *Set {
	t.Helper()
	s := NewSet()
	collectSource(s, src, name, svelte)
	return s
}

// A regex over the source would collect from comments and from strings, and
// would miss a call split across lines. This is why the collector lexes.
func TestCollectTSIgnoresCommentsAndStrings(t *testing.T) {
	src := `
// __("in a line comment")
/* __("in a block comment") */
const s = '__("inside a string")';
const re = /__\("inside a regex"\)/;
const a = __("plain");
const b = __(
  "split across lines"
);
const c = x / 2; const d = y / 2;   // division, not a regex
const e = __("with \"escapes\" and \n newline");
`
	got := texts(collect(t, src, "x.ts", false))
	want := []string{"plain", "split across lines", "with \"escapes\" and \n newline"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCollectTSTemplateLiteralIsDynamic(t *testing.T) {
	s := collect(t, "const a = __(`hi ${name}`);\nconst b = __(`plain`);", "x.ts", false)
	if got := texts(s); !reflect.DeepEqual(got, []string{"plain"}) {
		t.Fatalf("got %q, want [plain]", got)
	}
	if len(s.Dynamic) != 1 {
		t.Fatalf("want one dynamic call, got %d", len(s.Dynamic))
	}
}

func TestCollectTSSkipsDeclarations(t *testing.T) {
	s := collect(t, "interface API {\n  __(s: string, args?: any[]): string;\n}\n", "x.ts", false)
	if s.Len() != 0 || len(s.Dynamic) != 0 {
		t.Fatalf("a signature is not a call site: %v %v", texts(s), s.Dynamic)
	}
}

// Markup prose is not JavaScript: an apostrophe in it must not open a string
// literal that swallows the rest of the file.
func TestCollectSvelte(t *testing.T) {
	src := `<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  const title = __("Sign in");
</script>

<p>Don't let this eat the file</p>
<h1>{__("Welcome")}</h1>
{#if user}<span>{__("Sign out")}</span>{/if}

<style>.a { background: url("nope"); }</style>
<p>{__("Last")}</p>
`
	got := texts(collect(t, src, "x.svelte", true))
	want := []string{"Last", "Sign in", "Sign out", "Welcome"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCollectGo(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("internal/engine/doc.go", `package engine
func f(c *Ctx, x string) error {
	if x == "" {
		return cerr.Validation("{0} is required", c.T(x))
	}
	_ = cerr.New("Custom", 400, "not a key: New takes the type first")
	_ = c.T("A literal through T")
	_ = cerr.Unavailable("No renderer for {0}", x)
	return cerr.Permission("No permission for {0}", x)
}`)
	// the skip list keeps deliberate English out of the catalogue
	write("internal/mcp/mcp.go", `package mcp
func f() error { return cerr.Internal("an MCP tool description") }`)
	write("cmd/ddcore/main.go", `package main
func f() error { return cerr.Internal("a CLI message") }`)
	write("internal/meta/meta.go", `package meta
type Registry struct{}
func (r *Registry) Validate() error { return cerr.Validation("a developer error") }
func (r *Registry) Other() error { return cerr.Validation("a user error") }`)
	// tests are not a source of keys
	write("internal/engine/doc_test.go", `package engine
func f() error { return cerr.Validation("from a test") }`)

	s := NewSet()
	for _, sub := range []string{"internal", "cmd"} {
		if err := CollectGo(s, filepath.Join(dir, sub), dir); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"A literal through T", "No permission for {0}", "No renderer for {0}", "a user error", "{0} is required"}
	if got := texts(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if len(s.Dynamic) != 0 {
		t.Fatalf("c.T(expr) is the normal shape, not a warning: %v", s.Dynamic)
	}
}

func TestCatalogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "translations", "pt-BR.csv")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("Save,Salvar\nGone,Sumiu\n"), 0o644)

	s := NewSet()
	s.Add("Save", "desk/src/a.svelte", 12)
	s.Add("Delete", "desk/src/a.svelte", 20)

	cat, err := ReadCatalog(path, "pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	if got := cat.Missing(s); len(got) != 1 || got[0].Text != "Delete" {
		t.Fatalf("Missing = %v", got)
	}
	if got := cat.Orphans(s); !reflect.DeepEqual(got, []string{"Gone"}) {
		t.Fatalf("Orphans = %v", got)
	}

	// without --prune an orphan survives: a rename must not throw away work
	if err := cat.Write(s, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	want := "Delete,,# desk/src/a.svelte:20\nSave,Salvar,# desk/src/a.svelte:12\nGone,Sumiu,# orphan: no longer in the code\n"
	if string(b) != want {
		t.Fatalf("wrote:\n%s\nwant:\n%s", b, want)
	}

	if err := cat.Write(s, true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	want = "Delete,,# desk/src/a.svelte:20\nSave,Salvar,# desk/src/a.svelte:12\n"
	if string(b) != want {
		t.Fatalf("with --prune wrote:\n%s\nwant:\n%s", b, want)
	}
}

// A DocType's idLabel heads its list's name column, so it is a key — of the
// app that owns the DocType's own text, and of no other.
func TestCollectDocTypeIDLabel(t *testing.T) {
	d := &meta.DocType{Name: "Contract", App: "crm", Label: "Contract", IDLabel: "Contract No."}
	s := NewSet()
	CollectDocType(s, d, "crm", "crm doctype")
	if got, want := texts(s), []string{"Contract", "Contract No."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	other := NewSet()
	CollectDocType(other, d, "billing", "billing doctype")
	if got := texts(other); len(got) != 0 {
		t.Fatalf("another app collected %v", got)
	}
}

// The desk renders a workflow's state and action names through __(), so they
// are keys of the catalogue of the app that defines the workflow.
func TestCollectWorkflow(t *testing.T) {
	s := NewSet()
	CollectWorkflow(s, js.Workflow{
		States:      []js.WorkflowState{{State: "Draft"}, {State: "Pending Approval"}},
		Transitions: []js.WorkflowTransition{{State: "Draft", Action: "Submit for Approval", NextState: "Pending Approval"}},
	}, "demo workflow")
	want := []string{"Draft", "Pending Approval", "Submit for Approval"}
	if got := texts(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The format list shows a print template's label through c.T, falling back to
// its name, so the label is a key of the app that declares the template.
func TestCollectPrintTemplate(t *testing.T) {
	s := NewSet()
	CollectPrintTemplate(s, engine.PrintTemplate{Name: "demo.receipt", Label: "Official Receipt"}, "demo print")
	CollectPrintTemplate(s, engine.PrintTemplate{Name: "demo.unlabeled"}, "demo print")
	want := []string{"Official Receipt"}
	if got := texts(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Only a Select's options are display text. A Code field's option is a
// language, a Rating's is a number of stars and a Duration's are display flags
// — collecting any of them would demand a translation for "sql" and then show
// the translated word where a language name belongs.
func TestCollectDocTypeLeavesNonSelectOptionsAlone(t *testing.T) {
	d := &meta.DocType{Name: "Note", App: "crm", Label: "Note", Fields: []*meta.Field{
		{Fieldname: "snippet", Fieldtype: "Code", Label: "Snippet", Options: "sql"},
		{Fieldname: "score", Fieldtype: "Rating", Label: "Score", Options: float64(5)},
		{Fieldname: "spent", Fieldtype: "Duration", Label: "Time spent", Options: []any{"hideDays"}},
		{Fieldname: "state", Fieldtype: "Select", Label: "State", Options: []any{"Open", "Closed"}},
	}}
	s := NewSet()
	CollectDocType(s, d, "crm", "crm doctype")
	want := []string{"Note", "Snippet", "Score", "Time spent", "State", "Open", "Closed"}
	got := texts(s)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A report with no label is shown under its name, which the server translates;
// with a label, the name is only an identifier.
func TestCollectReportNameWithoutLabel(t *testing.T) {
	s := NewSet()
	CollectReport(s, map[string]any{"name": "Logged Claims"}, "crm report")
	if got, want := texts(s), []string{"Logged Claims"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	labelled := NewSet()
	CollectReport(labelled, map[string]any{"name": "claims", "label": "Claims"}, "crm report")
	if got, want := texts(labelled), []string{"Claims"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
