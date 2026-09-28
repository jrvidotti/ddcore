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
// binary that cannot say which release it is. siteRange means the site's
// ddcore.json already declares the range for all its apps, so the app gets
// none of its own — a second copy is one more line to forget on an upgrade.
func App(dir, name, title, ddcoreRange string, siteRange bool) error {
	if title == "" {
		title = strings.ToUpper(name[:1]) + name[1:]
	}
	if !meta.ValidIdentAscii(name) {
		return fmt.Errorf("invalid app name %q: use lowercase letters, digits and _", name)
	}
	rangeLine := fmt.Sprintf("  ddcore: %q, // the ddcore releases this app is tested against", ddcoreRange)
	switch {
	case siteRange:
		rangeLine = `  // ddcore: the site's range in ddcore.json applies; declare one here only to narrow it`
	case ddcoreRange == "":
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
	Name           string             `json:"name"`
	Label          string             `json:"label,omitempty"`
	Module         string             `json:"module,omitempty"`
	IDGeneration   *meta.IDGeneration `json:"idGeneration,omitempty"`
	Submittable    bool               `json:"submittable,omitempty"`
	IsChild        bool               `json:"isChild,omitempty"`
	TrackChanges   bool               `json:"trackChanges,omitempty"`
	TitleField     string             `json:"titleField,omitempty"`
	Fields         []map[string]any   `json:"fields"`
	Roles          []string           `json:"roles,omitempty"`
	WithController bool               `json:"withController,omitempty"`
	WithForm       bool               `json:"withForm,omitempty"`
	WithTest       bool               `json:"withTest,omitempty"`
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
	if spec.IDGeneration != nil {
		def["idGeneration"] = spec.IDGeneration
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

// ExtensionSpec is what the MCP receives to scaffold an `extendDoctype` file.
// Its keys mirror ExtensionDef in the SDK, so what an agent sends is what the
// file will say.
type ExtensionSpec struct {
	Fields      []map[string]any          `json:"fields,omitempty"`
	Set         map[string]map[string]any `json:"set,omitempty"`
	Doctype     map[string]any            `json:"doctype,omitempty"`
	Permissions []map[string]any          `json:"permissions,omitempty"`
	WithForm    bool                      `json:"withForm,omitempty"`
}

// Extension writes extensions/<snake>.extend.ts (and the .form.ts beside it
// when asked). It only creates: an existing file may carry hasPermission or
// permissionQuery code that regenerating would lose. Whether the extension is
// allowed at all is not checked here — loading it is the check.
func Extension(appDir, doctype string, spec ExtensionSpec) ([]string, error) {
	if doctype == "" {
		return nil, fmt.Errorf("doctype is required")
	}
	if len(spec.Fields) == 0 && len(spec.Set) == 0 && len(spec.Doctype) == 0 && len(spec.Permissions) == 0 {
		return nil, fmt.Errorf("nothing to extend: give fields, set, doctype or permissions")
	}
	snake := meta.Snake(doctype)
	dir := filepath.Join(appDir, "extensions")
	p := filepath.Join(dir, snake+".extend.ts")
	if _, err := os.Stat(p); err == nil {
		return nil, fmt.Errorf("%s already exists — edit it directly", p)
	}
	// a struct, not a map, so the sections come out in the order the docs use
	def := struct {
		Fields      []map[string]any          `json:"fields,omitempty"`
		Set         map[string]map[string]any `json:"set,omitempty"`
		Doctype     map[string]any            `json:"doctype,omitempty"`
		Permissions []map[string]any          `json:"permissions,omitempty"`
	}{spec.Fields, spec.Set, spec.Doctype, spec.Permissions}
	b, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return nil, err
	}
	body := keyRe.ReplaceAllString(string(b), "$1$2:")
	body = strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(body, "{"), "}"), "\n")
	src := fmt.Sprintf(`import { extendDoctype } from "@ddcore/sdk";

export default extendDoctype(%q, {%s,
  // hasPermission(doc, ptype, user) { return undefined; }, // any false denies
  // permissionQuery(user) { return undefined; },           // AND-ed with the host's filters
});
`, doctype, body)
	if err := write(p, src); err != nil {
		return nil, err
	}
	written := []string{p}
	if spec.WithForm {
		fp := filepath.Join(dir, snake+".form.ts")
		src := fmt.Sprintf(`import { defineForm } from "@ddcore/desk-sdk";

// runs alongside the owner's form script for %s
defineForm(%q, {
  refresh(frm) {},
  onChange: {
    // fieldname(frm) {},
  },
});
`, doctype, doctype)
		if err := write(fp, src); err != nil {
			os.Remove(p)
			return nil, err
		}
		written = append(written, fp)
	}
	return written, nil
}
