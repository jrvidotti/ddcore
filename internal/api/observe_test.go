package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// errorBody reads the error envelope, which is where a caller finds the id.
func errorBody(t *testing.T, r resp) map[string]any {
	t.Helper()
	e, ok := r.Body["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in %s", r.Raw)
	}
	return e
}

func TestPRD03_RequestIDIsGeneratedAndEchoed(t *testing.T) {
	x := setup(t)
	first := x.call("GET", "/api/health", nil, "").Header.Get(requestIDHeader)
	second := x.call("GET", "/api/health", nil, "").Header.Get(requestIDHeader)
	if len(first) != 32 || len(second) != 32 {
		t.Fatalf("expected 32 hex characters, got %q and %q", first, second)
	}
	if first == second {
		t.Fatalf("two requests share the id %q", first)
	}
}

func TestPRD03_InboundRequestIDIsHonoured(t *testing.T) {
	x := setup(t)
	const mine = "01JB8Z9K7Q-trace.1"
	got := x.call("GET", "/api/health", nil, "", requestIDHeader, mine).Header.Get(requestIDHeader)
	if got != mine {
		t.Fatalf("a well-formed inbound id must come back unchanged: sent %q, got %q", mine, got)
	}
}

// The inbound value reaches a log line, a response header and a text column.
// A newline in it forges a log entry and a megabyte of it floods the column, so
// anything outside the charset is replaced rather than escaped.
//
// The injection cases are unit-tested rather than sent over HTTP because Go's
// own client refuses to put a control character in a header — which is a
// second line of defence, not this one, and not one the server may rely on.
func TestPRD03_HostileRequestIDIsReplaced(t *testing.T) {
	for name, hostile := range map[string]string{
		"newline":   "abcdefgh\nlevel=ERROR msg=\"forged\"",
		"carriage":  "abcdefgh\r\nSet-Cookie: sid=x",
		"nul":       "abcdefgh\x00zzz",
		"too long":  strings.Repeat("a", 5000),
		"too short": "abc",
		"space":     "abcd efgh",
		"quote":     `abcdefgh"zzz`,
		"empty":     "",
	} {
		if got := sanitizeRequestID(hostile); got != "" {
			t.Fatalf("%s: sanitize kept %q", name, got)
		}
	}
	for name, ok := range map[string]string{
		"ulid":        "01JB8Z9K7QW3MFA1XN2R4T5V6Y",
		"uuid":        "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"hex":         "eb0ebd4f29e3122d5b6d98bcd9771b70",
	} {
		if got := sanitizeRequestID(ok); got != ok {
			t.Fatalf("%s: sanitize rejected a well-formed id, got %q", name, got)
		}
	}
}

// And over the wire: a value the client is willing to send but the server is
// not willing to repeat is replaced, not echoed.
func TestPRD03_OversizedRequestIDIsReplacedOverHTTP(t *testing.T) {
	x := setup(t)
	for name, hostile := range map[string]string{
		"too long":  strings.Repeat("a", 5000),
		"too short": "abc",
		"space":     "abcd efgh",
	} {
		got := x.call("GET", "/api/health", nil, "", requestIDHeader, hostile).Header.Get(requestIDHeader)
		if got == hostile {
			t.Fatalf("%s: the hostile id was repeated verbatim", name)
		}
		if len(got) != 32 {
			t.Fatalf("%s: expected a generated id, got %q", name, got)
		}
	}
}

func TestPRD03_ErrorCarriesTheRequestID(t *testing.T) {
	x := setup(t)
	r := x.call("GET", "/api/resource/Pessoa/nope", nil, "sid:"+x.sid("ana@x.com"))
	x.expect(r, 404, "DoesNotExistError")
	header := r.Header.Get(requestIDHeader)
	if got := errorBody(t, r)["requestId"]; got != header {
		t.Fatalf("the body says %v, the header says %q", got, header)
	}
}

// chi allows a route to be added after the router is built; only a late Use
// panics. That is what lets this test reach the panic path without a handler
// that could panic in production.
func TestPRD03_PanicBecomesAJSONErrorAndAnErrorLogRow(t *testing.T) {
	x := setup(t)
	x.s.Router.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("the secret is hunter2")
	})
	r := x.call("GET", "/boom", nil, "")
	x.expect(r, 500, "InternalError")
	if strings.Contains(r.Raw, "hunter2") {
		t.Fatalf("the panic text reached the client: %s", r.Raw)
	}
	id := r.Header.Get(requestIDHeader)
	if got := errorBody(t, r)["requestId"]; got != id {
		t.Fatalf("the panic response carries %v, the header says %q", got, id)
	}
	var rows []map[string]any
	x.asAdmin(func(c *engine.Ctx) error {
		var err error
		rows, err = c.GetList("Error Log", engine.ListArgs{
			Fields: []string{"method", "error", "request_id"}, OrderBy: "creation desc", Limit: 5})
		return err
	})
	for _, row := range rows {
		if row["request_id"] == id {
			if !strings.Contains(fmt.Sprint(row["error"]), "hunter2") {
				t.Fatalf("the Error Log row lost the panic text: %v", row)
			}
			return // the row and the client agree on the id, which is the point
		}
	}
	t.Fatalf("no Error Log row carries the id %q: %v", id, rows)
}

// A 500 is the error the caller cannot act on, so it earns a row keyed by the
// id they were shown. A 4xx is their own doing and is already in the access log.
func TestPRD03_ClientErrorsDoNotFillTheErrorLog(t *testing.T) {
	x := setup(t)
	before := x.errorLogCount()
	x.call("GET", "/api/resource/Pessoa/nope", nil, "sid:"+x.sid("ana@x.com"))
	x.call("GET", "/api/boot", nil, "token:garbage:garbage")
	if after := x.errorLogCount(); after != before {
		t.Fatalf("client errors wrote %d Error Log row(s)", after-before)
	}
}

func (x *env) errorLogCount() int64 {
	x.t.Helper()
	var n int64
	x.asAdmin(func(c *engine.Ctx) error {
		var err error
		n, err = c.Count("Error Log", nil)
		return err
	})
	return n
}

func TestPRD03_TheRequestIDReachesTheAppRuntime(t *testing.T) {
	x := setup(t)
	const mine = "runtime-probe-01"
	r := x.call("POST", "/api/method/demo.services.diag.whoami", nil,
		"sid:"+x.sid("ana@x.com"), requestIDHeader, mine)
	x.expect(r, 200, "")
	b, _ := json.Marshal(r.Body["data"])
	if string(b) != `"`+mine+`"` {
		t.Fatalf("app code saw %s, the caller sent %q", b, mine)
	}
}

// One desk page is around thirty requests through this same chain, so the level
// policy is what keeps the handful of lines that say something from being
// buried. It is pure, and cheap to pin.
func TestPRD03_AccessLogLevelKeepsTheNoiseDown(t *testing.T) {
	const slow = 2 * time.Second
	fast := 10 * time.Millisecond
	for name, tc := range map[string]struct {
		path   string
		status int
		d      time.Duration
		want   slog.Level
	}{
		"api call":       {"/api/resource/Pessoa", 200, fast, slog.LevelInfo},
		"asset":          {"/assets/apps/demo/desk.js", 200, fast, slog.LevelDebug},
		"desk fallback":  {"/app/task/TASK-001", 200, fast, slog.LevelDebug},
		"probe":          {"/readyz", 200, fast, slog.LevelDebug},
		"failing probe":  {"/readyz", 503, fast, slog.LevelError},
		"client error":   {"/api/resource/Pessoa/nope", 404, fast, slog.LevelWarn},
		"server error":   {"/api/method/x", 500, fast, slog.LevelError},
		"slow asset":     {"/assets/apps/demo/desk.js", 200, 5 * time.Second, slog.LevelWarn},
		"long-lived SSE": {"/api/events", 200, time.Hour, slog.LevelInfo},
	} {
		if got := logLevelFor(tc.path, tc.status, tc.d, slow); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}
