package engine

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
