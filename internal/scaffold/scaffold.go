// Package scaffold writes new apps and DocTypes from templates.
package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// App creates the skeleton of an app. ddcoreRange is the `ddcore` range of
// releases it is written against; empty leaves the line commented out, for a
// binary that cannot say which release it is.
func App(dir, name, title, ddcoreRange string) error {
	if title == "" {
		title = strings.ToUpper(name[:1]) + name[1:]
	}
	if !meta.ValidIdentAscii(name) {
		return fmt.Errorf("invalid app name %q: use lowercase letters, digits and _", name)
	}
	rangeLine := fmt.Sprintf("  ddcore: %q, // the ddcore releases this app is tested against", ddcoreRange)
	if ddcoreRange == "" {
		rangeLine = `  // ddcore: ">=0.15.0 <0.16.0", // the ddcore releases this app is tested against`
	}
	files := map[string]string{
		"ddcore.app.ts": fmt.Sprintf(`import { defineApp } from "@ddcore/sdk";

export default defineApp({
  name: %q,
  title: %q,
  version: "0.1.0",
%s
  roles: [],
  // docEvents: { "User": { validate(doc) {} } },
  // scheduler: { daily: ["%s.services.tasks.daily"] },
  desk: { include: [] },
});
`, name, title, rangeLine, name),
		"translations/pt-BR.csv": "",
		"services/.keep":         "",
	}
	for f, c := range files {
		if err := write(filepath.Join(dir, f), c); err != nil {
			return err
		}
	}
	return nil
}

// DoctypeSpec is what the MCP/CLI receives to scaffold a DocType.
type DoctypeSpec struct {
	Name        string           `json:"name"`
	Label       string           `json:"label,omitempty"`
	Module      string           `json:"module,omitempty"`
	Naming      *meta.Naming     `json:"naming,omitempty"`
	Submittable bool             `json:"submittable,omitempty"`
	IsChild     bool             `json:"isChild,omitempty"`
	TrackChanges bool            `json:"trackChanges,omitempty"`
	TitleField  string           `json:"titleField,omitempty"`
	Fields      []map[string]any `json:"fields"`
	Roles       []string         `json:"roles,omitempty"`
	WithController bool          `json:"withController,omitempty"`
	WithForm    bool             `json:"withForm,omitempty"`
	WithTest    bool             `json:"withTest,omitempty"`
}

func tsValue(v any) string {
	b, _ := json.MarshalIndent(v, "    ", "  ")
	s := string(b)
	// JSON is valid TS; unquote simple keys for readability
	return keyRe.ReplaceAllString(s, "$1$2:")
}

// Doctype writes the .doctype.ts (and optional controller/form/test files).
func Doctype(appDir, appName string, spec DoctypeSpec) ([]string, error) {
	if spec.Name == "" || len(spec.Fields) == 0 {
		return nil, fmt.Errorf("name and fields are required")
	}
	snake := meta.Snake(spec.Name)
	dir := filepath.Join(appDir, "doctypes", snake)
	def := map[string]any{"name": spec.Name}
	if spec.Label != "" {
		def["label"] = spec.Label
	}
	if spec.Module != "" {
		def["module"] = spec.Module
	}
	if spec.Naming != nil {
		def["naming"] = spec.Naming
	}
	if spec.Submittable {
		def["submittable"] = true
	}
	if spec.IsChild {
		def["isChild"] = true
	}
	if spec.TrackChanges {
		def["trackChanges"] = true
	}
	if spec.TitleField != "" {
		def["titleField"] = spec.TitleField
	}
	def["fields"] = spec.Fields
	if !spec.IsChild {
		roles := spec.Roles
		if len(roles) == 0 {
			roles = []string{"System Manager"}
		}
		var perms []map[string]any
		for _, r := range roles {
			p := map[string]any{"role": r, "read": true, "write": true, "create": true, "delete": true}
			if spec.Submittable {
				p["submit"], p["cancel"], p["amend"] = true, true, true
			}
			perms = append(perms, p)
		}
		def["permissions"] = perms
	}
	var written []string
	body := strings.TrimSpace(tsValue(def))
	body = strings.TrimSuffix(strings.TrimPrefix(body, "{"), "}")
	src := "import { defineDoctype } from \"@ddcore/sdk\";\n\nexport default defineDoctype({" + body + "\n});\n"
	p := filepath.Join(dir, snake+".doctype.ts")
	if err := write(p, src); err != nil {
		return nil, err
	}
	written = append(written, p)
	iface := strings.ReplaceAll(spec.Name, " ", "")
	if spec.WithController && !spec.IsChild {
		p := filepath.Join(dir, snake+".controller.ts")
		src := fmt.Sprintf(`import { defineController, _ } from "@ddcore/sdk";
import type { %s } from "../../.ddcore/types";

export default defineController<%s>(%q, {
  validate(doc, ctx) {
    // validation rules; ddcore.throw(_("message"), { title: _("Title") })
  },
  methods: {
    // example: called by the desk with frm.call("example", { x: 1 })
    // example(doc, args) { return { ok: true }; },
  },
});
`, iface, iface, spec.Name)
		if err := write(p, src); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
	if spec.WithForm && !spec.IsChild {
		p := filepath.Join(dir, snake+".form.ts")
		src := fmt.Sprintf(`import { defineForm } from "@ddcore/desk-sdk";

defineForm(%q, {
  refresh(frm) {
    // frm.addButton(__("Action"), () => frm.call("example"), __("Actions"));
  },
  onChange: {
    // fieldname(frm) {},
  },
});
`, spec.Name)
		if err := write(p, src); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
	if spec.WithTest && !spec.IsChild {
		p := filepath.Join(dir, snake+".test.ts")
		src := fmt.Sprintf(`import "@ddcore/sdk/test";

describe(%q, () => {
  it("creates a document", () => {
    const doc = ddcore.newDoc(%q, {});
    // populate required fields before inserting
    expect(doc.doctype).toBe(%q);
  });
});
`, spec.Name, spec.Name, spec.Name)
		if err := write(p, src); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
	return written, nil
}
