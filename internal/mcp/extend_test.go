package mcp

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/scaffold"
)

// `shop` owns Product; `addons` may extend it, `stray` has not declared it.
func extendEngine(t *testing.T) *engine.Engine {
	t.Helper()
	app := func(files map[string]string) string {
		dir := t.TempDir()
		for rel, src := range files {
			os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
			os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
		}
		return dir
	}
	shop := app(map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "shop", title: "Shop", roles: ["Clerk"] });`,
		"doctypes/product/product.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Product", label: "Product",
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title" }],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`,
	})
	addons := app(map[string]string{"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "addons", title: "Addons", requires: ["shop"] });`})
	stray := app(map[string]string{"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "stray", title: "Stray" });`})
	e, err := engine.New(context.Background(), engine.Config{Test: true, Apps: []js.App{
		{Name: "shop", Dir: shop}, {Name: "addons", Dir: addons}, {Name: "stray", Dir: stray},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestExtendDoctypeWritesAndLoads(t *testing.T) {
	e := extendEngine(t)
	out, err := extendDoctype(e, extendInput{App: "addons", Doctype: "Product", Spec: scaffold.ExtensionSpec{
		Fields: []map[string]any{{"fieldname": "warranty", "fieldtype": "Int", "label": "Warranty", "insertAfter": "title"}},
		Set:    map[string]map[string]any{"title": {"reqd": true}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := e.Current().Meta.Get("Product")
	if f := d.Field("warranty"); f == nil || f.App != "addons" {
		t.Fatalf("warranty did not reach the meta: %+v", f)
	}
	if !d.Field("title").Reqd || !slices.Contains(d.ExtendedBy, "addons") {
		t.Fatalf("the override did not apply: extendedBy=%v", d.ExtendedBy)
	}
	next := strings.Join(out["next"].([]string), " | ")
	for _, want := range []string{"migrate", "i18n_extract", "addons's catalogue"} {
		if !strings.Contains(next, want) {
			t.Errorf("next %q lacks %q", next, want)
		}
	}
}

func TestExtendDoctypeRefusesBeforeWriting(t *testing.T) {
	e := extendEngine(t)
	spec := scaffold.ExtensionSpec{Set: map[string]map[string]any{"title": {"reqd": true}}}
	cases := []struct {
		in   extendInput
		want string
	}{
		{extendInput{App: "stray", Doctype: "Product", Spec: spec}, "requires"},
		{extendInput{App: "shop", Doctype: "Product", Spec: spec}, "owns"},
		{extendInput{App: "addons", Doctype: "Nope", Spec: spec}, "does not exist"},
		{extendInput{App: "nope", Doctype: "Product", Spec: spec}, "does not exist"},
	}
	for _, c := range cases {
		_, err := extendDoctype(e, c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s/%s: got %v, want %q", c.in.App, c.in.Doctype, err, c.want)
		}
		if dir := e.Current().App(c.in.App).Dir; dir != "" {
			if _, err := os.Stat(filepath.Join(dir, "extensions")); err == nil {
				t.Errorf("%s: a file was written before the refusal", c.in.App)
			}
		}
	}
}

func TestExtendDoctypeRemovesWhatDoesNotLoad(t *testing.T) {
	e := extendEngine(t)
	for name, spec := range map[string]scaffold.ExtensionSpec{
		"collision": {Fields: []map[string]any{{"fieldname": "title", "fieldtype": "Data"}}, WithForm: true},
		"identity":  {Set: map[string]map[string]any{"title": {"fieldtype": "Int"}}},
		"regrant":   {Permissions: []map[string]any{{"role": "Clerk", "read": true}}},
	} {
		_, err := extendDoctype(e, extendInput{App: "addons", Doctype: "Product", Spec: spec})
		if err == nil || !strings.Contains(err.Error(), "were removed") {
			t.Fatalf("%s: got %v", name, err)
		}
		entries, _ := os.ReadDir(filepath.Join(e.Current().App("addons").Dir, "extensions"))
		if len(entries) != 0 {
			t.Fatalf("%s: files left behind: %v", name, entries)
		}
		// the site still serves the last good meta
		if d, ok := e.Current().Meta.Get("Product"); !ok || d.Field("title").Fieldtype != "Data" {
			t.Fatalf("%s: the previous meta was lost", name)
		}
	}
}

// AddTool infers the input schema and panics on a type it cannot describe.
func TestExtendDoctypeRegisters(t *testing.T) {
	if New(extendEngine(t)) == nil {
		t.Fatal("no server")
	}
}
