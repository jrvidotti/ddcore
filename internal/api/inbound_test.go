package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const diag = "/api/method/demo.services.diag."

// rawCall is call with a body sent as given, for a payload that is not JSON or
// whose exact bytes matter.
func (x *env) rawCall(method, path, body, contentType string, hdr ...string) resp {
	x.t.Helper()
	req, _ := http.NewRequest(method, x.ts.URL+path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw), Header: res.Header}
	json.Unmarshal(raw, &out.Body)
	return out
}

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

// A guest webhook answers the status its error type names, with the wait in
// Retry-After and extra in the body (#89).
func TestThrowAnswersTheTypeStatus(t *testing.T) {
	x := setup(t)
	r := x.call("POST", diag+"notYet", nil, "")
	x.expect(r, 503, "UnavailableError")
	if got := r.Header.Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
	e := errorBody(t, r)
	extra, _ := e["extra"].(map[string]any)
	if extra["code"] != "NOT_CONFIGURED" || extra["retryAfter"] != float64(60) || e["message"] != "Not configured yet" {
		t.Fatalf("error body = %v", e)
	}
	if e["requestId"] == "" || e["requestId"] == nil {
		t.Fatalf("no requestId in %v", e)
	}

	r = x.call("POST", diag+"busy", nil, "")
	x.expect(r, 429, "ValidationError")
	if got := r.Header.Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q with no retryAfter", got)
	}
	x.expect(x.call("GET", diag+"forbidden", nil, ""), 403, "PermissionError")
	// an app's own type is an error nobody anticipated
	x.expect(x.call("POST", diag+"unknownType", nil, ""), 500, "PaymentDeclinedError")
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
	// no `methods` declared: any verb, as before
	x.expect(x.call("GET", diag+"inspect", nil, ""), 200, "")
	x.expect(x.call("POST", diag+"inspect", nil, ""), 200, "")
}

func TestRequestCarriesTheRawBodyAndHeaders(t *testing.T) {
	x := setup(t)
	const body = "{ \"a\" :  1,\n \"b\":[1, 2] }"
	r := x.rawCall("POST", diag+"inspect", body, "application/json", "X-Custom-Thing", "v1")
	x.expect(r, 200, "")
	d := r.Body["data"].(map[string]any)
	if d["body"] != body {
		t.Fatalf("rawBody = %q, want the bytes as sent", d["body"])
	}
	if d["args"].(map[string]any)["a"] != float64(1) {
		t.Fatalf("args were not parsed: %v", d["args"])
	}
	h := d["headers"].(map[string]any)
	if h["x-custom-thing"] != "v1" {
		t.Fatalf("headers = %v", h)
	}
	for _, k := range []string{"cookie", "authorization"} {
		if _, ok := h[k]; ok {
			t.Fatalf("%s must not reach app code", k)
		}
	}
}

func TestSessionCredentialsStayOutOfHeaders(t *testing.T) {
	x := setup(t)
	r := x.call("POST", diag+"inspect", nil, "sid:"+x.sid("ana@x.com"))
	x.expect(r, 200, "")
	h := r.Body["data"].(map[string]any)["headers"].(map[string]any)
	if _, ok := h["cookie"]; ok {
		t.Fatalf("the session cookie reached app code: %v", h)
	}
}

func TestNonJSONBodyIsNotAnError(t *testing.T) {
	x := setup(t)
	r := x.rawCall("POST", diag+"inspect", "entry=1&x=2", "application/x-www-form-urlencoded")
	x.expect(r, 200, "")
	if r.Body["data"].(map[string]any)["body"] != "entry=1&x=2" {
		t.Fatalf("rawBody lost: %s", r.Raw)
	}
	// a caller that says it sends JSON is still told when it does not
	x.expect(x.rawCall("POST", diag+"inspect", "{nope", "application/json"), 417, "ValidationError")
}

func TestInboundSignatureCanBeVerified(t *testing.T) {
	x := setup(t)
	const body = `{"entry":[{"id":"1"}]}`
	mac := hmac.New(sha256.New, []byte("shh"))
	mac.Write([]byte(body))
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	r := x.rawCall("POST", diag+"verifySig", body, "application/json", "X-Hub-Signature-256", good)
	x.expect(r, 200, "")
	if r.Body["data"] != true {
		t.Fatalf("a valid signature was refused: %s", r.Raw)
	}
	r = x.rawCall("POST", diag+"verifySig", body+" ", "application/json", "X-Hub-Signature-256", good)
	if r.Body["data"] != false {
		t.Fatalf("a signature over other bytes was accepted: %s", r.Raw)
	}
	r = x.rawCall("POST", diag+"verifySig", body, "application/json")
	if r.Body["data"] != false {
		t.Fatalf("a missing signature was accepted: %s", r.Raw)
	}
}

// A provider that appends to the URL it was registered with (a bank posting
// to "<url>/pix") reaches a method that opts into pathTail, which reads what
// came after its own path, percent-decoded.
func TestPathTailReachesAnOptedInMethod(t *testing.T) {
	x := setup(t)
	for path, want := range map[string]string{
		"hook":                     "",
		"hook/":                    "",
		"hook/pix":                 "pix",
		"hook/pix/2024/10":         "pix/2024/10",
		"hook/pix/a%20b%C3%A9%40x": "pix/a bé@x",
	} {
		r := x.rawCall("POST", diag+path, `{"k":1}`, "application/json")
		x.expect(r, 200, "")
		d := r.Body["data"].(map[string]any)
		if d["tail"] != want {
			t.Fatalf("%s: pathTail = %#v, want %q", path, d["tail"], want)
		}
		if d["args"].(map[string]any)["k"] != float64(1) {
			t.Fatalf("%s: args lost: %v", path, d["args"])
		}
	}
	r := x.call("GET", diag+"hook/pix?a=1", nil, "")
	x.expect(r, 200, "")
	if d := r.Body["data"].(map[string]any); d["tail"] != "pix" || d["args"].(map[string]any)["a"] != "1" {
		t.Fatalf("GET with a tail: %s", r.Raw)
	}
}

// A method that did not opt in answers a sub-path exactly as it did before:
// the method does not exist there.
func TestPathTailIsRefusedWithoutTheOptIn(t *testing.T) {
	x := setup(t)
	x.expect(x.rawCall("POST", diag+"inspect/pix", "", ""), 404, "DoesNotExistError")
	x.expect(x.call("GET", diag+"inspect/pix", nil, ""), 404, "DoesNotExistError")
	r := x.call("POST", diag+"inspect", nil, "")
	x.expect(r, 200, "")
	if strings.Contains(r.Raw, "pathTail") {
		t.Fatalf("pathTail reached a method that did not ask for it: %s", r.Raw)
	}
}
