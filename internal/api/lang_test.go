package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// req builds a request already carrying the user the auth middleware would
// have put in the context.
func req(user string, hdr ...string) *http.Request {
	r := httptest.NewRequest("GET", "/api/boot", nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	return r.WithContext(context.WithValue(r.Context(), userKey, user))
}

// TestLangFor walks the five rungs of the resolution ladder:
// X-Lang → User.language (cached) → Accept-Language (Guest only) → site → "en".
func TestLangFor(t *testing.T) {
	x := setup(t)
	// core ships translations/pt-BR.csv, so the site serves en and pt-BR.
	if got := x.e.Current().I18n.Langs(); len(got) != 2 || got[0] != "en" || got[1] != "pt-BR" {
		t.Fatalf("Langs() = %v, want [en pt-BR]", got)
	}
	if x.e.Cfg.Lang != "pt-BR" {
		t.Fatalf("cfg.Lang = %q", x.e.Cfg.Lang)
	}

	x.e.Cache.Set("lang:ana@x.com", "en", time.Minute)
	defer x.e.Cache.Del("lang:ana@x.com")

	cases := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"X-Lang wins over everything", req("ana@x.com", "X-Lang", "pt-BR", "Accept-Language", "en"), "pt-BR"},
		{"X-Lang wins for Guest too", req("Guest", "X-Lang", "en"), "en"},
		{"unknown X-Lang is ignored", req("ana@x.com", "X-Lang", "zz-ZZ"), "en"},
		{"User.language from the cache", req("ana@x.com"), "en"},
		{"Accept-Language for Guest", req("Guest", "Accept-Language", "en-US,en;q=0.9"), "en"},
		{"Accept-Language is not read for a session", req("bia@x.com", "Accept-Language", "en-US,en;q=0.9"), "pt-BR"},
		{"unmatched Accept-Language falls to the site", req("Guest", "Accept-Language", "ja-JP"), "pt-BR"},
		{"nothing at all falls to the site", req("Guest"), "pt-BR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := x.s.langFor(tc.r); got != tc.want {
				t.Fatalf("langFor = %q, want %q", got, tc.want)
			}
		})
	}

	// last rung: no site language configured at all.
	old := x.e.Cfg.Lang
	x.e.Cfg.Lang = ""
	defer func() { x.e.Cfg.Lang = old }()
	if got := x.s.langFor(req("Guest")); got != "en" {
		t.Fatalf("without cfg.Lang: langFor = %q, want en", got)
	}
}

// TestBootResolvesUserLanguage checks the one place that reads User.language:
// the boot answers in it and leaves it in the cache for later requests.
func TestBootResolvesUserLanguage(t *testing.T) {
	x := setup(t)
	x.asAdmin(func(c *engine.Ctx) error {
		d, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		d["language"] = "en"
		_, err = c.Save(d, engine.SaveOpts{})
		return err
	})
	x.e.Cache.Del("lang:ana@x.com")

	sid := "sid:" + x.sid("ana@x.com")
	r := x.call("GET", "/api/boot", nil, sid)
	data, _ := r.Body["data"].(map[string]any)
	if data["lang"] != "en" {
		t.Fatalf("boot.lang = %v, want en", data["lang"])
	}
	if v, ok := x.e.Cache.Get("lang:ana@x.com"); !ok || v != "en" {
		t.Fatalf("cache lang: %v %v", v, ok)
	}
	// the site timezone travels in the boot from this phase on
	site, _ := data["site"].(map[string]any)
	if site["timezone"] != "UTC" {
		t.Fatalf("site.timezone = %v, want UTC", site["timezone"])
	}
	// an explicit X-Lang still outranks the stored preference
	r = x.call("GET", "/api/boot", nil, sid, "X-Lang", "pt-BR")
	data, _ = r.Body["data"].(map[string]any)
	if data["lang"] != "pt-BR" {
		t.Fatalf("with X-Lang: boot.lang = %v, want pt-BR", data["lang"])
	}
}

// TestMetaVariesByLang guards the header that keeps a proxy from serving one
// language's payload to another.
func TestMetaVariesByLang(t *testing.T) {
	x := setup(t)
	r := x.call("GET", "/api/meta/Pessoa", nil, "sid:"+x.sid("ana@x.com"))
	if r.Header.Get("Vary") != "X-Lang" {
		t.Fatalf("Vary = %q, want X-Lang", r.Header.Get("Vary"))
	}
}

// TestErrorTranslatedAtTheBorder: a typed error keeps its key and arguments,
// and `writeErr` — the one place an error becomes a response — renders it in
// the request's language.
func TestErrorTranslatedAtTheBorder(t *testing.T) {
	x := setup(t)
	sid := "sid:" + x.sid("ana@x.com")

	pt := x.call("GET", "/api/method/nope", nil, sid, "X-Lang", "pt-BR")
	e, _ := pt.Body["error"].(map[string]any)
	if e["message"] != "Método nope não existe ou não é whitelisted" {
		t.Fatalf("pt-BR message = %v", e["message"])
	}
	// key and args travel too, for telemetry and grouping
	if e["key"] != "Método {0} não existe ou não é whitelisted" {
		t.Fatalf("key = %v", e["key"])
	}
	if args, _ := e["args"].([]any); len(args) != 1 || args[0] != "nope" {
		t.Fatalf("args = %v", e["args"])
	}

	en := x.call("GET", "/api/method/nope", nil, sid, "X-Lang", "en")
	e, _ = en.Body["error"].(map[string]any)
	if e["message"] != "Method nope does not exist or is not whitelisted" {
		t.Fatalf("en message = %v", e["message"])
	}
}

// An error raised inside the JS runtime went through `_()` there, so it
// arrives with no key. The border must leave it alone — even when the text
// happens to match a catalogue entry, which is exactly when translating it a
// second time would corrupt it.
func TestJSErrorIsNotRetranslated(t *testing.T) {
	x := setup(t)
	// the test app's en.csv maps "Loop" to "Looped"
	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "loop"}, "sid:"+x.sid("ana@x.com"), "X-Lang", "en")
	e, _ := r.Body["error"].(map[string]any)
	if e == nil {
		t.Fatalf("expected an error, got %s", r.Raw)
	}
	if e["message"] != "Loop" {
		t.Fatalf("message = %v, want Loop (translated once, in the VM)", e["message"])
	}
	if e["key"] != nil {
		t.Fatalf("a VM error should carry no key, got %v", e["key"])
	}
}
