package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A DocType name with an over-encoded letter ("Cr%65dential") reaches chi as
// the raw path segment, so the name the insert ran under was decoded while the
// name the response was redacted under was not; RedactDoc fails open on a
// lookup miss and the secret came back in the clear.
func TestInsertRedactsAPasswordUnderAnOverEncodedDocTypeName(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"] });`)
	write("doctypes/credential/credential.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Credential", idGeneration: { field: "label" },
  fields: [
    { fieldname: "label", fieldtype: "Data", label: "Label", reqd: true },
    { fieldname: "secret", fieldtype: "Password", label: "Secret" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }] });`)
	x := setupApp(t, dir)
	admin := "sid:" + x.sid("Admin")

	for _, path := range []string{"/api/resource/Credential", "/api/resource/Cr%65dential"} {
		label := "key-" + strings.NewReplacer("/", "-", "%", "").Replace(path)
		r := x.call("POST", path, map[string]any{"label": label, "secret": "hunter2-plain"}, admin)
		x.expect(r, 200, "")
		if strings.Contains(r.Raw, "hunter2-plain") {
			t.Fatalf("POST %s returned the password in the clear: %s", path, r.Raw)
		}
	}
}
