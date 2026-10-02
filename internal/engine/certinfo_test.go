package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/js"
)

// certInfoOf evaluates expr, a ddcore.crypto call, and returns what it describes.
func certInfoOf(t *testing.T, rt *js.Runtime, expr string) map[string]any {
	t.Helper()
	v, err := rt.Eval(expr)
	if err != nil {
		t.Fatal(err)
	}
	var info map[string]any
	if err := json.Unmarshal([]byte(v.String()), &info); err != nil {
		t.Fatal(err)
	}
	return info
}

// wantClientCertInfo checks info against the fixture's client certificate.
func wantClientCertInfo(t *testing.T, f *mtlsFixture, info map[string]any, chain float64) {
	t.Helper()
	want := map[string]any{
		"notBefore": f.clientCert.NotBefore.UTC().Format(time.RFC3339),
		"notAfter":  f.clientCert.NotAfter.UTC().Format(time.RFC3339),
		"subject":   "CN=pix client",
		"issuer":    "CN=test ca",
		"serial":    "3",
		"chain":     chain,
	}
	if len(info) != len(want) {
		t.Fatalf("got %v, want exactly the keys of %v", info, want)
	}
	for k, w := range want {
		if info[k] != w {
			t.Errorf("%s = %v, want %v", k, info[k], w)
		}
	}
}

// A PKCS#12 file is described by its leaf, and by nothing of its key.
func TestCryptoPfxInfo(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	info := certInfoOf(t, rt, fmt.Sprintf(`ddcore.crypto.pfxInfo(%q, %q)`, f.pfx, mtlsPassword))
	wantClientCertInfo(t, f, info, 1)
}

// A base64 file pasted into a secret often comes wrapped in lines.
func TestCryptoPfxInfoLineWrappedBase64(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	var wrapped strings.Builder
	for i := 0; i < len(f.pfx); i += 64 {
		wrapped.WriteString(f.pfx[i:min(i+64, len(f.pfx))])
		wrapped.WriteString("\n")
	}
	info := certInfoOf(t, rt, fmt.Sprintf(`ddcore.crypto.pfxInfo(%q, %q)`, wrapped.String(), mtlsPassword))
	wantClientCertInfo(t, f, info, 1)
}

func TestCryptoCertInfo(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	info := certInfoOf(t, rt, fmt.Sprintf(`ddcore.crypto.certInfo(%q)`, f.certPEM))
	wantClientCertInfo(t, f, info, 0)

	// a key pasted along with the certificate is skipped, and a second
	// certificate counts as the chain
	info = certInfoOf(t, rt, fmt.Sprintf(`ddcore.crypto.certInfo(%q)`, f.keyPEM+f.certPEM+f.certPEM))
	wantClientCertInfo(t, f, info, 1)
}

// A certificate that cannot be read is a validation error that says nothing
// of the material.
func TestCryptoCertInfoErrors(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	garbage := base64.StdEncoding.EncodeToString([]byte("not a pkcs12 file at all"))
	badPEM := "-----BEGIN CERTIFICATE-----\n" + garbage + "\n-----END CERTIFICATE-----\n"
	for _, tc := range []struct{ name, call, want string }{
		{"wrong password", fmt.Sprintf(`pfxInfo(%q, "wr0ng-guess")`, f.pfx), "wrong password or malformed file"},
		{"no password", fmt.Sprintf(`pfxInfo(%q)`, f.pfx), "wrong password or malformed file"},
		{"malformed file", fmt.Sprintf(`pfxInfo(%q, %q)`, garbage, mtlsPassword), "wrong password or malformed file"},
		{"not base64", fmt.Sprintf(`pfxInfo("@@not-base64@@", %q)`, mtlsPassword), "not valid base64"},
		{"empty pem", `certInfo("")`, "not a valid PEM certificate"},
		{"key only", fmt.Sprintf(`certInfo(%q)`, f.keyPEM), "not a valid PEM certificate"},
		{"bad certificate", fmt.Sprintf(`certInfo(%q)`, badPEM), "not a valid PEM certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rt.Eval(`ddcore.crypto.` + tc.call)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			typ, terr := rt.Eval(`(() => { try { ddcore.crypto.` + tc.call + ` } catch (e) { return e.name } })()`)
			if terr != nil || !strings.Contains(typ.String(), "ValidationError") {
				t.Fatalf("want a ValidationError, got %v (%v)", typ, terr)
			}
			for _, secret := range []string{mtlsPassword, "wr0ng-guess", f.pfx[:24], garbage, "not-base64", "BEGIN"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("the error echoes the material: %v", err)
				}
			}
		})
	}
}
