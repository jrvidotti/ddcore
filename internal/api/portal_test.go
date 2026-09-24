package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const (
	portalAna  = "p_ana@x.com" // Website User, member
	portalBeto = "p_beto@x.com"
	portalDesk = "p_desk@x.com" // System User with the desk role
)

func portalAPIApp(t *testing.T) string {
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
export default defineApp({ name: "demo", title: "Portal API Test", roles: ["Gestor", "Member", "Clerk"],
  portal: { include: ["client/masks.ts"] } });`)
	write("client/masks.ts", `console.log("portal-mask-marker");`)
	write("doctypes/member/member.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Member", titleField: "member_name",
  fields: [
    { fieldname: "member_name", fieldtype: "Data", label: "Name", reqd: true },
    { fieldname: "user", fieldtype: "Link", label: "User", options: "User" },
    { fieldname: "phone", fieldtype: "Data", label: "Phone" },
    { fieldname: "salary", fieldtype: "Currency", label: "Salary" },
  ],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`)
	write("doctypes/claim/claim.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Claim", titleField: "subject",
  fields: [
    { fieldname: "member", fieldtype: "Link", label: "Member", options: "Member", reqd: true },
    { fieldname: "subject", fieldtype: "Data", label: "Subject", reqd: true },
    { fieldname: "kind", fieldtype: "Link", label: "Kind", options: "Expense Kind" },
    { fieldname: "receipt", fieldtype: "Attach", label: "Receipt" },
    { fieldname: "internal_note", fieldtype: "Data", label: "Internal note" },
  ],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`)
	write("doctypes/expense_kind/expense_kind.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Expense Kind", idGeneration: { field: "kind_name" }, titleField: "kind_name",
  fields: [{ fieldname: "kind_name", fieldtype: "Data", label: "Kind", reqd: true }],
  permissions: [{ role: "Clerk", read: true, write: true, create: true }] });`)
	write("services/portal.ts", `import { whitelisted } from "@ddcore/sdk";
export const hello = whitelisted(() => ({ hello: ddcore.session.user }), { portal: true });
export const deskOnly = whitelisted(() => ({ ok: true }));
export const claimDefaults = whitelisted(() => ({ subject: "Expense", internal_note: "dropped" }), { portal: true });`)
	write("portal/members.portal.ts", `import { definePortal } from "@ddcore/sdk";
export default definePortal({
  name: "Members", title: "Member Portal", roles: ["Member"],
  identity: { doctype: "Member", userField: "user" },
  pages: [
    { name: "me", label: "My data", kind: "record", doctype: "Member", match: { id: "id" },
      fields: ["member_name", "phone"], editable: ["phone"], write: true },
    { name: "claims", label: "My claims", doctype: "Claim", match: { member: "id" },
      fields: ["subject", "kind", "receipt"], editable: ["subject", "kind", "receipt"], create: true,
      defaultsMethod: "demo.services.portal.claimDefaults" },
  ],
});`)
	return dir
}

type portalEnv struct {
	*env
	anaMember, betoMember, betoClaim string
}

func setupPortalAPI(t *testing.T) *portalEnv {
	t.Helper()
	x := &portalEnv{env: setupApp(t, portalAPIApp(t))}
	x.asAdmin(func(c *engine.Ctx) error {
		for _, u := range []struct {
			email, typ, role string
		}{{portalAna, "Website User", "Member"}, {portalBeto, "Website User", "Member"}, {portalDesk, "System User", "Clerk"}} {
			d, _ := c.NewDoc("User", engine.Doc{"email": u.email, "full_name": u.email, "new_password": "segredo123",
				"user_type": u.typ, "roles": []any{map[string]any{"role": u.role}}})
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		ins := func(doctype string, v engine.Doc) string {
			d, _ := c.NewDoc(doctype, v)
			d, err := c.Insert(d, engine.SaveOpts{})
			if err != nil {
				t.Fatal(err)
			}
			return d.ID()
		}
		ins("Expense Kind", engine.Doc{"kind_name": "Travel"})
		x.anaMember = ins("Member", engine.Doc{"member_name": "Ana", "user": portalAna, "salary": 5000})
		x.betoMember = ins("Member", engine.Doc{"member_name": "Beto", "user": portalBeto})
		x.betoClaim = ins("Claim", engine.Doc{"member": x.betoMember, "subject": "Beto's claim", "internal_note": "x"})
		return nil
	})
	return x
}

// uploadField posts a file the way the portal does: into a field of a
// document that may not exist yet.
func (x *env) uploadField(auth, doctype, docID, fieldname, filename, content string) resp {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write([]byte(content))
	if doctype != "" {
		mw.WriteField("doctype", doctype)
	}
	if docID != "" {
		mw.WriteField("doc_id", docID)
	}
	if fieldname != "" {
		mw.WriteField("fieldname", fieldname)
	}
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

func data(r resp) map[string]any {
	d, _ := r.Body["data"].(map[string]any)
	return d
}

func TestPortal_WebsiteUserIsConfined(t *testing.T) {
	x := setupPortalAPI(t)
	ana := "sid:" + x.sid(portalAna)
	for _, path := range []string{
		"/api/resource/Member", "/api/resource/Claim/" + x.betoClaim, "/api/meta/Claim", "/api/count/Claim",
		"/api/search/link?doctype=Member&txt=a", "/api/search/global?txt=a", "/api/notifications",
		"/api/method/demo.services.portal.deskOnly", "/api/method/core.services.api_keys.listMyAPIKeys",
	} {
		x.expect(x.call("GET", path, nil, ana), 403, "PermissionError")
	}
	x.expect(x.call("PUT", "/api/resource/Member/"+x.anaMember, map[string]any{"salary": 1}, ana), 403, "PermissionError")
	// what a portal needs stays open
	r := x.call("GET", "/api/method/demo.services.portal.hello", nil, ana)
	x.expect(r, 200, "")
	x.expect(x.call("GET", "/api/method/core.services.profile.getMyProfile", nil, ana), 200, "")
	boot := x.call("GET", "/api/boot", nil, ana)
	x.expect(boot, 200, "")
	b := data(boot)
	if dts, _ := b["doctypes"].(map[string]any); b["website"] != true || len(dts) != 0 || len(b["workspaces"].([]any)) != 0 {
		t.Fatalf("a Website User's boot carries desk data: %v", b)
	}
	if ps, _ := b["portals"].([]any); len(ps) != 1 {
		t.Fatalf("boot portals = %v", b["portals"])
	}
	// a desk user is not confined, and sees the portal in boot too
	desk := "sid:" + x.sid(portalDesk)
	x.expect(x.call("GET", "/api/resource/Claim", nil, desk), 200, "")
	x.expect(x.call("GET", "/api/method/demo.services.portal.deskOnly", nil, desk), 200, "")
	// Guest keeps its own rules
	x.expect(x.call("GET", "/api/boot", nil, ""), 200, "")
}

// portal.include ships an app's client scripts to the portal as their own
// bundle, and the boot names the apps that have one, the Website User's too.
func TestPortal_ClientIncludes(t *testing.T) {
	x := setupPortalAPI(t)
	r := x.call("GET", "/assets/apps/demo/portal.js", nil, "")
	if r.Status != 200 || !strings.Contains(r.Raw, "portal-mask-marker") {
		t.Fatalf("portal.js = %d %q", r.Status, r.Raw)
	}
	// the desk bundle is separate: this app declares no desk include
	if r := x.call("GET", "/assets/apps/demo/desk.js", nil, ""); r.Status != 200 || strings.TrimSpace(r.Raw) != "export {};" {
		t.Fatalf("desk.js = %d %q", r.Status, r.Raw)
	}
	for _, user := range []string{portalAna, portalDesk} {
		b := data(x.call("GET", "/api/boot", nil, "sid:"+x.sid(user)))
		if inc, _ := b["portalIncludes"].([]any); len(inc) != 1 || inc[0] != "demo" {
			t.Fatalf("%s: boot portalIncludes = %v", user, b["portalIncludes"])
		}
	}
	// a Website User still sees none of the desk's apps
	if apps, _ := data(x.call("GET", "/api/boot", nil, "sid:"+x.sid(portalAna)))["apps"].([]any); len(apps) != 0 {
		t.Fatalf("a Website User's boot lists apps: %v", apps)
	}
}

func TestPortal_LoginLandsInPortal(t *testing.T) {
	x := setupPortalAPI(t)
	r := x.call("POST", "/api/login", map[string]any{"usr": portalAna, "pwd": "segredo123"}, "")
	x.expect(r, 200, "")
	if data(r)["home"] != "/portal" {
		t.Fatalf("a Website User lands on %v", data(r)["home"])
	}
	r = x.call("POST", "/api/login", map[string]any{"usr": portalDesk, "pwd": "segredo123"}, "")
	if data(r)["home"] != "/app" {
		t.Fatalf("a System User lands on %v", data(r)["home"])
	}
}

func TestPortal_PagesRoundTrip(t *testing.T) {
	x := setupPortalAPI(t)
	ana := "sid:" + x.sid(portalAna)

	r := x.call("GET", "/api/portal", nil, ana)
	x.expect(r, 200, "")
	meta := x.call("GET", "/api/portal/members/claims", nil, ana)
	x.expect(meta, 200, "")
	if fields, _ := data(meta)["fields"].([]any); len(fields) != 3 {
		t.Fatalf("page fields = %v", data(meta)["fields"])
	}

	// the record page: her own member, projected — no salary, no user
	r = x.call("GET", "/api/portal/members/me/doc", nil, ana)
	x.expect(r, 200, "")
	me := data(r)
	if me["id"] != x.anaMember || me["member_name"] != "Ana" {
		t.Fatalf("record page = %v", me)
	}
	if _, ok := me["salary"]; ok {
		t.Fatalf("a field the page does not show leaked: %v", me)
	}
	// update: an editable field goes through, anything else is refused
	x.expect(x.call("PUT", "/api/portal/members/me/doc/"+x.anaMember, map[string]any{"salary": 9}, ana), 417, "ValidationError")
	r = x.call("PUT", "/api/portal/members/me/doc/"+x.anaMember, map[string]any{"phone": "555", "modified": me["modified"]}, ana)
	x.expect(r, 200, "")
	if data(r)["phone"] != "555" {
		t.Fatalf("update = %v", data(r))
	}
	// a stale form is refused
	x.expect(x.call("PUT", "/api/portal/members/me/doc/"+x.anaMember, map[string]any{"phone": "666", "modified": me["modified"]}, ana), 409, "")
	// Beto's member is not hers
	x.expect(x.call("GET", "/api/portal/members/me/doc/"+x.betoMember, nil, ana), 403, "PermissionError")

	// defaults are limited to the editable fields
	r = x.call("GET", "/api/portal/members/claims/new", nil, ana)
	x.expect(r, 200, "")
	if d := data(r); d["subject"] != "Expense" || d["internal_note"] != nil {
		t.Fatalf("defaults = %v", d)
	}
	// create: the member is forced to hers, whatever the body says
	x.expect(x.call("POST", "/api/portal/members/claims/doc", map[string]any{"subject": "x", "member": x.betoMember}, ana), 417, "ValidationError")
	r = x.call("POST", "/api/portal/members/claims/doc", map[string]any{"subject": "Taxi", "kind": "Travel"}, ana)
	x.expect(r, 200, "")
	claim := data(r)
	if claim["subject"] != "Taxi" {
		t.Fatalf("create = %v", claim)
	}
	// a field left out starts from the defaults
	r = x.call("POST", "/api/portal/members/claims/doc", map[string]any{"kind": "Travel"}, ana)
	x.expect(r, 200, "")
	if data(r)["subject"] != "Expense" {
		t.Fatalf("an omitted field did not take its default: %v", data(r))
	}
	r = x.call("GET", "/api/portal/members/claims/list", nil, ana)
	x.expect(r, 200, "")
	rows, _ := data(r)["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("list = %v", rows)
	}
	x.expect(x.call("GET", "/api/portal/members/claims/doc/"+x.betoClaim, nil, ana), 403, "PermissionError")
	// the page does not write claims
	x.expect(x.call("PUT", "/api/portal/members/claims/doc/"+fmt.Sprint(claim["id"]), map[string]any{"subject": "y"}, ana), 403, "PermissionError")
	// link search, only for an editable Link
	r = x.call("GET", "/api/portal/members/claims/search/kind?q=tra", nil, ana)
	x.expect(r, 200, "")
	if items, _ := r.Body["data"].([]any); len(items) != 1 {
		t.Fatalf("search = %v", r.Raw)
	}
	x.expect(x.call("GET", "/api/portal/members/claims/search/member?q=", nil, ana), 403, "PermissionError")
	x.expect(x.call("GET", "/api/portal/members/nope", nil, ana), 404, "")
}

func TestPortal_UploadsAndFiles(t *testing.T) {
	x := setupPortalAPI(t)
	ana, beto := "sid:"+x.sid(portalAna), "sid:"+x.sid(portalBeto)
	desk := "sid:" + x.sid(portalDesk)

	// a Website User must name an editable attachment field
	x.expect(x.uploadField(ana, "", "", "", "a.txt", "x"), 403, "PermissionError")
	x.expect(x.uploadField(ana, "Member", "", "phone", "a.txt", "x"), 403, "PermissionError")
	up := x.uploadField(ana, "Claim", "", "receipt", "receipt.pdf", "%PDF-1.4 receipt")
	x.expect(up, 200, "")
	url := fmt.Sprint(data(up)["file_url"])
	if !strings.HasPrefix(url, "/private/files/") {
		t.Fatalf("a portal upload must be private: %s", url)
	}
	r := x.call("POST", "/api/portal/members/claims/doc", map[string]any{"subject": "With receipt", "receipt": url}, ana)
	x.expect(r, 200, "")
	// the reviewer reads the file through the document; the other member cannot
	x.expect(x.call("GET", url, nil, desk), 200, "")
	x.expect(x.call("GET", url, nil, ana), 200, "")
	x.expect(x.call("GET", url, nil, beto), 403, "PermissionError")
}

func TestPortal_UploadNeedsWriteOnTheDocument(t *testing.T) {
	x := setupPortalAPI(t)
	// ze has no role at all: he may not hang a file on someone's claim
	ze := "sid:" + x.sid("ze@x.com")
	x.expect(x.uploadField(ze, "Claim", x.betoClaim, "receipt", "a.txt", "x"), 403, "PermissionError")
	x.expect(x.uploadField(ze, "Claim", "", "receipt", "a.txt", "x"), 403, "PermissionError")
	desk := "sid:" + x.sid(portalDesk)
	x.expect(x.uploadField(desk, "Claim", x.betoClaim, "receipt", "a.txt", "x"), 200, "")
	// an id that does not exist yet is a document being created
	x.expect(x.uploadField(desk, "Claim", "not-saved-yet", "receipt", "a.txt", "x"), 200, "")
}

func TestPortal_WriteLimit(t *testing.T) {
	x := setupPortalAPI(t)
	ana := "sid:" + x.sid(portalAna)
	limit := x.e.Cfg.Portal.Writes()
	// fill the window directly: sixty real inserts would only slow the test
	for i := 0; i < limit; i++ {
		x.s.limiter.allow("write:"+portalAna, limit, time.Now())
	}
	r := x.call("POST", "/api/portal/members/claims/doc", map[string]any{"subject": "one too many"}, ana)
	x.expect(r, 429, "")
	if r.Header.Get("Retry-After") == "" {
		t.Fatalf("a throttled write says when to retry")
	}
	// a desk user is not metered
	desk := "sid:" + x.sid(portalDesk)
	x.expect(x.call("POST", "/api/resource/Claim", map[string]any{"subject": "desk", "member": x.betoMember}, desk), 200, "")
}

func TestPortalLimiter_SlidingWindow(t *testing.T) {
	l := newPortalLimiter()
	now := time.Now()
	for i := 0; i < 2; i++ {
		if ok, _ := l.allow("k", 2, now); !ok {
			t.Fatal("within the limit")
		}
	}
	if ok, wait := l.allow("k", 2, now); ok || wait <= 0 {
		t.Fatal("past the limit")
	}
	if ok, _ := l.allow("k", 2, now.Add(61*time.Minute)); !ok {
		t.Fatal("the window slides")
	}
}
