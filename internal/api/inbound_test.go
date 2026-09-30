package api

import (
	"strings"
	"testing"
)

const diag = "/api/method/demo.services.diag."

func TestRawResponseIsTheBodyWithNoEnvelope(t *testing.T) {
	x := setup(t)
	r := x.call("GET", diag+"challenge?hub.mode=subscribe&hub.challenge=1158201444", nil, "")
	x.expect(r, 200, "")
	if r.Raw != "1158201444" {
		t.Fatalf("body = %q, want the bare challenge", r.Raw)
	}
	if ct := r.Header.Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("content type = %q", ct)
	}
}

func TestRawResponseErrorsStayJSON(t *testing.T) {
	x := setup(t)
	r := x.call("GET", diag+"denied", nil, "")
	if r.Status < 400 || !strings.Contains(r.Header.Get("Content-Type"), "json") {
		t.Fatalf("a thrown error must be a JSON error, got %d %s %q", r.Status, r.Header.Get("Content-Type"), r.Raw)
	}
	r = x.call("GET", diag+"notString", nil, "")
	x.expect(r, 500, "InternalError")
}

func TestMethodsIsEnforced(t *testing.T) {
	x := setup(t)
	r := x.call("POST", diag+"challenge", nil, "")
	x.expect(r, 405, "MethodNotAllowedError")
	if got := r.Header.Get("Allow"); got != "GET" {
		t.Fatalf("Allow = %q", got)
	}
	x.expect(x.call("GET", diag+"postOnly", nil, ""), 405, "MethodNotAllowedError")
	x.expect(x.call("POST", diag+"postOnly", nil, ""), 200, "")
}
