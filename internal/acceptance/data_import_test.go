package acceptance

// Data Import end to end (DAT-01, the spreadsheet half): a Project Manager
// downloads the template, checks a file with a dry run, imports it, and
// loads the site's own export back as an update — over the HTTP surface the
// desk uses, against the testapp fixture.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func postImport(t *testing.T, srv *httptest.Server, tok, doctype, content string, fields map[string]string) engine.DataImportResult {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "rows.csv")
	fw.Write([]byte(content))
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/api/data-import/"+doctype, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("X-Lang", "en")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("POST data-import %s: %d %s", doctype, res.StatusCode, raw)
	}
	var out struct {
		Data engine.DataImportResult `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out.Data
}

func TestDataImportOverHTTP(t *testing.T) {
	e := setup(t, "dimp")
	srv, _ := server(t, e)
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		d, _ := c.NewDoc("User", engine.Doc{"email": "pm@x.com", "full_name": "PM"})
		d["roles"] = []any{map[string]any{"role": "Project Manager"}}
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := e.CreateAPIKey(ctx, "pm@x.com", "acceptance")
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("GET", srv.URL+"/api/data-import/Project/template", nil)
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("X-Lang", "en")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := strings.TrimSpace(strings.TrimPrefix(readAll(t, res), "\ufeff"))
	// the id comes from `code`; status and progress are derived, the cover is
	// an attachment and milestones a child table, so none of them is a column
	if tmpl != "Code,Title,Description,Assignee,Start date,End date,Budget,Overview" {
		t.Fatalf("template: %q", tmpl)
	}

	file := tmpl + "\r\n" +
		"P-100,Alpha,,pm@x.com,2026-01-10,,1.500,\r\n" +
		"P-101,Beta,,ghost@x.com,2026-01-10,,,\r\n" +
		"P-102,Gamma,,pm@x.com,10/02/2026,09/02/2026,,\r\n" +
		"P-103,Delta,,Admin,15/02/2026,,,\r\n"
	opts := map[string]string{"dry_run": "1", "decimal": ",", "date_order": "dmy"}
	before := countIn(t, e, "SELECT count(*) FROM tab_project")
	dry := postImport(t, srv, tok, "Project", file, opts)
	if dry.Counts.Inserted != 2 || dry.Counts.Errors != 2 || countIn(t, e, "SELECT count(*) FROM tab_project") != before {
		t.Fatalf("dry run: %+v", dry.Rows)
	}
	// an unknown assignee, and the controller's own date check
	if dry.Rows[1].Type != "LinkExistsError" || !strings.Contains(dry.Rows[2].Message, "end date") {
		t.Fatalf("dry run errors: %+v", dry.Rows)
	}

	delete(opts, "dry_run")
	run := postImport(t, srv, tok, "Project", file, opts)
	if run.Counts.Inserted != 2 || countIn(t, e, "SELECT count(*) FROM tab_project") != before+2 {
		t.Fatalf("run: %+v", run.Rows)
	}
	if countIn(t, e, "SELECT count(*) FROM tab_project WHERE id = 'P-100' AND budget = 1500 AND owner = 'pm@x.com' AND status = 'Planned'") != 1 {
		t.Fatal("P-100 not written through the document path")
	}
	if countIn(t, e, "SELECT count(*) FROM tab_audit_event WHERE action = 'data.import' AND actor = 'pm@x.com'") != 1 {
		t.Fatal("the import was not audited")
	}

	// a Task names its project by title, the way a person writes it
	tasks := postImport(t, srv, tok, "Task", "code,project,title,assignee,due_date,priority\r\nT-1,Alpha,First,pm@x.com,2026-03-01,High\r\n", nil)
	if tasks.Counts.Inserted != 1 || countIn(t, e, "SELECT count(*) FROM tab_task WHERE id = 'T-1' AND project = 'P-100'") != 1 {
		t.Fatalf("tasks: %+v", tasks.Rows)
	}

	// the site's own export, read back as an update, changes nothing and fails nothing
	res = getRaw(t, srv, tok, "/api/export/Project")
	export := readAll(t, res)
	back := postImport(t, srv, tok, "Project", export, map[string]string{"mode": "update"})
	if back.Counts.Errors != 0 || back.Counts.Updated != 2 {
		t.Fatalf("round trip: %+v", back.Rows)
	}
}
