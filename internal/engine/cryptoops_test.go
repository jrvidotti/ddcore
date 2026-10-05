package engine

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/js"
)

func cryptoRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	pool, err := js.NewPool(&Engine{}, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Release)
	rt.Ctx = &Ctx{}
	return rt
}

func evalStr(t *testing.T, rt *js.Runtime, expr string) string {
	t.Helper()
	v, err := rt.Eval(expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	return strings.Trim(v.String(), `"`)
}

func TestCryptoSha256(t *testing.T) {
	rt := cryptoRuntime(t)
	const abc = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := evalStr(t, rt, `ddcore.crypto.sha256("abc")`); got != abc {
		t.Fatalf("hex = %s", got)
	}
	if got := evalStr(t, rt, `ddcore.crypto.sha256("abc", {output: "base64"})`); got != "ungWv48Bz+pBQUDeXa4iI7ADYaOWF3qctBD/YfIAFa0=" {
		t.Fatalf("base64 = %s", got)
	}
	if got := evalStr(t, rt, `ddcore.crypto.sha256("abc", {output: "base64url"})`); got != "ungWv48Bz-pBQUDeXa4iI7ADYaOWF3qctBD_YfIAFa0" {
		t.Fatalf("base64url = %s", got)
	}
	if _, err := rt.Eval(`ddcore.crypto.sha256("abc", {output: "bin"})`); err == nil {
		t.Fatal("an unknown output should throw")
	}
}

func TestCryptoHmacSha256Options(t *testing.T) {
	rt := cryptoRuntime(t)
	// The Standard Webhooks vector: the whsec_ key decoded, base64 out.
	want := "g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
	expr := `ddcore.crypto.hmacSha256("MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw", "msg_p5jXN8AQM9LWM0D4loKWxJek.1614265330.{\"test\": 2432232314}", {keyEncoding: "base64", output: "base64"})`
	if got := evalStr(t, rt, expr); got != want {
		t.Fatalf("got %s", got)
	}
	// "key" in hex and in UTF-8 are the same key.
	a := evalStr(t, rt, `ddcore.crypto.hmacSha256("6b6579", "x", {keyEncoding: "hex"})`)
	b := evalStr(t, rt, `ddcore.crypto.hmacSha256("key", "x")`)
	if a != b || len(a) != 64 {
		t.Fatalf("hex key %s vs utf8 %s", a, b)
	}
	if _, err := rt.Eval(`ddcore.crypto.hmacSha256("%%%", "x", {keyEncoding: "base64"})`); err == nil {
		t.Fatal("a bad key should throw")
	}
}

func TestRandomToken(t *testing.T) {
	rt := cryptoRuntime(t)
	re := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	if tok := evalStr(t, rt, `ddcore.crypto.randomToken()`); len(tok) != 43 || !re.MatchString(tok) {
		t.Fatalf("default token = %q", tok)
	}
	if tok := evalStr(t, rt, `ddcore.crypto.randomToken(16)`); len(tok) != 22 || !re.MatchString(tok) {
		t.Fatalf("16-byte token = %q", tok)
	}
	if evalStr(t, rt, `ddcore.crypto.randomToken(32)`) == evalStr(t, rt, `ddcore.crypto.randomToken(32)`) {
		t.Fatal("two tokens were equal")
	}
	for _, bad := range []string{"0", "-1", "1.5", "100000"} {
		if _, err := rt.Eval(`ddcore.crypto.randomToken(` + bad + `)`); err == nil {
			t.Fatalf("randomToken(%s) should throw", bad)
		}
	}
}

func TestRandomInt(t *testing.T) {
	rt := cryptoRuntime(t)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		v := evalStr(t, rt, `ddcore.crypto.randomInt(-2, 3)`)
		seen[v] = true
		if !regexp.MustCompile(`^(-2|-1|0|1|2)$`).MatchString(v) {
			t.Fatalf("out of [-2, 3): %s", v)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("200 draws covered only %v", seen)
	}
	for _, bad := range []string{"1, 1", "5, 1", "0.5, 3", "0, 2**60", "0, NaN"} {
		if _, err := rt.Eval(`ddcore.crypto.randomInt(` + bad + `)`); err == nil {
			t.Fatalf("randomInt(%s) should throw", bad)
		}
	}
}

func TestRandomStringAlphabet(t *testing.T) {
	rt := cryptoRuntime(t)
	re := regexp.MustCompile(`^[a-z0-9]{10}$`)
	if s := evalStr(t, rt, `ddcore.utils.randomString()`); !re.MatchString(s) {
		t.Fatalf("default = %q", s)
	}
	if s := evalStr(t, rt, `ddcore.utils.randomString(500)`); len(s) != 500 || strings.Trim(s, alphabet) != "" {
		t.Fatalf("500 chars = %q", s)
	}
	if s := evalStr(t, rt, `ddcore.utils.randomString(0)`); s != "" {
		t.Fatalf("zero = %q", s)
	}
	if !regexp.MustCompile(`^[a-z0-9]{10}$`).MatchString(randomID()) {
		t.Fatal("randomID alphabet")
	}
}

func TestWebhookVerify(t *testing.T) {
	rt := cryptoRuntime(t)
	const id, body = "msg_p5jXN8AQM9LWM0D4loKWxJek", `{"test": 2432232314}`
	const ts = 1614265330
	good := "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
	call := func(sig string, opts string) string {
		hdr := fmt.Sprintf(`{"Webhook-Id": %q, "WEBHOOK-TIMESTAMP": "%d", "webhook-signature": %q}`, id, ts, sig)
		return evalStr(t, rt, fmt.Sprintf(`ddcore.webhooks.verify(%q, %s, %q, %s)`, hookSecret, hdr, body, opts))
	}
	// the vector is from 2021: widen the window instead of faking the clock
	wide := fmt.Sprintf(`{toleranceSeconds: %d}`, int64(time.Since(time.Unix(ts, 0)).Seconds())+60)
	if call(good, wide) != "true" {
		t.Fatal("the official vector should verify, header names in any case")
	}
	if call("v1,AAAA "+good+" v2,xyz", wide) != "true" {
		t.Fatal("any of several signatures may match")
	}
	if call(good, `{}`) != "false" {
		t.Fatal("a 2021 timestamp is outside the default 300 s window")
	}
	if call("v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OF=", wide) != "false" || call("v1,AAAA", wide) != "false" {
		t.Fatal("a wrong signature must fail")
	}
	if call("", wide) != "false" {
		t.Fatal("a missing signature must fail")
	}
	if got := evalStr(t, rt, fmt.Sprintf(`ddcore.webhooks.verify(%q, {}, "x")`, hookSecret)); got != "false" {
		t.Fatalf("no headers = %s", got)
	}
	// a tampered body
	hdr := fmt.Sprintf(`{"webhook-id": %q, "webhook-timestamp": "%d", "webhook-signature": %q}`, id, ts, good)
	if got := evalStr(t, rt, fmt.Sprintf(`ddcore.webhooks.verify(%q, %s, "{}", %s)`, hookSecret, hdr, wide)); got != "false" {
		t.Fatalf("tampered body = %s", got)
	}
}

func TestWebhookVerifyWindow(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	sign := func(ts int64) map[string]string {
		return map[string]string{"webhook-id": "i", "webhook-timestamp": fmt.Sprint(ts),
			"webhook-signature": SignWebhook(hookSecret, "i", ts, []byte("b"))}
	}
	tol := 300.0
	for _, c := range []struct {
		ts   int64
		want bool
	}{{now.Unix(), true}, {now.Unix() - 300, true}, {now.Unix() - 301, false}, {now.Unix() + 300, true}, {now.Unix() + 301, false}} {
		if got := VerifyWebhook(hookSecret, sign(c.ts), []byte("b"), tol, now); got != c.want {
			t.Errorf("ts offset %d: got %v", c.ts-now.Unix(), got)
		}
	}
}

func TestWebhookVerifyFailsClosed(t *testing.T) {
	rt := cryptoRuntime(t)
	now := time.Now().Unix()
	hdr := func(secret string, drop string, ts string) string {
		if ts == "" {
			ts = fmt.Sprint(now)
		}
		h := map[string]string{"webhook-id": "i", "webhook-timestamp": ts,
			"webhook-signature": SignWebhook(secret, "i", now, []byte("b"))}
		delete(h, drop)
		parts := []string{}
		for k, v := range h {
			parts = append(parts, fmt.Sprintf("%q: %q", k, v))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	verify := func(secret, headers string) string {
		return evalStr(t, rt, fmt.Sprintf(`ddcore.webhooks.verify(%s, %s, "b")`, secret, headers))
	}
	// the key "null" or "" would be known to the sender of the forgery
	for _, s := range []string{"null", "undefined", `""`, `"whsec_"`} {
		forged := hdr("null", "", "")
		if s == `""` || s == `"whsec_"` {
			forged = hdr("", "", "")
		}
		if got := verify(s, forged); got != "false" {
			t.Errorf("secret %s verified a forged request: %s", s, got)
		}
	}
	if verify(`"plain-secret"`, hdr("plain-secret", "", "")) != "true" {
		t.Error("a non-whsec_ secret is the raw key")
	}
	if verify(`"plain-secret"`, hdr("plain-secret", "webhook-id", "")) != "false" {
		t.Error("missing webhook-id")
	}
	if verify(`"plain-secret"`, hdr("plain-secret", "webhook-timestamp", "")) != "false" {
		t.Error("missing webhook-timestamp")
	}
	if verify(`"plain-secret"`, hdr("plain-secret", "", "abc")) != "false" {
		t.Error("non-numeric timestamp")
	}
	if VerifyWebhook("", map[string]string{}, nil, 300, time.Now()) || VerifyWebhook("whsec_", nil, nil, 300, time.Now()) {
		t.Error("Go level: empty secret")
	}
}

func TestWebhookVerifyHugeTolerance(t *testing.T) {
	rt := cryptoRuntime(t)
	ts := int64(1614265330)
	h := fmt.Sprintf(`{"webhook-id": "i", "webhook-timestamp": "%d", "webhook-signature": %q}`, ts, SignWebhook("k", "i", ts, []byte("b")))
	if got := evalStr(t, rt, fmt.Sprintf(`ddcore.webhooks.verify("k", %s, "b", {toleranceSeconds: 1e10})`, h)); got != "true" {
		t.Fatalf("a huge tolerance should switch the window off, got %s", got)
	}
}
