package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/js"
)

// credentialSite loads two apps, "erp" and "crm", whose manifests carry the
// given auth blocks, without a database: the checks run in Load. erp adds a
// Data field erp_user and a Check field erp_flag to User.
func credentialSite(t *testing.T, erp, crm string, oidc ...config.OIDCProvider) (*Engine, error) {
	t.Helper()
	root := t.TempDir()
	for _, app := range []struct{ name, auth string }{{"erp", erp}, {"crm", crm}} {
		dir := filepath.Join(root, app.name)
		os.MkdirAll(filepath.Join(dir, "extensions"), 0o755)
		extra := ""
		if app.auth != "" {
			extra = ", auth: " + app.auth
		}
		src := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "` + app.name + `", title: "` + app.name + `"` + extra + ` });`
		if err := os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ext := `import { extendDoctype } from "@ddcore/sdk";
export default extendDoctype("User", { fields: [
  { fieldname: "erp_user", fieldtype: "Data", label: "ERP user" },
  { fieldname: "erp_flag", fieldtype: "Check", label: "ERP flag" },
] });`
	if err := os.WriteFile(filepath.Join(root, "erp", "extensions", "user.extend.ts"), []byte(ext), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DSN: unreachableDSN, DeferDB: true, LogOut: io.Discard, OIDC: oidc, Apps: []js.App{
		{Name: "erp", Dir: filepath.Join(root, "erp")}, {Name: "crm", Dir: filepath.Join(root, "crm")},
	}}
	return New(context.Background(), cfg)
}

const okProvider = `{ label: "ERP", userField: "erp_user", verify() { return { ok: false }; } }`

func TestCredentialProvidersLoad(t *testing.T) {
	e, err := credentialSite(t, `{ providers: { erp: { label: "ERP", userField: "erp_user",
	  enabled() { return true; }, verify() { return { ok: false }; }, afterSignIn() {} } } }`, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p, ok := e.Current().CredentialProvider("erp")
	want := CredentialProvider{ID: "erp", App: "erp", Label: "ERP", UserField: "erp_user", HasEnabled: true, HasAfterSignIn: true}
	if !ok || p != want {
		t.Fatalf("provider %+v, want %+v", p, want)
	}
	if got := e.Current().CredentialProviders(); len(got) != 1 {
		t.Fatalf("providers %+v", got)
	}
}

func TestCredentialProvidersRefuseBadDeclarations(t *testing.T) {
	for _, c := range []struct{ name, erp, crm, want string }{
		{"bad id", `{ providers: { "ERP-1": ` + okProvider + ` } }`, "", "the id must be"},
		{"no label", `{ providers: { erp: { userField: "erp_user", verify() { return { ok: false }; } } } }`, "", "label is required"},
		{"no verify", `{ providers: { erp: { label: "ERP", userField: "erp_user" } } }`, "", "verify must be a function"},
		{"no userField", `{ providers: { erp: { label: "ERP", verify() { return { ok: false }; } } } }`, "", "userField is required"},
		{"unknown userField", `{ providers: { erp: { label: "ERP", userField: "nope", verify() { return { ok: false }; } } } }`, "", "is not a field of User"},
		{"not Data", `{ providers: { erp: { label: "ERP", userField: "erp_flag", verify() { return { ok: false }; } } } }`, "", "must be a stored Data field"},
		{"two apps", `{ providers: { erp: ` + okProvider + ` } }`, `{ providers: { erp: ` + okProvider + ` } }`, "is taken by app"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := credentialSite(t, c.erp, c.crm)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error containing %q", err, c.want)
			}
		})
	}
	t.Run("an OIDC provider's id", func(t *testing.T) {
		_, err := credentialSite(t, `{ providers: { google: `+okProvider+` } }`, "", config.OIDCProvider{ID: "google"})
		if err == nil || !strings.Contains(err.Error(), "single sign-on provider") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestUndeliverable(t *testing.T) {
	for addr, want := range map[string]bool{
		"joao@modaverao.invalid": true, "JOAO@X.INVALID": true, "a@invalid": true,
		"a@example.com": false, "a@invalid.com": false, "nobody": false,
	} {
		if got := Undeliverable(addr); got != want {
			t.Errorf("Undeliverable(%q) = %v, want %v", addr, got, want)
		}
	}
}
