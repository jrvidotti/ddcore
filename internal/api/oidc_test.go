package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// fakeIdP is just enough of an OpenID provider: discovery, keys, and a token
// endpoint that signs whatever claims the test staged for a code.
type fakeIdP struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	codes  map[string]fakeGrant
	issuer string // what the id_token claims; defaults to the server URL
}

type fakeGrant struct {
	claims    map[string]any
	challenge string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{t: t, key: key, codes: map[string]fakeGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize",
			"token_endpoint": f.srv.URL + "/token", "jwks_uri": f.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		g, ok := f.codes[r.Form.Get("code")]
		delete(f.codes, r.Form.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant"})
			return
		}
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		payload, _ := json.Marshal(g.claims)
		jws, _ := signer.Sign(payload)
		tok, _ := jws.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": tok})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// ssoEnv wires a provider called "fake" into the test site.
func ssoEnv(t *testing.T, domains ...string) (*env, *fakeIdP) {
	x := setup(t)
	idp := newFakeIdP(t)
	x.e.Cfg.SiteURL = x.ts.URL
	x.e.Cfg.OIDC = []config.OIDCProvider{{ID: "fake", Label: "Fake", Issuer: idp.srv.URL,
		ClientID: "client-1", ClientSecret: "s3cret", Scopes: []string{"openid", "email"}, AllowedDomains: domains}}
	return x, idp
}

var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type ssoStart struct {
	state, nonce, challenge string
	cookie                  *http.Cookie
}

func (x *env) ssoStart(redirect string) ssoStart {
	x.t.Helper()
	res, err := noFollow.Get(x.ts.URL + "/api/auth/oidc/fake/start?redirect=" + url.QueryEscape(redirect))
	if err != nil {
		x.t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 302 {
		x.t.Fatalf("start: %d", res.StatusCode)
	}
	loc, _ := url.Parse(res.Header.Get("Location"))
	q := loc.Query()
	if q.Get("client_id") != "client-1" || q.Get("code_challenge_method") != "S256" ||
		q.Get("redirect_uri") != x.ts.URL+"/api/auth/oidc/fake/callback" {
		x.t.Fatalf("authorize URL: %s", loc)
	}
	var ck *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == oidcStateCookie {
			ck = c
		}
	}
	if ck == nil || !ck.HttpOnly || ck.Value != q.Get("state") {
		x.t.Fatalf("state cookie: %+v", ck)
	}
	return ssoStart{state: q.Get("state"), nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), cookie: ck}
}

// grant stages the id_token the provider will issue for code.
func (f *fakeIdP) grant(code string, st ssoStart, over map[string]any) {
	claims := map[string]any{
		"iss": f.srv.URL, "aud": "client-1", "sub": "sub-ana", "email": "ana@x.com", "email_verified": true,
		"nonce": st.nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}
	for k, v := range over {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	f.mu.Lock()
	f.codes[code] = fakeGrant{claims: claims, challenge: st.challenge}
	f.mu.Unlock()
}

// callback returns the redirect target and the session cookie, if any.
func (x *env) ssoCallback(query string, ck *http.Cookie) (string, string) {
	x.t.Helper()
	req, _ := http.NewRequest("GET", x.ts.URL+"/api/auth/oidc/fake/callback?"+query, nil)
	if ck != nil {
		req.AddCookie(ck)
	}
	res, err := noFollow.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 302 {
		x.t.Fatalf("callback: %d", res.StatusCode)
	}
	sid := ""
	for _, c := range res.Cookies() {
		if c.Name == "sid" && c.MaxAge > 0 {
			sid = c.Value
		}
	}
	return res.Header.Get("Location"), sid
}

func (x *env) ssoLogin(idp *fakeIdP, redirect string, over map[string]any) (string, string) {
	x.t.Helper()
	st := x.ssoStart(redirect)
	code := engine.RandomToken()
	idp.grant(code, st, over)
	return x.ssoCallback("code="+code+"&state="+url.QueryEscape(st.state), st.cookie)
}

func (x *env) countSQL(sql string, args ...any) int {
	x.t.Helper()
	var n int
	if err := x.e.DB.Pool.QueryRow(x.ctx, sql, args...).Scan(&n); err != nil {
		x.t.Fatal(err)
	}
	return n
}

func TestSEC05_SignInLinksByVerifiedEmail(t *testing.T) {
	x, idp := ssoEnv(t)
	loc, sid := x.ssoLogin(idp, "/app/pessoa", nil)
	if loc != "/app/pessoa" || sid == "" {
		t.Fatalf("sign-in: %q %q", loc, sid)
	}
	if u, _ := x.e.UserFromSession(x.ctx, sid); u != "ana@x.com" {
		t.Fatalf("session user: %q", u)
	}
	if n := x.countSQL(`SELECT count(*) FROM ddcore_user_identity WHERE provider='fake' AND subject='sub-ana' AND "user"='ana@x.com'`); n != 1 {
		t.Fatalf("identity rows: %d", n)
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_audit_event WHERE action='account.login_sso' AND outcome='Allowed' AND actor='ana@x.com'`); n != 1 {
		t.Fatalf("login audit: %d", n)
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_audit_event WHERE action='account.identity_link'`); n != 1 {
		t.Fatalf("link audit: %d", n)
	}

	// Linked by subject now: a new address at the provider still lands on the
	// same account, and is not linked again.
	_, sid = x.ssoLogin(idp, "", map[string]any{"email": "ana.new@elsewhere.com"})
	if u, _ := x.e.UserFromSession(x.ctx, sid); u != "ana@x.com" {
		t.Fatalf("by subject: %q", u)
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_audit_event WHERE action='account.identity_link'`); n != 1 {
		t.Fatalf("relinked: %d", n)
	}

	// Deleting the User takes the identity with it.
	x.asAdmin(func(c *engine.Ctx) error { return c.Delete("User", "ana@x.com", false, true) })
	if n := x.countSQL(`SELECT count(*) FROM ddcore_user_identity`); n != 0 {
		t.Fatalf("identity survived its user: %d", n)
	}
}

func TestSEC05_Refusals(t *testing.T) {
	x, idp := ssoEnv(t)
	cases := []struct {
		name, want string
		over       map[string]any
	}{
		{"unverified", "unverified_email", map[string]any{"email_verified": false}},
		{"verified missing", "unverified_email", map[string]any{"email_verified": nil}},
		{"no account", "no_account", map[string]any{"sub": "sub-x", "email": "nobody@x.com"}},
		{"wrong nonce", "state", map[string]any{"nonce": "other"}},
		{"wrong audience", "provider", map[string]any{"aud": "someone-else"}},
		{"wrong issuer", "provider", map[string]any{"iss": "https://evil.example"}},
		{"expired", "provider", map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			loc, sid := x.ssoLogin(idp, "/app", c.over)
			if sid != "" || loc != "/login?sso_error="+c.want {
				t.Fatalf("got %q sid=%q", loc, sid)
			}
		})
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_user WHERE name='nobody@x.com'`); n != 0 {
		t.Fatal("a user was provisioned")
	}
	if n := x.countSQL(`SELECT count(*) FROM tab_audit_event WHERE action='account.login_sso' AND outcome='Denied'`); n != len(cases) {
		t.Fatalf("denied audits: %d", n)
	}

	// disabled
	x.asAdmin(func(c *engine.Ctx) error { return c.SetValue("User", "ze@x.com", engine.Doc{"enabled": false}) })
	if loc, sid := x.ssoLogin(idp, "", map[string]any{"sub": "sub-ze", "email": "ze@x.com"}); sid != "" || loc != "/login?sso_error=disabled" {
		t.Fatalf("disabled: %q", loc)
	}

	t.Run("state", func(t *testing.T) {
		st := x.ssoStart("/app")
		code := engine.RandomToken()
		idp.grant(code, st, nil)
		q := "code=" + code + "&state=" + url.QueryEscape(st.state)
		// no cookie: the link alone is not enough
		if loc, sid := x.ssoCallback(q, nil); sid != "" || loc != "/login?sso_error=state" {
			t.Fatalf("without cookie: %q", loc)
		}
		// someone else's cookie
		other := x.ssoStart("/app")
		if loc, sid := x.ssoCallback(q, other.cookie); sid != "" || loc != "/login?sso_error=state" {
			t.Fatalf("foreign cookie: %q", loc)
		}
		// spent state
		st2 := x.ssoStart("/app")
		code2 := engine.RandomToken()
		idp.grant(code2, st2, nil)
		q2 := "code=" + code2 + "&state=" + url.QueryEscape(st2.state)
		if _, sid := x.ssoCallback(q2, st2.cookie); sid == "" {
			t.Fatal("first use refused")
		}
		idp.grant(code2, st2, nil)
		if loc, sid := x.ssoCallback(q2, st2.cookie); sid != "" || loc != "/login?sso_error=state" {
			t.Fatalf("replayed state: %q", loc)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		st := x.ssoStart("/app")
		if loc, sid := x.ssoCallback("error=access_denied&state="+url.QueryEscape(st.state), st.cookie); sid != "" || loc != "/login?sso_error=provider" {
			t.Fatalf("cancelled: %q", loc)
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		r := x.call("GET", "/api/auth/oidc/nope/start", nil, "")
		x.expect(r, 404, "")
	})
}

func TestSEC05_AllowedDomains(t *testing.T) {
	x, idp := ssoEnv(t, "y.com")
	if loc, sid := x.ssoLogin(idp, "", nil); sid != "" || loc != "/login?sso_error=domain" {
		t.Fatalf("outside the domain: %q", loc)
	}
}

func TestSEC05_OpenRedirect(t *testing.T) {
	x, idp := ssoEnv(t)
	for _, to := range []string{"//evil.com", "https://evil.com/x", "/\\evil.com", "javascript:alert(1)"} {
		if loc, sid := x.ssoLogin(idp, to, nil); sid == "" || loc != "/app" {
			t.Fatalf("%s: went to %q", to, loc)
		}
	}
}

func TestSEC05_PasswordLoginOff(t *testing.T) {
	x, _ := ssoEnv(t)
	off := false
	x.e.Cfg.Auth.PasswordLogin = &off

	r := x.call("POST", "/api/login", map[string]any{"usr": "ana@x.com", "pwd": "segredo123"}, "")
	x.expect(r, 401, "AuthenticationError")
	if _, err := x.e.Login(x.ctx, "Administrator", "admin12345", engine.LoginFrom{}); err != nil {
		t.Fatalf("Administrator keeps a password: %v", err)
	}

	before := x.countSQL(`SELECT count(*) FROM ddcore_auth_token`)
	x.expect(x.call("POST", "/api/auth/forgot-password", map[string]any{"usr": "ana@x.com"}, ""), 200, "")
	if n := x.countSQL(`SELECT count(*) FROM ddcore_auth_token`); n != before {
		t.Fatalf("a reset link was issued with password sign-in off")
	}

	r = x.call("GET", "/api/boot", nil, "")
	l := r.Body["data"].(map[string]any)["site"].(map[string]any)["login"].(map[string]any)
	ps := l["providers"].([]any)
	if l["password"] != false || len(ps) != 1 || ps[0].(map[string]any)["label"] != "Fake" {
		t.Fatalf("boot: %v", l)
	}
	if strings.Contains(r.Raw, "s3cret") || strings.Contains(r.Raw, "client-1") {
		t.Fatal("boot leaks provider credentials")
	}
}

// A sign-in that worked clears the address's failures, as a password sign-in
// does: an office that fumbled a few attempts must not stay near the limit.
func TestSEC05_SuccessClearsAddressThrottle(t *testing.T) {
	x, idp := ssoEnv(t)
	for range 2 {
		if loc, _ := x.ssoLogin(idp, "", map[string]any{"sub": "sub-x", "email": "nobody@x.com"}); loc != "/login?sso_error=no_account" {
			t.Fatalf("expected a refusal, got %q", loc)
		}
	}
	if n := x.countSQL(`SELECT count(*) FROM ddcore_login_attempt WHERE identity LIKE 'ssoip:%' AND NOT ok`); n != 2 {
		t.Fatalf("failures recorded against the address: %d, want 2", n)
	}

	if _, sid := x.ssoLogin(idp, "", nil); sid == "" {
		t.Fatal("the sign-in should have worked")
	}
	if n := x.countSQL(`SELECT count(*) FROM ddcore_login_attempt WHERE identity LIKE 'ssoip:%' AND NOT ok`); n != 0 {
		t.Errorf("a completed sign-in left %d failures against the address", n)
	}
}

// Our own failure is not the provider's fault: it gets a code of its own, so
// an operator reading the logs is not sent to the identity provider.
func TestSEC05_InternalFailureIsNotBlamedOnTheProvider(t *testing.T) {
	x, _ := ssoEnv(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/auth/oidc/idp/callback", nil)
	x.s.oidcFail(w, r, errors.New("the database went away"))
	if loc := w.Header().Get("Location"); loc != "/login?sso_error=server" {
		t.Errorf("Location = %q, want /login?sso_error=server", loc)
	}
}
