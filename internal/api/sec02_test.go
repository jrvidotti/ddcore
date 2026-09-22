package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func sec02APIApp(t *testing.T) string {
	t.Helper()
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
export default defineApp({ name: "demo", title: "Field Permission API Test", roles: ["Gestor", "HR"] });`)
	write("doctypes/employee/employee.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Employee", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "department", fieldtype: "Data", label: "Department" },
    { fieldname: "salary", fieldtype: "Currency", label: "Salary", permlevel: 1 },
    { fieldname: "contract", fieldtype: "Attach", label: "Contract", permlevel: 1 },
  ],
  permissions: [
    { role: "Gestor", read: true, write: true, create: true },
    { role: "HR", read: true, write: true, create: true },
    { role: "HR", permlevel: 1, read: true, write: true },
  ] });`)
	write("doctypes/employee/employee.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Employee", {
  methods: { raise(doc) { return { seen: doc.salary }; } },
});`)
	return dir
}

func (x *env) uploadToField(auth, doctype, name, field, private string) resp {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("doctype", doctype)
	_ = mw.WriteField("doc_id", name)
	_ = mw.WriteField("fieldname", field)
	_ = mw.WriteField("is_private", private)
	fw, _ := mw.CreateFormFile("file", "contract.pdf")
	fw.Write([]byte("%PDF"))
	mw.Close()
	req, _ := http.NewRequest("POST", x.ts.URL+"/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "test")
	req.AddCookie(&http.Cookie{Name: "sid", Value: strings.TrimPrefix(auth, "sid:")})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw), Header: res.Header}
	json.Unmarshal(raw, &out.Body)
	return out
}

func TestSEC02_API(t *testing.T) {
	x := setupApp(t, sec02APIApp(t))
	x.asAdmin(func(c *engine.Ctx) error {
		u, _ := c.NewDoc("User", engine.Doc{"email": "hr@x.com", "full_name": "HR", "new_password": "segredo123",
			"roles": []any{map[string]any{"role": "HR"}}})
		if _, err := c.Insert(u, engine.SaveOpts{}); err != nil {
			return err
		}
		doc, _ := c.NewDoc("Employee", engine.Doc{"title": "Ana", "department": "Ops", "salary": 100})
		_, err := c.Insert(doc, engine.SaveOpts{})
		return err
	})
	ana := "sid:" + x.sid("ana@x.com")
	hr := "sid:" + x.sid("hr@x.com")

	t.Run("meta carries the user's field levels", func(t *testing.T) {
		r := x.call("GET", "/api/meta/Employee", nil, ana)
		x.expect(r, 200, "")
		levels := fmt.Sprint(r.Body["data"].(map[string]any)["fieldLevels"])
		if levels != "map[read:[0] write:[0]]" {
			t.Fatalf("ana fieldLevels = %s", levels)
		}
		r = x.call("GET", "/api/meta/Employee", nil, hr)
		if levels := fmt.Sprint(r.Body["data"].(map[string]any)["fieldLevels"]); levels != "map[read:[0 1] write:[0 1]]" {
			t.Fatalf("hr fieldLevels = %s", levels)
		}
	})

	t.Run("get and list omit the field", func(t *testing.T) {
		r := x.call("GET", "/api/resource/Employee/Ana", nil, ana)
		x.expect(r, 200, "")
		if _, ok := r.Body["data"].(map[string]any)["salary"]; ok {
			t.Fatalf("get leaked salary: %s", r.Raw)
		}
		r = x.call("GET", "/api/resource/Employee?fields="+url.QueryEscape(`["id","salary"]`), nil, ana)
		x.expect(r, 200, "")
		if strings.Contains(r.Raw, "salary") {
			t.Fatalf("list leaked salary: %s", r.Raw)
		}
		r = x.call("GET", "/api/resource/Employee?filters="+url.QueryEscape(`[["salary",">",50]]`), nil, ana)
		x.expect(r, 403, "PermissionError")
		r = x.call("GET", "/api/count/Employee?filters="+url.QueryEscape(`[["salary",">",50]]`), nil, ana)
		x.expect(r, 403, "PermissionError")
		r = x.call("GET", "/api/count/Employee?filters="+url.QueryEscape(`[["salary",">",50]]`), nil, hr)
		x.expect(r, 200, "")
	})

	t.Run("update round trip keeps the value; a change is refused", func(t *testing.T) {
		r := x.call("PUT", "/api/resource/Employee/Ana", map[string]any{"department": "Sales", "salary": nil}, ana)
		x.expect(r, 200, "")
		x.asAdmin(func(c *engine.Ctx) error {
			doc, _ := c.GetDoc("Employee", "Ana")
			if doc.Str("department") != "Sales" || fmt.Sprint(doc["salary"]) == "<nil>" {
				t.Fatalf("stored = %v", doc)
			}
			return nil
		})
		r = x.call("PUT", "/api/resource/Employee/Ana", map[string]any{"salary": 1}, ana)
		x.expect(r, 403, "PermissionError")
		r = x.call("PUT", "/api/resource/Employee/Ana", map[string]any{"salary": 120}, hr)
		x.expect(r, 200, "")
	})

	t.Run("controller method response is redacted", func(t *testing.T) {
		r := x.call("POST", "/api/resource/Employee/Ana/run_method", map[string]any{"method": "raise"}, ana)
		x.expect(r, 200, "")
		data := r.Body["data"].(map[string]any)
		if fmt.Sprint(data["result"].(map[string]any)["seen"]) != "120" {
			t.Fatalf("the method itself sees the whole document: %s", r.Raw)
		}
		if _, ok := data["doc"].(map[string]any)["salary"]; ok {
			t.Fatalf("method doc leaked salary: %s", r.Raw)
		}
	})

	t.Run("uploads into a restricted field", func(t *testing.T) {
		r := x.uploadToField(ana, "Employee", "Ana", "contract", "0")
		x.expect(r, 403, "PermissionError")
		r = x.uploadToField(hr, "Employee", "Ana", "contract", "0")
		x.expect(r, 200, "")
		fileURL := fmt.Sprint(r.Body["data"].(map[string]any)["file_url"])
		if !strings.HasPrefix(fileURL, "/private/") {
			t.Fatalf("a restricted attachment must be private, got %s", fileURL)
		}
		x.expect(x.call("GET", fileURL, nil, ana), 403, "PermissionError")
		if r := x.call("GET", fileURL, nil, hr); r.Status != 200 {
			t.Fatalf("hr download: %d %s", r.Status, r.Raw)
		}
	})
}
