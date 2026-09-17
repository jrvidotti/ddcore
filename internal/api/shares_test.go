package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const (
	shareEditorUser = "share_editor@x.com"
	sharePlainUser  = "share_plain@x.com"
)

func sharesAPIApp(t *testing.T) string {
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
export default defineApp({ name: "demo", title: "Share API Test", roles: ["Gestor", "Note Editor"] });`)
	write("doctypes/shared_note/shared_note.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Shared Note", naming: { field: "title" }, trackChanges: true,
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "body", fieldtype: "Data", label: "Body" },
    { fieldname: "secret", fieldtype: "Data", label: "Secret", permlevel: 1 },
    { fieldname: "attachment", fieldtype: "Attach", label: "Attachment" },
  ],
  permissions: [
    { role: "Note Editor", read: true, write: true, create: true, delete: true, share: true },
    { role: "Note Editor", permlevel: 1, read: true, write: true },
  ] });`)
	return dir
}

func setupSharesAPI(t *testing.T) *env {
	t.Helper()
	x := setupApp(t, sharesAPIApp(t))
	x.asAdmin(func(c *engine.Ctx) error {
		for email, roles := range map[string][]any{
			shareEditorUser: {map[string]any{"role": "Note Editor"}},
			sharePlainUser:  {},
		} {
			doc, err := c.NewDoc("User", engine.Doc{"email": email, "full_name": email, "new_password": "segredo123", "roles": roles})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		for _, title := range []string{"N1", "N2"} {
			doc, err := c.NewDoc("Shared Note", engine.Doc{"title": title, "secret": "hidden"})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	return x
}

func TestShares_HTTPGrantEveryChannelThenRevoke(t *testing.T) {
	x := setupSharesAPI(t)
	editor, plain := "sid:"+x.sid(shareEditorUser), "sid:"+x.sid(sharePlainUser)
	upload := x.uploadAttachment(editor, "Shared Note", "N1", "note.txt", "note body")
	if upload.Status != 200 {
		t.Fatalf("upload: %d %s", upload.Status, upload.Raw)
	}
	fileURL := fmt.Sprint(upload.Body["data"].(map[string]any)["file_url"])
	authorize := x.s.eventAuthorizer(context.Background(), sharePlainUser)

	// every channel is closed before the share
	channels := map[string]string{
		"doc":      "/api/resource/Shared%20Note/N1",
		"list":     "/api/resource/Shared%20Note",
		"versions": "/api/versions/Shared%20Note/N1",
		"comments": "/api/comments/Shared%20Note/N1",
		"print":    "/api/print/Shared%20Note/N1",
		"file":     fileURL,
		"shares":   "/api/shares/Shared%20Note/N1",
	}
	closed := func(when string) {
		t.Helper()
		for name, path := range channels {
			if r := x.call("GET", path, nil, plain); r.Status != 403 {
				t.Errorf("%s: %s answered %d: %s", when, name, r.Status, r.Raw)
			}
		}
		if authorize("Shared Note", "N1") {
			t.Errorf("%s: the event authorizer let the document through", when)
		}
	}
	closed("before the share")

	r := x.call("POST", "/api/shares/add", map[string]any{"doctype": "Shared Note", "name": "N1", "user": sharePlainUser, "read": true}, editor)
	x.expect(r, 200, "")
	if d := r.Body["data"].(map[string]any); d["user"] != sharePlainUser || d["read"] != true || d["write"] != false {
		t.Fatalf("share = %v", d)
	}
	// unknown fields and a grant by someone without the share right are refused
	x.expect(x.call("POST", "/api/shares/add", map[string]any{"doctype": "Shared Note", "name": "N1", "user": shareEditorUser, "admin": true}, plain), 417, "")
	x.expect(x.call("POST", "/api/shares/add", map[string]any{"doctype": "Shared Note", "name": "N1", "user": shareEditorUser}, plain), 403, "PermissionError")

	for name, path := range channels {
		if r := x.call("GET", path, nil, plain); r.Status != 200 {
			t.Errorf("shared: %s answered %d: %s", name, r.Status, r.Raw)
		}
	}
	if !authorize("Shared Note", "N1") {
		t.Error("shared: the event authorizer kept the cached denial")
	}
	r = x.call("GET", "/api/resource/Shared%20Note/N1", nil, plain)
	if _, ok := r.Body["data"].(map[string]any)["secret"]; ok {
		t.Error("a share exposed a permlevel 1 field")
	}
	r = x.call("GET", "/api/resource/Shared%20Note", nil, plain)
	if rows := r.Body["data"].([]any); len(rows) != 1 {
		t.Errorf("shared list = %v", rows)
	}
	r = x.call("GET", "/api/shares/Shared%20Note/N1", nil, plain)
	if d := r.Body["data"].(map[string]any); d["canShare"] != false || len(d["shares"].([]any)) != 1 {
		t.Errorf("recipient's share list = %v", d)
	}
	x.expect(x.call("PUT", "/api/resource/Shared%20Note/N1", map[string]any{"body": "x"}, plain), 403, "PermissionError")
	r = x.call("GET", "/api/notifications", nil, plain)
	notes := r.Body["data"].(map[string]any)["data"].([]any)
	if len(notes) != 1 || !strings.Contains(fmt.Sprint(notes[0].(map[string]any)["title"]), "N1") {
		t.Errorf("notifications = %v", notes)
	}

	x.expect(x.call("POST", "/api/shares/remove", map[string]any{"doctype": "Shared Note", "name": "N1", "user": sharePlainUser}, editor), 200, "")
	closed("after the revoke")
	r = x.call("GET", "/api/notifications", nil, plain)
	if notes := r.Body["data"].(map[string]any)["data"].([]any); len(notes) != 0 {
		t.Errorf("notifications after revoke = %v", notes)
	}
	x.expect(x.call("POST", "/api/shares/remove", map[string]any{"doctype": "Shared Note", "name": "N1", "user": sharePlainUser}, editor), 404, "")

	for _, action := range []string{"permission.share_grant", "permission.share_revoke"} {
		n, err := x.e.CountAuditEvents(x.ctx, engine.AuditFilter{Action: action, TargetDocType: "Shared Note", TargetName: "N1", Outcome: "Allowed"})
		if err != nil || n != 1 {
			t.Errorf("%s audit events = %d, %v", action, n, err)
		}
	}
}
