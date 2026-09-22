package api

import (
	"bytes"
	"context"
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

const (
	sec01AlfaUser    = "user_alfa@x.com"
	sec01BetaUser    = "user_beta@x.com"
	sec01ManagerUser = "scoped_manager@x.com"
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
export default defineDoctype({ name: "Test Company", idGeneration: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Scope User", read: true, create: true }] });`)
	write("doctypes/test_record/test_record.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Record", idGeneration: { field: "title" }, trackChanges: true,
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
		for _, user := range []struct{ email, role string }{
			{sec01AlfaUser, "Scope User"},
			{sec01BetaUser, "Scope User"},
			{sec01ManagerUser, "System Manager"},
		} {
			doc, err := c.NewDoc("User", engine.Doc{
				"email": user.email, "full_name": user.email, "new_password": "segredo123",
				"roles": []any{map[string]any{"role": user.role}},
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
				alfaRecord = saved.ID()
			} else {
				betaRecord = saved.ID()
				saved["title"] = "Beta Record Revised"
				if _, err := c.Save(saved, engine.SaveOpts{}); err != nil {
					return err
				}
			}
		}
		comment, err := c.NewDoc("Comment", engine.Doc{"reference_doctype": "Test Record", "reference_id": betaRecord, "content": "Beta comment"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(comment, engine.SaveOpts{}); err != nil {
			return err
		}
		for _, permission := range []struct{ user, company string }{
			{sec01AlfaUser, "Alfa"},
			{sec01BetaUser, "Beta"},
			{sec01ManagerUser, "Alfa"},
		} {
			doc, err := c.NewDoc("User Permission", engine.Doc{"user": permission.user, "allow": "Test Company", "for_value": permission.company})
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
	_ = mw.WriteField("doc_id", name)
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
	admin := "sid:" + x.sid("Admin")
	beta := "sid:" + x.sid(sec01BetaUser)
	upload := x.uploadAttachment(beta, "Test Record", betaRecord, "sec01-beta.txt", "beta")
	if upload.Status != 200 {
		t.Fatalf("upload Beta attachment: %d %s", upload.Status, upload.Raw)
	}
	fileURL := fmt.Sprint(upload.Body["data"].(map[string]any)["file_url"])

	if r := x.call("GET", fileURL, nil, alfa); r.Status != 403 || r.errType() != "PermissionError" {
		t.Errorf("scoped user downloaded Beta attachment: %d %s", r.Status, r.Raw)
	}
	if r := x.call("GET", fileURL, nil, admin); r.Status != 200 || r.Raw != "beta" {
		t.Errorf("Admin lost attachment access: %d %s", r.Status, r.Raw)
	}

	if r := x.call("GET", "/api/versions/Test%20Record/"+betaRecord, nil, alfa); r.Status != 403 || r.errType() != "PermissionError" {
		t.Errorf("scoped user read Beta version history: %d %s", r.Status, r.Raw)
	}
	if r := x.call("GET", "/api/versions/Test%20Record/"+betaRecord, nil, admin); r.Status != 200 {
		t.Errorf("Admin lost version access: %d %s", r.Status, r.Raw)
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
		permission = saved.ID()
		return c.Delete("User Permission", permission, false, false)
	})
	for _, action := range []string{"permission.scope_grant", "permission.scope_revoke"} {
		events, err := x.e.ListAuditEvents(x.ctx, engine.AuditFilter{Action: action, TargetDocType: "User", TargetID: sec01AlfaUser})
		if err != nil {
			t.Errorf("list %s audit events: %v", action, err)
			continue
		}
		if len(events) == 0 {
			t.Errorf("missing %s audit event for %s", action, fmt.Sprint(sec01AlfaUser))
		}
	}
}

func TestSEC01_ProductionNotificationsRespectScopeChanges(t *testing.T) {
	x, _, betaRecord := setupSEC01API(t)
	authorize := x.s.eventAuthorizer(context.Background(), sec01AlfaUser)
	if authorize("Test Record", betaRecord) {
		t.Fatal("Alfa scope authorized a Beta document before a grant")
	}
	ch := x.e.Events.Subscribe(sec01AlfaUser, authorize)
	defer x.e.Events.Unsubscribe(ch)
	betaAuthorize := x.s.eventAuthorizer(context.Background(), sec01BetaUser)
	betaCh := x.e.Events.Subscribe(sec01BetaUser, betaAuthorize)
	defer x.e.Events.Unsubscribe(betaCh)

	var permission string
	x.asAdmin(func(c *engine.Ctx) error {
		doc, err := c.NewDoc("User Permission", engine.Doc{"user": sec01AlfaUser, "allow": "Test Company", "for_value": "Beta"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, engine.SaveOpts{})
		permission = saved.ID()
		return err
	})
	if !authorize("Test Record", betaRecord) {
		t.Error("grant did not invalidate the cached Beta SSE denial")
	}

	x.asAdmin(func(c *engine.Ctx) error {
		doc, err := c.GetDoc("User Permission", permission)
		if err != nil {
			return err
		}
		doc["for_value"] = "Alfa"
		_, err = c.Save(doc, engine.SaveOpts{})
		return err
	})
	if authorize("Test Record", betaRecord) {
		t.Error("Save did not invalidate the cached Beta SSE grant")
	}

	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.DBSet("User Permission", permission, engine.Doc{"for_value": "Beta"}, true)
		return err
	})
	if !authorize("Test Record", betaRecord) {
		t.Error("DBSet did not invalidate the cached Beta SSE denial")
	}

	x.asAdmin(func(c *engine.Ctx) error {
		return c.Delete("User Permission", permission, false, false)
	})
	if authorize("Test Record", betaRecord) {
		t.Error("revoke did not invalidate the cached Beta SSE grant")
	}
	for {
		select {
		case <-ch:
		default:
			goto drained
		}
	}

drained:

	x.asAdmin(func(c *engine.Ctx) error {
		doc, err := c.GetDoc("Test Record", betaRecord)
		if err != nil {
			return err
		}
		doc["title"] = "Beta notification"
		_, err = c.Save(doc, engine.SaveOpts{})
		return err
	})
	for {
		select {
		case event := <-ch:
			if event.Name == "doc_update" && event.Payload.(map[string]any)["id"] == betaRecord {
				t.Errorf("production notification leaked Beta document: %+v", event)
			}
		default:
			goto alfaDrained
		}
	}

alfaDrained:
	received := false
	for {
		select {
		case event := <-betaCh:
			if event.Name == "doc_update" && event.Doctype == "Test Record" && event.DocID == betaRecord {
				received = true
			}
		default:
			if !received {
				t.Error("authorized Beta user did not receive the production document notification")
			}
			return
		}
	}
}

func TestSEC01_ScopedSystemManagerCannotListReferencedHistory(t *testing.T) {
	x, _, betaRecord := setupSEC01API(t)
	manager := "sid:" + x.sid(sec01ManagerUser)
	for _, tc := range []struct {
		doctype string
		filters string
	}{
		{"Version", fmt.Sprintf(`{"ref_doctype":"Test Record","doc_id":"%s"}`, betaRecord)},
		{"Comment", fmt.Sprintf(`{"reference_doctype":"Test Record","reference_id":"%s"}`, betaRecord)},
	} {
		path := "/api/resource/" + tc.doctype + "?filters=" + url.QueryEscape(tc.filters)
		if r := x.call("GET", path, nil, manager); r.Status != 403 || r.errType() != "PermissionError" {
			t.Errorf("scoped System Manager listed Beta %s: %d %s", tc.doctype, r.Status, r.Raw)
		}
	}
}

func TestSEC01_UserPermissionScopeMutationsAreAudited(t *testing.T) {
	x, _, _ := setupSEC01API(t)
	count := func(action string) int64 {
		n, err := x.e.CountAuditEvents(x.ctx, engine.AuditFilter{Action: action, TargetDocType: "User", TargetID: sec01BetaUser})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	hasForValue := func(action, want string) bool {
		events, err := x.e.ListAuditEvents(x.ctx, engine.AuditFilter{Action: action, TargetDocType: "User", TargetID: sec01BetaUser, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if detail, ok := event["detail"].(map[string]any); ok && fmt.Sprint(detail["for_value"]) == want {
				return true
			}
		}
		return false
	}
	grants, revokes := count("permission.scope_grant"), count("permission.scope_revoke")
	var permission string
	x.asAdmin(func(c *engine.Ctx) error {
		doc, err := c.NewDoc("User Permission", engine.Doc{"user": sec01BetaUser, "allow": "Test Company", "for_value": "Beta"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, engine.SaveOpts{})
		permission = saved.ID()
		return err
	})
	grants++
	x.asAdmin(func(c *engine.Ctx) error {
		doc, err := c.GetDoc("User Permission", permission)
		if err != nil {
			return err
		}
		doc["for_value"] = "Alfa"
		_, err = c.Save(doc, engine.SaveOpts{})
		return err
	})
	if got := count("permission.scope_grant"); got != grants+1 {
		t.Errorf("Save grant audit count = %d, want %d", got, grants+1)
	}
	if got := count("permission.scope_revoke"); got != revokes+1 {
		t.Errorf("Save revoke audit count = %d, want %d", got, revokes+1)
	}

	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.DBSet("User Permission", permission, engine.Doc{"for_value": "Beta"}, true)
		return err
	})
	if got := count("permission.scope_grant"); got != grants+2 {
		t.Errorf("DBSet grant audit count = %d, want %d", got, grants+2)
	}
	if got := count("permission.scope_revoke"); got != revokes+2 {
		t.Errorf("DBSet revoke audit count = %d, want %d", got, revokes+2)
	}
	if !hasForValue("permission.scope_grant", "Alfa") {
		t.Error("Save did not record the granted Alfa scope")
	}
	if !hasForValue("permission.scope_revoke", "Alfa") {
		t.Error("DBSet did not record the revoked Alfa scope")
	}
}
