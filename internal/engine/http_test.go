package engine

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

func TestHTTPVerbs(t *testing.T) {
	for _, tc := range []struct{ name, verb, args, body, contentType string }{
		{"get", "GET", `, opts`, "", ""},
		{"post", "POST", `, {amount: 42}, opts`, `{"amount":42}`, "application/json"},
		{"put", "PUT", `, {amount: 42}, opts`, `{"amount":42}`, "application/json"},
		{"patch", "PATCH", `, "amount=42", opts`, "amount=42", ""},
		{"del", "DELETE", `, opts`, "", ""},
		{"post", "POST", ``, "", ""},
		{"put", "PUT", ``, "", ""},
		{"patch", "PATCH", ``, "", ""},
		{"get", "GET", ``, "", ""},
		{"del", "DELETE", ``, "", ""},
	} {
		t.Run(tc.name+tc.args, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if r.Method != tc.verb || r.URL.Path != "/payments/1" || string(body) != tc.body {
					t.Errorf("request: %s %s %q", r.Method, r.URL.Path, body)
				}
				if got := r.Header.Get("Content-Type"); got != tc.contentType {
					t.Errorf("content type: %q", got)
				}
				if tc.args != "" && r.Header.Get("Authorization") != "Bearer test" {
					t.Error("missing authorization header")
				}
				w.Header().Set("RateLimit-Remaining", "9")
				w.WriteHeader(http.StatusAccepted)
				fmt.Fprint(w, `{"ok":true}`)
			}))
			defer server.Close()
			pool, err := js.NewPool(&Engine{}, nil, 1, false)
			if err != nil {
				t.Fatal(err)
			}
			rt, err := pool.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Release()
			rt.Ctx = &Ctx{}
			for _, method := range []string{"", ", method: \"HEAD\""} {
				code := fmt.Sprintf(`const opts = {headers: {Authorization: "Bearer test"}, timeout: 2%s};
const response = ddcore.http.%s(%q%s);
if (response.status !== 202 || response.body !== '{"ok":true}' || response.json().ok !== true || response.headers["Ratelimit-Remaining"] !== "9") throw new Error("unexpected response");`, method, tc.name, server.URL+"/payments/1", tc.args)
				if _, err := rt.Eval(code); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// A binary body survives as base64, and a body above the limit is an error
// rather than a silent truncation.
func TestHTTPBinaryAndLimit(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff, 0xfe, 0x80}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	}))
	defer server.Close()
	pool, err := js.NewPool(&Engine{}, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	rt.Ctx = &Ctx{}
	want := base64.StdEncoding.EncodeToString(png)
	v, err := rt.Eval(fmt.Sprintf(`ddcore.http.get(%q, {responseType: "base64"}).body`, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != `"`+want+`"` {
		t.Fatalf("base64 body %q, want %q", v.String(), want)
	}
	if _, err := rt.Eval(fmt.Sprintf(`ddcore.http.get(%q, {maxBytes: %d})`, server.URL, len(png))); err != nil {
		t.Fatalf("a body of exactly maxBytes is fine: %v", err)
	}
	_, err = rt.Eval(fmt.Sprintf(`ddcore.http.get(%q, {maxBytes: %d})`, server.URL, len(png)-1))
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("want a size error, got %v", err)
	}
	_, err = rt.Eval(fmt.Sprintf(`ddcore.http.get(%q, {responseType: "blob"})`, server.URL))
	if err == nil || !strings.Contains(err.Error(), "responseType") {
		t.Fatalf("want a responseType error, got %v", err)
	}
}

func TestHTTPRequestBodies(t *testing.T) {
	audio := []byte{0x4f, 0x67, 0x67, 0x53, 0x00, 0xff, 0xfe, 0x80, 0xc3, 0x28}
	b64 := base64.StdEncoding.EncodeToString(audio)
	var raw []byte
	var ctype string
	var form *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		ctype = r.Header.Get("Content-Type")
		if strings.HasPrefix(ctype, "multipart/") {
			r.Body = io.NopCloser(strings.NewReader(string(raw)))
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			form = r
		}
	}))
	defer server.Close()
	pool, err := js.NewPool(&Engine{}, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	rt.Ctx = &Ctx{}
	t.Run("base64", func(t *testing.T) {
		raw, ctype = nil, ""
		if _, err := rt.Eval(fmt.Sprintf(`ddcore.http.post(%q, %q, {bodyEncoding: "base64"})`, server.URL, b64)); err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(audio) || ctype != "application/octet-stream" {
			t.Errorf("got %x as %q", raw, ctype)
		}
		if _, err := rt.Eval(fmt.Sprintf(`ddcore.http.post(%q, %q, {bodyEncoding: "base64", headers: {"content-type": "audio/ogg"}})`, server.URL, b64)); err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(audio) || ctype != "audio/ogg" {
			t.Errorf("got %x as %q", raw, ctype)
		}
	})

	t.Run("multipart", func(t *testing.T) {
		form = nil
		src := fmt.Sprintf(`ddcore.http.post(%q, [
			{name: "model", value: "whisper"},
			{name: "file", filename: "a.ogg", contentType: "audio/ogg", base64: %q},
			{name: "raw", base64: %q},
		], {bodyEncoding: "multipart", headers: {"Content-Type": "application/json"}})`, server.URL, b64, b64)
		if _, err := rt.Eval(src); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(ctype, "multipart/form-data; boundary=") {
			t.Fatalf("content type %q", ctype)
		}
		if form == nil {
			t.Fatal("no multipart form")
		}
		if got := form.MultipartForm.Value["model"]; len(got) != 1 || got[0] != "whisper" {
			t.Errorf("model: %v", got)
		}
		for name, wantType := range map[string]string{"file": "audio/ogg", "raw": "application/octet-stream"} {
			fhs := form.MultipartForm.File[name]
			if len(fhs) != 1 {
				t.Fatalf("part %s: %v", name, fhs)
			}
			f, _ := fhs[0].Open()
			data, _ := io.ReadAll(f)
			f.Close()
			if string(data) != string(audio) || fhs[0].Header.Get("Content-Type") != wantType {
				t.Errorf("part %s: %x as %q", name, data, fhs[0].Header.Get("Content-Type"))
			}
		}
		if got := form.MultipartForm.File["file"][0].Filename; got != "a.ogg" {
			t.Errorf("filename %q", got)
		}
		if got := form.MultipartForm.File["raw"][0].Filename; got != "raw" {
			t.Errorf("default filename %q", got)
		}
	})

	t.Run("a JSON body with a multipart key stays JSON", func(t *testing.T) {
		if _, err := rt.Eval(fmt.Sprintf(`ddcore.http.post(%q, {multipart: [1]})`, server.URL)); err != nil {
			t.Fatal(err)
		}
		if string(raw) != `{"multipart":[1]}` || ctype != "application/json" {
			t.Errorf("got %q as %q", raw, ctype)
		}
	})

	for name, tc := range map[string]struct{ src, want string }{
		"unknown encoding":  {`"x", {bodyEncoding: "gzip"}`, "bodyEncoding"},
		"bad base64":        {`"@@@", {bodyEncoding: "base64"}`, "base64"},
		"base64 non string": {`{a: 1}, {bodyEncoding: "base64"}`, "base64"},
		"no name":           {`[{value: "x"}], {bodyEncoding: "multipart"}`, "no name"},
		"value and base64":  {`[{name: "f", value: "x", base64: "AA=="}], {bodyEncoding: "multipart"}`, "both"},
		"non string value":  {`[{name: "f", value: 3}], {bodyEncoding: "multipart"}`, "must be a string"},
		"not an array":      {`{name: "f"}, {bodyEncoding: "multipart"}`, "array of parts"},
		"part bad base64":   {`[{name: "f", base64: "@@"}], {bodyEncoding: "multipart"}`, "not valid base64"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := rt.Eval(fmt.Sprintf(`ddcore.http.post(%q, %s)`, server.URL, tc.src))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

// evalHTTP runs code in a runtime with no database, as the other HTTP tests do.
func evalHTTP(t *testing.T, code string) (any, error) {
	t.Helper()
	pool, err := js.NewPool(&Engine{}, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	rt.Ctx = &Ctx{}
	return rt.Eval(code)
}

// A redirect to another host must not carry the caller's credential headers:
// net/http only drops Authorization and Cookie, so a token sent as a custom
// header reached object storage behind Chatwoot's Active Storage redirect.
func TestHTTPRedirectDropsCallerHeadersAcrossHosts(t *testing.T) {
	var got http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		fmt.Fprint(w, "blob")
	}))
	defer target.Close()
	var sameHost http.Header
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, target.URL+"/blob", http.StatusFound)
		case "/here":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			sameHost = r.Header.Clone()
			fmt.Fprint(w, "ok")
		}
	}))
	defer origin.Close()

	opts := `{headers: {api_access_token: "s3cr3t", "X-Api-Key": "k", Accept: "audio/ogg"}}`
	if _, err := evalHTTP(t, fmt.Sprintf(`if (ddcore.http.get(%q, %s).body !== "blob") throw new Error("not followed")`, origin.URL+"/away", opts)); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"Api_access_token", "X-Api-Key", "Accept"} {
		if v := got.Get(h); v != "" {
			t.Errorf("the other host received %s: %q", h, v)
		}
	}
	if got.Get("User-Agent") == "" {
		t.Error("the other host lost the User-Agent")
	}
	if _, err := evalHTTP(t, fmt.Sprintf(`ddcore.http.get(%q, %s)`, origin.URL+"/here", opts)); err != nil {
		t.Fatal(err)
	}
	if sameHost.Get("Api_access_token") != "s3cr3t" || sameHost.Get("X-Api-Key") != "k" {
		t.Errorf("a same-host redirect lost the headers: %v", sameHost)
	}
}

// maxRedirects: 0 hands the 3xx back so the app decides where to go and with
// which headers.
func TestHTTPMaxRedirects(t *testing.T) {
	hops := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/end" {
			fmt.Fprint(w, "end")
			return
		}
		hops++
		http.Redirect(w, r, "/end", http.StatusFound)
	}))
	defer server.Close()
	code := fmt.Sprintf(`const r = ddcore.http.get(%q, {maxRedirects: 0});
if (r.status !== 302 || r.headers.Location !== "/end") throw new Error("got " + r.status + " " + JSON.stringify(r.headers));
if (ddcore.http.get(%[1]q).body !== "end") throw new Error("the default no longer follows");`, server.URL+"/start")
	if _, err := evalHTTP(t, code); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"-1", "1.5"} {
		_, err := evalHTTP(t, fmt.Sprintf(`ddcore.http.get(%q, {maxRedirects: %s})`, server.URL, bad))
		if err == nil || !strings.Contains(err.Error(), "maxRedirects") {
			t.Errorf("maxRedirects %s: err = %v", bad, err)
		}
	}
}
