package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The header a deployment names comes first; without a valid address in it,
// chi's order (True-Client-IP, X-Real-IP, X-Forwarded-For) decides.
func TestRealIPReadsTheNamedHeaderFirst(t *testing.T) {
	cases := []struct {
		name, header string
		in           map[string]string
		want         string
	}{
		{"behind Cloudflare and Traefik", "CF-Connecting-IP",
			map[string]string{"CF-Connecting-IP": "189.115.252.117", "X-Real-IP": "172.64.222.38", "X-Forwarded-For": "6.6.6.6, 172.64.222.38"},
			"189.115.252.117"},
		{"an IPv6 visitor", "CF-Connecting-IP",
			map[string]string{"CF-Connecting-IP": "2804:1b3:8101::1", "X-Real-IP": "172.64.222.38"},
			"2804:1b3:8101::1"},
		{"a list keeps its first address", "CF-Connecting-IP",
			map[string]string{"CF-Connecting-IP": " 189.115.252.117 , 10.0.0.1"},
			"189.115.252.117"},
		{"missing: reached the inner proxy directly", "CF-Connecting-IP",
			map[string]string{"X-Real-IP": "200.1.2.3"},
			"200.1.2.3"},
		{"not an address", "CF-Connecting-IP",
			map[string]string{"CF-Connecting-IP": "unknown", "X-Real-IP": "200.1.2.3"},
			"200.1.2.3"},
		{"no header named: chi's order", "",
			map[string]string{"CF-Connecting-IP": "189.115.252.117", "X-Real-IP": "172.64.222.38"},
			"172.64.222.38"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got string
			h := realIP(c.header)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = clientIP(r) }))
			r := httptest.NewRequest("GET", "/api/ready", nil)
			r.RemoteAddr = "10.0.1.2:41000"
			for k, v := range c.in {
				r.Header.Set(k, v)
			}
			h.ServeHTTP(httptest.NewRecorder(), r)
			if got != c.want {
				t.Errorf("client IP = %q, want %q", got, c.want)
			}
		})
	}
}
