package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const (
	sec01AlfaUser = "user_alfa@x.com"
	sec01BetaUser = "user_beta@x.com"
)

func sec01APIApp(t *testing.T) string {
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
export default defineApp({ name: "demo", title: "Scope API Test", roles: ["Gestor", "Scope User"] });`)
	write("doctypes/test_company/test_company.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Company", naming: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Scope User", read: true, create: true }] });`)
	write("doctypes/test_record/test_record.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Record", naming: { field: "title" }, trackChanges: true,
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "company", fieldtype: "Link", label: "Company", options: "Test Company" },
  ],
  permissions: [{ role: "Scope User", read: true, create: true, write: true, delete: true, export: true }] });`)
	return dir
}

func setupSEC01API(t *testing.T) (*env, string, string) {
	t.Helper()
	x := setupApp(t, sec01APIApp(t))
	var alfaRecord, betaRecord string
	x.asAdmin(func(c *engine.Ctx) error {
		for _, user := range []string{sec01AlfaUser, sec01BetaUser} {
			doc, err := c.NewDoc("User", engine.Doc{
				"email": user, "full_name": user, "new_password": "segredo123",
				"roles": []any{map[string]any{"role": "Scope User"}},
			})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Company", engine.Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		for _, tc := range []struct{ title, company string }{{"Alfa Record", "Alfa"}, {"Beta Record", "Beta"}} {
			doc, err := c.NewDoc("Test Record", engine.Doc{"title": tc.title, "company": tc.company})
			if err != nil {
				return err
			}
			saved, err := c.Insert(doc, engine.SaveOpts{})
			if err != nil {
				return err
			}
			if tc.company == "Alfa" {
				alfaRecord = saved.Name()
			} else {
				betaRecord = saved.Name()
				saved["title"] = "Beta Record Revised"
				if _, err := c.Save(saved, engine.SaveOpts{}); err != nil {
					return err
				}
			}
		}
		for _, user := range []string{sec01AlfaUser, sec01BetaUser} {
			company := "Alfa"
			if user == sec01BetaUser {
				company = "Beta"
			}
			doc, err := c.NewDoc("User Permission", engine.Doc{"user": user, "allow": "Test Company", "for_value": company})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	return x, alfaRecord, betaRecord
}

func (x *env) uploadAttachment(auth, doctype, name, filename, content string) resp {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("doctype", doctype)
	_ = mw.WriteField("docname", name)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		x.t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		x.t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		x.t.Fatal(err)
	}
	req, err := http.NewRequest("POST", x.ts.URL+"/api/upload", &buf)
	if err != nil {
		x.t.Fatal(err)
	}
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

func TestSEC01_CrossCuttingChannels(t *testing.T) {
	x, alfaRecord, betaRecord := setupSEC01API(t)
	alfa := "sid:" + x.sid(sec01AlfaUser)
	admin := "sid:" + x.sid("Administrator")
	upload := x.uploadAttachment(alfa, "Test Record", betaRecord, "sec01-beta.txt", "beta")
	if upload.Status != 200 {
		t.Fatalf("upload Beta attachment: %d %s", upload.Status, upload.Raw)
	}
	fileURL := fmt.Sprint(upload.Body["data"].(map[string]any)["file_url"])

	if r := x.call("GET", fileURL, nil, alfa); r.Status != 403 || r.errType() != "PermissionError" {
		t.Errorf("scoped user downloaded Beta attachment: %d %s", r.Status, r.Raw)
	}
	if r := x.call("GET", fileURL, nil, admin); r.Status != 200 || r.Raw != "beta" {
		t.Errorf("Administrator lost attachment access: %d %s", r.Status, r.Raw)
	}

	if r := x.call("GET", "/api/versions/Test%20Record/"+betaRecord, nil, alfa); r.Status != 403 || r.errType() != "PermissionError" {
		t.Errorf("scoped user read Beta version history: %d %s", r.Status, r.Raw)
	}
	if r := x.call("GET", "/api/versions/Test%20Record/"+betaRecord, nil, admin); r.Status != 200 {
		t.Errorf("Administrator lost version access: %d %s", r.Status, r.Raw)
	}

	authorize := x.s.eventAuthorizer(context.Background(), sec01AlfaUser)
	if authorize("Test Record", betaRecord) {
		t.Error("scoped user authorized for Beta document event")
	}
	ch := x.e.Events.Subscribe(sec01AlfaUser, authorize)
	defer x.e.Events.Unsubscribe(ch)
	x.e.Events.Publish(engine.Event{Name: "doc_update", Doctype: "Test Record", DocName: betaRecord})
	select {
	case ev := <-ch:
		t.Errorf("scoped user received Beta document event: %+v", ev)
	default:
	}

	r := x.call("GET", "/api/export/Test%20Record", nil, alfa)
	if r.Status != 200 {
		t.Errorf("scoped export failed: %d %s", r.Status, r.Raw)
	} else if r.Header.Get("X-DDCore-Export-Count") != "1" || !strings.Contains(r.Raw, alfaRecord) || strings.Contains(r.Raw, betaRecord) {
		t.Errorf("scoped export leaked Beta records: count=%q body=%s", r.Header.Get("X-DDCore-Export-Count"), r.Raw)
	}

	var permission string
	x.asAdmin(func(c *engine.Ctx) error {
		doc, err := c.NewDoc("User Permission", engine.Doc{"user": sec01AlfaUser, "allow": "Test Company", "for_value": "Alfa", "applicable_for": "Test Record"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, engine.SaveOpts{})
		if err != nil {
			return err
		}
		permission = saved.Name()
		return c.Delete("User Permission", permission, false, false)
	})
	for _, action := range []string{"permission.scope_grant", "permission.scope_revoke"} {
		events, err := x.e.ListAuditEvents(x.ctx, engine.AuditFilter{Action: action, TargetDocType: "User", TargetName: sec01AlfaUser})
		if err != nil {
			t.Errorf("list %s audit events: %v", action, err)
			continue
		}
		if len(events) == 0 {
			t.Errorf("missing %s audit event for %s", action, fmt.Sprint(sec01AlfaUser))
		}
	}
}
