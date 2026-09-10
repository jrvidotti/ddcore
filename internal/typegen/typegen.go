// Package typegen writes .cerne/types.d.ts for an app: one interface per
// DocType so controllers, tests and form scripts get autocomplete.
package typegen

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrvidotti/cerne/internal/meta"
	desksdk "github.com/jrvidotti/cerne/packages/desk-sdk"
	"github.com/jrvidotti/cerne/packages/sdk"
)

func tsType(f *meta.Field) string {
	switch f.Fieldtype {
	case "Int", "Float", "Currency", "Percent":
		return "number | null"
	case "Check":
		return "boolean"
	case "Select":
		opts := f.SelectValues()
		if len(opts) == 0 {
			return "string | null"
		}
		q := make([]string, 0, len(opts))
		for _, o := range opts {
			if o == "" {
				continue
			}
			q = append(q, fmt.Sprintf("%q", o))
		}
		return strings.Join(q, " | ") + " | null"
	case "Table":
		return ifaceName(f.OptionsString()) + "[]"
	case "JSON":
		return "any"
	}
	return "string | null"
}

func ifaceName(doctype string) string {
	return strings.ReplaceAll(strings.ReplaceAll(doctype, " ", ""), "-", "")
}

// Generate renders the declarations for all doctypes (apps see everything).
func Generate(reg *meta.Registry) string {
	var b strings.Builder
	b.WriteString("// Gerado por `cerne types` — não edite.\n")
	b.WriteString("import type { BaseDoc, ChildDoc } from \"@cerne/sdk\";\n\n")
	names := reg.Names()
	for _, n := range names {
		d := reg.DocTypes[n]
		base := "BaseDoc"
		if d.IsChild {
			base = "ChildDoc"
		}
		fmt.Fprintf(&b, "/** %s (%s) */\nexport interface %s extends %s {\n  doctype: %q;\n", d.Label, d.App, ifaceName(n), base, n)
		for _, f := range d.Fields {
			if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
				continue
			}
			if f.Label != "" && f.Label != f.Fieldname {
				fmt.Fprintf(&b, "  /** %s */\n", f.Label)
			}
			fmt.Fprintf(&b, "  %s: %s;\n", f.Fieldname, tsType(f))
		}
		b.WriteString("}\n\n")
	}
	b.WriteString("export interface DocTypeMap {\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  %q: %s;\n", n, ifaceName(n))
	}
	b.WriteString("}\n\nexport type DocTypeName = keyof DocTypeMap;\n")
	sort.Strings(names)
	return b.String()
}

func materialize(embedded fs.FS, appDir, target string) error {
	return fs.WalkDir(embedded, "src", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("src", path)
		if err != nil {
			return err
		}
		out := filepath.Join(appDir, ".cerne", target, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		content, err := fs.ReadFile(embedded, path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, content, 0o644)
	})
}

// Write puts generated DocType declarations and the embedded SDK sources under
// <appDir>/.cerne, so typechecking never depends on a framework checkout.
func Write(appDir string, reg *meta.Registry) error {
	dir := filepath.Join(appDir, ".cerne")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "types.d.ts"), []byte(Generate(reg)), 0o644); err != nil {
		return err
	}
	if err := materialize(sdk.FS, appDir, "sdk"); err != nil {
		return err
	}
	if err := materialize(desksdk.FS, appDir, "desk-sdk"); err != nil {
		return err
	}
	tsconfig := filepath.Join(appDir, "tsconfig.json")
	if _, err := os.Stat(tsconfig); err != nil {
		cfg := fmt.Sprintf(`{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true,
    "baseUrl": ".",
    "paths": { "@cerne/sdk": [%q], "@cerne/sdk/test": [%q], "@cerne/desk-sdk": [%q] },
    "types": []
  },
  "include": ["**/*.ts", ".cerne/types.d.ts"],
  "exclude": ["node_modules"]
}
`, ".cerne/sdk/index.ts", ".cerne/sdk/test.ts", ".cerne/desk-sdk/index.ts")
		if err := os.WriteFile(tsconfig, []byte(cfg), 0o644); err != nil {
			return err
		}
	}
	return nil
}
