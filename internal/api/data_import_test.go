package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// dataImport posts a file to /api/data-import/<doctype>. csrf=false leaves the
// header a browser request carries out, to prove a cookie alone is not enough.
func (x *env) dataImport(auth, doctype, filename, content string, fields map[string]string, csrf bool, hdr ...string) resp {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write([]byte(content))
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", x.ts.URL+"/api/data-import/"+doctype, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if csrf {
		req.Header.Set("X-Requested-With", "test")
	}
	req.AddCookie(&http.Cookie{Name: "sid", Value: strings.TrimPrefix(auth, "sid:")})
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
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

func importResult(t *testing.T, r resp) engine.DataImportResult {
	t.Helper()
	if r.Status != 200 {
		t.Fatalf("data import: %d %s", r.Status, r.Raw)
	}
	var out struct {
		Data engine.DataImportResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(r.Raw), &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func TestDataImportEndpoint(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")
	ana := "sid:" + x.sid("ana@x.com")
	file := "Nome;Tipo\nJoão;PJ\nMaria;XX\n"
	count := func() int {
		var n int
		x.asAdmin(func(c *engine.Ctx) error {
			return c.Q().QueryRow(c.Ctx, "SELECT count(*) FROM tab_pessoa").Scan(&n)
		})
		return n
	}

	// Gestor may create Pessoa but holds no import permission
	x.expect(x.dataImport(ana, "Pessoa", "p.csv", file, nil, true), 403, "PermissionError")
	// a session cookie without the CSRF header is refused before anything runs
	if r := x.dataImport(admin, "Pessoa", "p.csv", file, map[string]string{"dry_run": "1"}, false); r.Status == 200 {
		t.Fatalf("a cookie-only POST must be refused: %s", r.Raw)
	}

	before := count()
	dry := importResult(t, x.dataImport(admin, "Pessoa", "p.csv", file, map[string]string{"dry_run": "1"}, true, "X-Lang", "en"))
	if !dry.DryRun || dry.Counts.Inserted != 1 || dry.Counts.Errors != 1 || count() != before {
		t.Fatalf("dry run: %+v", dry)
	}
	if dry.File.Format != "csv" || dry.File.Sep != ";" || dry.File.Name != "p.csv" || len(dry.File.SHA256) != 64 {
		t.Fatalf("file: %+v", dry.File)
	}
	if dry.Rows[1].Row != 3 || len(dry.Rows[1].Cells) != 2 || dry.Rows[1].Cells[1] != "XX" {
		t.Fatalf("error row: %+v", dry.Rows[1])
	}
	en := dry.Rows[1].Message

	pt := importResult(t, x.dataImport(admin, "Pessoa", "p.csv", file, map[string]string{"dry_run": "1"}, true, "X-Lang", "pt-BR"))
	if pt.Rows[1].Message == en || pt.Rows[1].Message == "" {
		t.Fatalf("row messages follow the request language: %q vs %q", pt.Rows[1].Message, en)
	}

	run := importResult(t, x.dataImport(admin, "Pessoa", "p.csv", file, nil, true))
	if run.DryRun || run.Counts.Inserted != 1 || count() != before+1 || run.Rows[0].ID != "João" {
		t.Fatalf("run: %+v", run)
	}

	// an override, sent as JSON
	cols, _ := json.Marshal(map[string]string{"Full name": "nome"})
	over := importResult(t, x.dataImport(admin, "Pessoa", "p.csv", "Full name\nPaula\n", map[string]string{"dry_run": "1", "columns": string(cols)}, true))
	if over.Counts.Inserted != 1 {
		t.Fatalf("override: %+v", over)
	}

	big := strings.Repeat("x", maxImportBytes+10)
	x.expect(x.dataImport(admin, "Pessoa", "big.csv", big, nil, true), 417, "ValidationError")
	x.expect(x.dataImport(admin, "Pessoa", "p.csv", file, map[string]string{"sep": "ab"}, true), 417, "ValidationError")
	x.expect(x.dataImport(admin, "Pessoa", "p.csv", file, map[string]string{"columns": "{"}, true), 417, "ValidationError")
}

func TestDataImportTemplateEndpoint(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")
	r := x.call("GET", "/api/data-import/Pessoa/template?sep=%3B", nil, admin, "X-Lang", "en")
	if r.Status != 200 {
		t.Fatalf("%d %s", r.Status, r.Raw)
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatal(ct)
	}
	if cd := r.Header.Get("Content-Disposition"); !strings.Contains(cd, `Pessoa-import-template.csv`) {
		t.Fatal(cd)
	}
	// Pessoa takes its id from `nome`, so there is no id column; the child
	// table and the attachment are not columns of an import either
	if want := "\xef\xbb\xbfNome;Tipo;Bio\r\n"; r.Raw != want {
		t.Fatalf("template: %q, want %q", r.Raw, want)
	}
	x.expect(x.call("GET", "/api/data-import/Pessoa/template", nil, "sid:"+x.sid("ana@x.com")), 403, "PermissionError")
	x.expect(x.call("GET", "/api/data-import/Pessoa/template", nil, ""), 401, "")
}
