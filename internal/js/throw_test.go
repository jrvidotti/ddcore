package js

import (
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// Every type cerr defines answers its own status from the JS runtime, not a
// 500 (#89); a type nobody defined is still a 500.
func TestStatusForCoversEveryCerrType(t *testing.T) {
	want := map[string]int{
		"ValidationError": 417, "MandatoryError": 417, "LinkExistsError": 417,
		"PermissionError": 403, "DoesNotExistError": 404, "NotFound": 404,
		"TimestampMismatchError": 409, "DuplicateEntryError": 409, "AuthenticationError": 401,
		"MethodNotAllowedError": 405, "TooManyRequestsError": 429, "UnavailableError": 503,
		"MaintenanceError": 503, "InternalError": 500, "PaymentDeclinedError": 500, "": 500,
	}
	for typ, n := range want {
		if got := statusFor(typ); got != n {
			t.Errorf("statusFor(%q) = %d, want %d", typ, got, n)
		}
	}
}

func thrown(t *testing.T, rt *Runtime, code string) *cerr.Error {
	t.Helper()
	_, err := rt.Eval(code)
	var e *cerr.Error
	if !errors.As(err, &e) {
		t.Fatalf("%s: expected a *cerr.Error, got %v", code, err)
	}
	return e
}

func TestThrowCarriesStatusExtraAndRetryAfter(t *testing.T) {
	rt, err := newRuntime(&fakeHost{}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		code   string
		typ    string
		status int
		extra  any
		retry  int // 0: no Retry-After
	}{
		{`ddcore.throw("no")`, "ValidationError", 417, nil, 0},
		{`ddcore.throw("later", { type: "UnavailableError", retryAfter: 60 })`, "UnavailableError", 503,
			map[string]any{"retryAfter": int64(60)}, 60},
		{`ddcore.throw("slow", { type: "TooManyRequestsError", retryAfter: 1.5, extra: { code: "RATE" } })`, "TooManyRequestsError", 429,
			map[string]any{"code": "RATE", "retryAfter": int64(2)}, 2},
		{`ddcore.throw("bad", { status: 422, extra: { code: "E42", fields: ["a"] } })`, "ValidationError", 422,
			map[string]any{"code": "E42", "fields": []any{"a"}}, 0},
		{`ddcore.throw("busy", { status: 429 })`, "ValidationError", 429, nil, 0},
		// out of range or not a number: the type's status stands
		{`ddcore.throw("ok?", { status: 200 })`, "ValidationError", 417, nil, 0},
		{`ddcore.throw("ok?", { type: "PermissionError", status: "503" })`, "PermissionError", 403, nil, 0},
		{`ddcore.throw("ok?", { status: 503.5 })`, "ValidationError", 417, nil, 0},
		{`ddcore.throw("mine", { type: "PaymentDeclinedError" })`, "PaymentDeclinedError", 500, nil, 0},
		// a caught error rethrown keeps what it was thrown with
		{`try { ddcore.throw("x", { type: "PermissionError", status: 451 }) } catch (e) { throw e }`, "PermissionError", 451, nil, 0},
	}
	for _, c := range cases {
		e := thrown(t, rt, c.code)
		if e.Type != c.typ || e.Status != c.status || !reflect.DeepEqual(e.Extra, c.extra) {
			t.Errorf("%s: got %s %d %#v, want %s %d %#v", c.code, e.Type, e.Status, e.Extra, c.typ, c.status, c.extra)
		}
		if n, ok := e.RetryAfter(); n != c.retry || ok != (c.retry != 0) {
			t.Errorf("%s: RetryAfter = %d, %v; want %d", c.code, n, ok, c.retry)
		}
	}
}

// A Go error raised by a host op inside app code comes out of the runtime with
// the status and the wait it went in with: a paused write stays a 503 with its
// Retry-After, not a 500 (#89).
func TestGoErrorCrossingJSKeepsItsStatus(t *testing.T) {
	h := &fakeHost{}
	rt, err := newRuntime(h, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cases := []*cerr.Error{
		cerr.Maintenance("paused").WithRetryAfter(60),
		cerr.TooMany("slow down").WithRetryAfter(9),
		cerr.Unavailable("PDF renderer down"),
		cerr.MethodNotAllowed("GET only"),
		cerr.Duplicate("taken"),
		cerr.New("ValidationError", 422, "custom status"),
	}
	for _, in := range cases {
		h.fail = in
		for _, code := range []string{
			`ddcore.db.getValue("X", "1", "a")`,
			// caught and rethrown by app code
			`try { ddcore.db.getValue("X", "1", "a") } catch (e) { throw e }`,
			// the bridge called bare, past the prelude's toError: the
			// message still carries the JSON, status included
			`__host("db.getValue", "{}")`,
		} {
			e := thrown(t, rt, code)
			if e.Type != in.Type || e.Status != in.Status || e.Message != in.Message {
				t.Errorf("%s through %s: got %s %d %q", in.Type, code, e.Type, e.Status, e.Message)
			}
			wantN, wantOK := in.RetryAfter()
			if n, ok := e.RetryAfter(); n != wantN || ok != wantOK {
				t.Errorf("%s through %s: RetryAfter = %d, %v; want %d, %v", in.Type, code, n, ok, wantN, wantOK)
			}
		}
	}
}

// The prelude's own errors use types the status table knows: a missing mail or
// print template answers 404, not the 500 of a type nobody defined.
func TestPreludeThrowsOnlyKnownTypes(t *testing.T) {
	for _, m := range regexp.MustCompile(`new DDCoreError\("([A-Za-z]+)"`).FindAllStringSubmatch(prelude, -1) {
		if _, ok := cerr.StatusOf(m[1]); !ok {
			t.Errorf("prelude throws %s, which has no status", m[1])
		}
	}
}
