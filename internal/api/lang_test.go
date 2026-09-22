package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	// the key is the English source text, and it travels with its arguments
	// for telemetry and grouping
	if e["key"] != "Method {0} does not exist or is not whitelisted" {
		t.Fatalf("key = %v", e["key"])
	}
	if args, _ := e["args"].([]any); len(args) != 1 || args[0] != "nope" {
		t.Fatalf("args = %v", e["args"])
	}

	// English is the source language: the key renders as itself
	en := x.call("GET", "/api/method/nope", nil, sid, "X-Lang", "en")
	e, _ = en.Body["error"].(map[string]any)
	if e["message"] != "Method nope does not exist or is not whitelisted" {
		t.Fatalf("en message = %v", e["message"])
	}
}

// A title is a key too, and gets translated alongside the message.
func TestErrorTitleTranslatedAtTheBorder(t *testing.T) {
	x := setup(t)
	sid := "sid:" + x.sid("ana@x.com")
	r := x.call("POST", "/api/resource/Pedido", map[string]any{}, sid, "X-Lang", "pt-BR")
	e, _ := r.Body["error"].(map[string]any)
	if e["titleKey"] != "Required fields" {
		t.Fatalf("titleKey = %v", e["titleKey"])
	}
	if e["title"] != "Campos obrigatórios" {
		t.Fatalf("title = %v (message %v)", e["title"], e["message"])
	}
	if e["message"] != "Preencha os campos obrigatórios: Cliente" {
		t.Fatalf("message = %v", e["message"])
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

// TestMetaTranslatedAndETag: metadata is a catalogue key like any other, and
// the ETag has to follow the language or a 304 hands back the wrong payload.
func TestMetaTranslatedAndETag(t *testing.T) {
	x := setup(t)
	// User is a System Manager DocType, so its meta is read as one
	sid := "sid:" + x.sid("root@x.com")

	label := func(r resp) string {
		data, _ := r.Body["data"].(map[string]any)
		dt, _ := data["doctype"].(map[string]any)
		s, _ := dt["label"].(string)
		return s
	}

	pt := x.call("GET", "/api/meta/User", nil, sid, "X-Lang", "pt-BR")
	if got := label(pt); got != "Usuário" {
		t.Fatalf("pt-BR label = %q, want Usuário", got)
	}
	en := x.call("GET", "/api/meta/User", nil, sid, "X-Lang", "en")
	if got := label(en); got != "User" {
		t.Fatalf("en label = %q, want User", got)
	}

	etagPT, etagEN := pt.Header.Get("ETag"), en.Header.Get("ETag")
	if etagPT == "" || etagPT == etagEN {
		t.Fatalf("the ETag must vary by language: %q vs %q", etagPT, etagEN)
	}
	// same language, same ETag → 304; other language → a full answer
	if r := x.call("GET", "/api/meta/User", nil, sid, "X-Lang", "pt-BR", "If-None-Match", etagPT); r.Status != 304 {
		t.Fatalf("same language: status = %d, want 304", r.Status)
	}
	if r := x.call("GET", "/api/meta/User", nil, sid, "X-Lang", "en", "If-None-Match", etagPT); r.Status != 200 || label(r) != "User" {
		t.Fatalf("other language: status = %d, label = %q", r.Status, label(r))
	}
}

// idLabel is a catalogue key like label, translated with the rest of the meta.
func TestMetaTranslatesTheIDLabel(t *testing.T) {
	x := setup(t)
	d, err := x.e.DocType("User")
	if err != nil {
		t.Fatal(err)
	}
	d.IDLabel = "User" // a key the core catalogue already translates
	r := x.call("GET", "/api/meta/User", nil, "sid:"+x.sid("root@x.com"), "X-Lang", "pt-BR")
	data, _ := r.Body["data"].(map[string]any)
	dt, _ := data["doctype"].(map[string]any)
	if got := dt["idLabel"]; got != "Usuário" {
		t.Fatalf("pt-BR idLabel = %v, want Usuário", got)
	}
	if d.IDLabel != "User" {
		t.Fatalf("the registry was mutated: idLabel = %q", d.IDLabel)
	}
}

// The registry is shared by every request in flight: translating for one must
// not leave the next one reading Portuguese.
func TestTranslationDoesNotMutateTheRegistry(t *testing.T) {
	x := setup(t)
	d, err := x.e.DocType("User")
	if err != nil {
		t.Fatal(err)
	}
	// snapshot first: `language` carries autonyms from the start, so the test
	// is that translating gains nothing, not that nothing is there.
	before := map[string][]string{}
	for _, f := range d.Fields {
		before[f.Fieldname] = f.OptionLabels
	}

	x.call("GET", "/api/meta/User", nil, "sid:"+x.sid("root@x.com"), "X-Lang", "pt-BR")

	if d.Label != "User" {
		t.Fatalf("the registry was mutated: label = %q", d.Label)
	}
	for _, f := range d.Fields {
		if !reflect.DeepEqual(f.OptionLabels, before[f.Fieldname]) {
			t.Fatalf("the registry was mutated: %s labels %v → %v", f.Fieldname, before[f.Fieldname], f.OptionLabels)
		}
	}
}

// A Select is canonical English in the database; only its display text moves.
func TestSelectOptionsStayCanonical(t *testing.T) {
	x := setup(t)
	r := x.call("GET", "/api/meta/User", nil, "sid:"+x.sid("root@x.com"), "X-Lang", "pt-BR")
	data, _ := r.Body["data"].(map[string]any)
	dt, _ := data["doctype"].(map[string]any)
	fields, _ := dt["fields"].([]any)
	for _, fAny := range fields {
		f, _ := fAny.(map[string]any)
		if f["fieldname"] != "user_type" {
			continue
		}
		opts, _ := f["options"].([]any)
		if len(opts) != 2 || opts[0] != "System User" {
			t.Fatalf("options are not canonical: %v", opts)
		}
		return
	}
	t.Fatal("user_type not found")
}

// The prelude mirrors the catalogue per (VM, language) instead of crossing the
// bridge for every string. VMs are pooled and reused across requests, so the
// mirror has to be keyed by language — this is the test that says so.
func TestRuntimeCatalogueFollowsTheRequestLanguage(t *testing.T) {
	x := setup(t)
	sid := "sid:" + x.sid("ana@x.com")
	call := func(lang string) map[string]any {
		r := x.call("POST", "/api/method/demo.services.i18n.echo", map[string]any{}, sid, "X-Lang", lang)
		data, _ := r.Body["data"].(map[string]any)
		if data == nil {
			t.Fatalf("%s: %s", lang, r.Raw)
		}
		return data
	}
	// the test app's en.csv maps "Loop"; core's pt-BR.csv maps "Save"
	if got := call("pt-BR"); got["save"] != "Salvar" || got["n"] != "Loop" {
		t.Fatalf("pt-BR = %v", got)
	}
	if got := call("en"); got["save"] != "Save" || got["n"] != "Looped" {
		t.Fatalf("en = %v", got)
	}
	// back again on the same pool: the mirror must not have gone stale
	if got := call("pt-BR"); got["save"] != "Salvar" {
		t.Fatalf("second pt-BR = %v", got)
	}
}

// TestUserLanguageIsAPicker: the list of languages a site serves is not knowable
// when the DocType is declared, so it is injected into the registry at load.
func TestUserLanguageIsAPicker(t *testing.T) {
	x := setup(t)
	sid := "sid:" + x.sid("root@x.com")

	field := func(lang string) map[string]any {
		r := x.call("GET", "/api/meta/User", nil, sid, "X-Lang", lang)
		data, _ := r.Body["data"].(map[string]any)
		dt, _ := data["doctype"].(map[string]any)
		fields, _ := dt["fields"].([]any)
		for _, fAny := range fields {
			f, _ := fAny.(map[string]any)
			if f["fieldname"] == "language" {
				return f
			}
		}
		t.Fatalf("%s: no language field", lang)
		return nil
	}

	pt, en := field("pt-BR"), field("en")
	if pt["fieldtype"] != "Select" {
		t.Fatalf("fieldtype = %v, want Select", pt["fieldtype"])
	}
	// the options are the site's languages, canonical and identical either way
	want := []any{"en", "pt-BR"}
	if !reflect.DeepEqual(pt["options"], want) || !reflect.DeepEqual(en["options"], want) {
		t.Fatalf("options: pt-BR=%v en=%v, want %v", pt["options"], en["options"], want)
	}
	// autonyms: each language in itself, so the labels do NOT vary by request
	labels, _ := pt["optionLabels"].([]any)
	if len(labels) != 2 || labels[0] != "English" {
		t.Fatalf("optionLabels = %v", pt["optionLabels"])
	}
	if !reflect.DeepEqual(pt["optionLabels"], en["optionLabels"]) {
		t.Fatalf("autonyms must not vary by language: pt-BR=%v en=%v", pt["optionLabels"], en["optionLabels"])
	}

	// and, being a Select, a language the site does not serve is refused
	x.asAdmin(func(c *engine.Ctx) error {
		d, err := c.GetDoc("User", "ana@x.com")
		if err != nil {
			return err
		}
		d["language"] = "xx"
		if _, err := c.Save(d, engine.SaveOpts{}); err == nil {
			t.Fatal("saving an unknown language should be refused")
		}
		return nil
	})
}

// TestSetMyLanguage: permissions are per document, so a user cannot write their
// own User record — the whitelisted method is the way round that, and it has to
// drop the cached language itself because it writes without firing the hooks.
func TestSetMyLanguage(t *testing.T) {
	x := setup(t)
	const path = "/api/method/core.services.i18n.setMyLanguage"
	sid := "sid:" + x.sid("ana@x.com")

	// the premise: ana cannot write her own User document
	r := x.call("PUT", "/api/resource/User/ana@x.com", map[string]any{"language": "en"}, sid)
	if r.Status == 200 {
		t.Fatal("ana should not be able to write her own User record")
	}

	// but she can set her own language
	r = x.call("POST", path, map[string]any{"language": "en"}, sid)
	if data, _ := r.Body["data"].(map[string]any); data == nil || data["language"] != "en" {
		t.Fatalf("set: %s", r.Raw)
	}
	if got := x.call("GET", "/api/boot", nil, sid); func() string {
		d, _ := got.Body["data"].(map[string]any)
		s, _ := d["lang"].(string)
		return s
	}() != "en" {
		t.Fatalf("the boot should answer in the new language: %s", got.Raw)
	}

	// blank means "follow the site"
	r = x.call("POST", path, map[string]any{"language": ""}, sid)
	if data, _ := r.Body["data"].(map[string]any); data == nil || data["language"] != nil {
		t.Fatalf("blank: %s", r.Raw)
	}

	// and a language the site does not serve is refused
	r = x.call("POST", path, map[string]any{"language": "xx"}, sid)
	if r.Status == 200 {
		t.Fatalf("an unknown language should be refused: %s", r.Raw)
	}
}
