package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupCORS is the test app with three guest methods: two that opt into CORS,
// one of them POST only, and one that does not.
func setupCORS(t *testing.T, origins ...string) *env {
	t.Helper()
	dir := testApp(t)
	src := `import { whitelisted } from "@ddcore/sdk";
export const open = whitelisted(() => "ok", { allowGuest: true, cors: true });
export const pay = whitelisted(() => "paid", { allowGuest: true, cors: true, methods: ["POST"] });
export const shut = whitelisted(() => "ok", { allowGuest: true });
export const hook = whitelisted(() => ddcore.session.request.pathTail, { allowGuest: true, cors: true, pathTail: true });`
	if err := os.WriteFile(filepath.Join(dir, "services/cors.ts"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	x := setupApp(t, dir)
	x.e.Cfg.CORS.Origins = origins
	return x
}

func (x *env) preflight(path, origin, method string) resp {
	x.t.Helper()
	req, _ := http.NewRequest("OPTIONS", x.ts.URL+path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if method != "" {
		req.Header.Set("Access-Control-Request-Method", method)
	}
	req.Header.Set("Access-Control-Request-Headers", "content-type,authorization")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	res.Body.Close()
	return resp{Status: res.StatusCode, Header: res.Header}
}

func corsHeaders(h http.Header) []string {
	var out []string
	for k := range h {
		if strings.HasPrefix(k, "Access-Control-") {
			out = append(out, k)
		}
	}
	return out
}

func TestCORSPreflightAccepted(t *testing.T) {
	x := setupCORS(t, "https://shop.example.com")
	r := x.preflight("/api/method/demo.services.cors.open", "https://shop.example.com", "POST")
	if r.Status != 204 {
		t.Fatalf("status %d", r.Status)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":  "https://shop.example.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Authorization, Content-Type, X-Tenant",
		"Access-Control-Max-Age":       "600",
		"Vary":                         "Origin",
	}
	for k, v := range want {
		if got := r.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if r.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Error("a preflight must never allow credentials")
	}
	// the origin is compared without regard to case, and echoed as sent
	if r := x.preflight("/api/method/demo.services.cors.open", "https://SHOP.example.com", "GET"); r.Status != 204 || r.Header.Get("Access-Control-Allow-Origin") != "https://SHOP.example.com" {
		t.Fatalf("case: %d %q", r.Status, r.Header.Get("Access-Control-Allow-Origin"))
	}
	// Allow-Methods follows the method's own `methods`
	if r := x.preflight("/api/method/demo.services.cors.pay", "https://shop.example.com", "POST"); r.Status != 204 || r.Header.Get("Access-Control-Allow-Methods") != "POST" {
		t.Fatalf("pay: %d %q", r.Status, r.Header.Get("Access-Control-Allow-Methods"))
	}
	// a pathTail method is reached below its path, as the real call would be
	if r := x.preflight("/api/method/demo.services.cors.hook/pix", "https://shop.example.com", "POST"); r.Status != 204 {
		t.Fatalf("hook/pix: %d", r.Status)
	}
}

func TestCORSPreflightRefused(t *testing.T) {
	x := setupCORS(t, "https://shop.example.com", "https://*.partner.com")
	for _, c := range []struct{ name, path, origin, method string }{
		{"unknown origin", "/api/method/demo.services.cors.open", "https://evil.example.com", "POST"},
		{"other scheme", "/api/method/demo.services.cors.open", "http://shop.example.com", "POST"},
		{"other port", "/api/method/demo.services.cors.open", "https://shop.example.com:8443", "POST"},
		{"method without cors", "/api/method/demo.services.cors.shut", "https://shop.example.com", "POST"},
		{"no such method", "/api/method/demo.services.cors.nope", "https://shop.example.com", "POST"},
		{"sub-path without pathTail", "/api/method/demo.services.cors.open/x", "https://shop.example.com", "POST"},
		{"bare domain of a wildcard", "/api/method/demo.services.cors.open", "https://partner.com", "POST"},
		{"lookalike of a wildcard", "/api/method/demo.services.cors.open", "https://evilpartner.com", "POST"},
		{"no origin", "/api/method/demo.services.cors.open", "", "POST"},
		{"not a preflight", "/api/method/demo.services.cors.open", "https://shop.example.com", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := x.preflight(c.path, c.origin, c.method)
			if r.Status != 403 || len(corsHeaders(r.Header)) != 0 {
				t.Fatalf("%d %v", r.Status, corsHeaders(r.Header))
			}
		})
	}
	// the wildcard does match a subdomain, any depth
	for _, o := range []string{"https://a.partner.com", "https://x.y.partner.com"} {
		if r := x.preflight("/api/method/demo.services.cors.open", o, "POST"); r.Status != 204 || r.Header.Get("Access-Control-Allow-Origin") != o {
			t.Fatalf("%s: %d", o, r.Status)
		}
	}
}

func TestCORSAnyOrigin(t *testing.T) {
	x := setupCORS(t, "*")
	r := x.preflight("/api/method/demo.services.cors.open", "https://anyone.example", "POST")
	// echoed rather than "*", so a cache keyed on Vary: Origin stays right
	if r.Status != 204 || r.Header.Get("Access-Control-Allow-Origin") != "https://anyone.example" {
		t.Fatalf("%d %q", r.Status, r.Header.Get("Access-Control-Allow-Origin"))
	}
	if r := x.preflight("/api/method/demo.services.cors.shut", "https://anyone.example", "POST"); r.Status != 403 {
		t.Fatalf("a method without cors under *: %d", r.Status)
	}
}

func TestCORSNoOriginsConfigured(t *testing.T) {
	x := setupCORS(t)
	if r := x.preflight("/api/method/demo.services.cors.open", "https://shop.example.com", "POST"); r.Status != 403 {
		t.Fatalf("%d", r.Status)
	}
}

func TestCORSActualRequest(t *testing.T) {
	x := setupCORS(t, "https://shop.example.com")
	r := x.call("POST", "/api/method/demo.services.cors.open", map[string]any{}, "", "Origin", "https://shop.example.com")
	if r.Status != 200 || r.Body["data"] != "ok" {
		t.Fatalf("%d %s", r.Status, r.Raw)
	}
	if r.Header.Get("Access-Control-Allow-Origin") != "https://shop.example.com" ||
		r.Header.Get("Access-Control-Expose-Headers") != "X-Request-Id" ||
		!strings.Contains(r.Header.Get("Vary"), "Origin") {
		t.Fatalf("headers: %v", r.Header)
	}
	if r.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials allowed")
	}
	// an error the caller can read is still a response to the allowed origin
	r = x.call("GET", "/api/method/demo.services.cors.pay", nil, "", "Origin", "https://shop.example.com")
	if r.Status != 405 || r.Header.Get("Access-Control-Allow-Origin") != "https://shop.example.com" {
		t.Fatalf("405: %d %v", r.Status, r.Header)
	}
	// an API key authenticates a cross-origin call; nothing else is needed
	r = x.call("POST", "/api/method/demo.services.cors.open", map[string]any{}, "token:"+x.apiKey("ze@x.com"), "Origin", "https://shop.example.com")
	if r.Status != 200 || r.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("with a key: %d %v", r.Status, r.Header)
	}
}

// Everything that does not opt in behaves exactly as before.
func TestCORSLeavesTheRestAlone(t *testing.T) {
	x := setupCORS(t, "*")
	for _, c := range []struct{ name, method, path string }{
		{"method without cors", "POST", "/api/method/demo.services.cors.shut"},
		{"resource", "GET", "/api/resource/Pessoa"},
		{"boot", "GET", "/api/boot"},
	} {
		r := x.call(c.method, c.path, nil, "", "Origin", "https://shop.example.com")
		if h := corsHeaders(r.Header); len(h) != 0 {
			t.Fatalf("%s: %v", c.name, h)
		}
	}
	// the site's own pages, and a caller that sends no Origin, are not CORS
	for _, origin := range []string{x.ts.URL, ""} {
		var hdr []string
		if origin != "" {
			hdr = []string{"Origin", origin}
		}
		r := x.call("POST", "/api/method/demo.services.cors.open", map[string]any{}, "", hdr...)
		if r.Status != 200 || len(corsHeaders(r.Header)) != 0 {
			t.Fatalf("origin %q: %d %v", origin, r.Status, corsHeaders(r.Header))
		}
	}
	// a preflight to a resource is not answered as one
	if r := x.preflight("/api/resource/Pessoa", "https://shop.example.com", "GET"); len(corsHeaders(r.Header)) != 0 {
		t.Fatalf("resource preflight: %d %v", r.Status, corsHeaders(r.Header))
	}
}
