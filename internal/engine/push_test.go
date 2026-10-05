package engine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func b64url(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("%q: %v", s, err)
	}
	return b
}

// The example of RFC 8291 Appendix A, reproduced byte for byte: with the
// application server's key and the salt fixed, the body is fully determined.
func TestPushEncryptRFC8291Vector(t *testing.T) {
	asKey, err := ecdh.P256().NewPrivateKey(b64url(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.RawURLEncoding.EncodeToString(asKey.PublicKey().Bytes()); got != "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8" {
		t.Fatalf("as_public = %s", got)
	}
	body, err := pushEncrypt(
		[]byte("When I grow up, I want to be a watermelon"),
		b64url(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		b64url(t, "BTBZMqHH6r4Tts7J_aSIgg"),
		asKey,
		b64url(t, "DGv6ra1nlYgDCS1FRnbzlw"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if got := base64.RawURLEncoding.EncodeToString(body); got != want {
		t.Fatalf("body\n got %s\nwant %s", got, want)
	}
}

// One record of 4096 bytes is all a push message carries: 3993 bytes of
// plaintext fit, one more is a clear error.
func TestPushEncryptPlaintextLimit(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	as, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := bytes.Repeat([]byte{7}, 16)
	salt := bytes.Repeat([]byte{9}, 16)
	body, err := pushEncrypt(bytes.Repeat([]byte("x"), 3993), ua.PublicKey().Bytes(), auth, as, salt)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 4096 {
		t.Fatalf("a full message is %d bytes, want 4096", len(body))
	}
	_, err = pushEncrypt(bytes.Repeat([]byte("x"), 3994), ua.PublicKey().Bytes(), auth, as, salt)
	if err == nil || !strings.Contains(err.Error(), "3993") {
		t.Fatalf("want an error naming the limit, got %v", err)
	}
}

// pushDecrypt is what a browser does with a push message: the receiving half
// of RFC 8291, written apart from pushEncrypt.
func pushDecrypt(t *testing.T, body []byte, ua *ecdh.PrivateKey, auth []byte) []byte {
	t.Helper()
	if len(body) < 86 || binary.BigEndian.Uint32(body[16:20]) != 4096 || body[20] != 65 {
		t.Fatalf("bad header: % x", body[:min(len(body), 21)])
	}
	salt, asPublic := body[:16], body[21:86]
	asKey, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := ua.ECDH(asKey)
	if err != nil {
		t.Fatal(err)
	}
	// HKDF-Extract, and an HKDF-Expand of one block, are each one HMAC.
	hmacKey := func(key, data []byte) []byte {
		m := hmac.New(sha256.New, key)
		m.Write(data)
		return m.Sum(nil)
	}
	prkKey := hmacKey(auth, secret)
	keyInfo := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), asPublic...)
	ikm := hmacKey(prkKey, append(keyInfo, 1))
	prk := hmacKey(salt, ikm)
	cek := hmacKey(prk, []byte("Content-Encoding: aes128gcm\x00\x01"))[:16]
	nonce := hmacKey(prk, []byte("Content-Encoding: nonce\x00\x01"))[:12]
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[86:], nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	plain = bytes.TrimRight(plain, "\x00")
	if len(plain) == 0 || plain[len(plain)-1] != 2 {
		t.Fatal("the last record has no 0x02 delimiter")
	}
	return plain[:len(plain)-1]
}

// vapidTestKeys returns a VAPID pair in the form `ddcore push keys` prints.
func vapidTestKeys(t *testing.T) (pub, priv string, key *ecdh.PrivateKey) {
	t.Helper()
	pub, priv, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	key, err = ecdh.P256().NewPrivateKey(b64url(t, priv))
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv, key
}

// verifyVAPID checks a VAPID JWT the way a push service does, with the public
// key alone, and returns its claims.
func verifyVAPID(t *testing.T, jwt string, pub []byte) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %s", jwt)
	}
	var header map[string]any
	if err := json.Unmarshal(b64url(t, parts[0]), &header); err != nil || header["alg"] != "ES256" || header["typ"] != "JWT" {
		t.Fatalf("header %v (%v)", header, err)
	}
	sig := b64url(t, parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes, want r||s of 64", len(sig))
	}
	pk, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pub)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pk, hash[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("the signature does not verify with the public key")
	}
	var claims map[string]any
	if err := json.Unmarshal(b64url(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

// The JWT is ES256 over the endpoint's origin, signed r||s, and verifies with
// the public key a browser subscribed with.
func TestPushVAPIDJWT(t *testing.T) {
	pub, priv, _ := vapidTestKeys(t)
	key, err := vapidKey(pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(12 * time.Hour).Truncate(time.Second)
	jwt, err := vapidJWT(key, "https://push.example.net", "mailto:ops@example.com", exp)
	if err != nil {
		t.Fatal(err)
	}
	claims := verifyVAPID(t, jwt, b64url(t, pub))
	if claims["aud"] != "https://push.example.net" || claims["sub"] != "mailto:ops@example.com" || claims["exp"] != float64(exp.Unix()) {
		t.Fatalf("claims %v", claims)
	}
}

// A public key that is not the private key's half is refused, and the error
// says nothing of either.
func TestPushVAPIDKeysMustBeAPair(t *testing.T) {
	pub, _, _ := vapidTestKeys(t)
	_, priv, _ := vapidTestKeys(t)
	_, err := vapidKey(pub, priv)
	if err == nil || !strings.Contains(err.Error(), "not a pair") {
		t.Fatalf("want a mismatch error, got %v", err)
	}
	if strings.Contains(err.Error(), priv) || strings.Contains(err.Error(), pub) {
		t.Fatalf("the error echoes a key: %v", err)
	}
	if _, err := vapidKey(pub, "@@"); err == nil || strings.Contains(err.Error(), "@@") || !strings.Contains(err.Error(), "DDCORE_SECRET_VAPID_PRIVATE_KEY") {
		t.Fatalf("want an error naming the variable, got %v", err)
	}
}

// setVAPID configures the site's VAPID secrets for one test.
func setVAPID(t *testing.T) (pub string) {
	t.Helper()
	pub, priv, _ := vapidTestKeys(t)
	t.Setenv("DDCORE_SECRET_VAPID_PUBLIC_KEY", pub)
	t.Setenv("DDCORE_SECRET_VAPID_PRIVATE_KEY", priv)
	t.Setenv("DDCORE_SECRET_VAPID_SUBJECT", "mailto:ops@example.com")
	return pub
}

// subscriptionJS is a browser's PushSubscription.toJSON() for ua, at endpoint.
func subscriptionJS(endpoint string, ua *ecdh.PrivateKey, auth []byte) string {
	return fmt.Sprintf(`{endpoint: %q, keys: {p256dh: %q, auth: %q}}`, endpoint,
		base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(auth))
}

// The whole way: ddcore.push.send encrypts for the subscription, signs for
// the push service, and hands back the service's answer; 410 is an answer,
// not an exception, because deleting the subscription is the app's call.
func TestPushSendRoundTrip(t *testing.T) {
	vapidPub := setVAPID(t)
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := bytes.Repeat([]byte{3}, 16)
	status := http.StatusCreated
	var got struct {
		plain                         string
		ttl, urgency, topic, encoding string
		claims                        map[string]any
	}
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/push/abc" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		got.plain = string(pushDecrypt(t, body, ua, auth))
		got.ttl, got.urgency, got.topic = r.Header.Get("TTL"), r.Header.Get("Urgency"), r.Header.Get("Topic")
		got.encoding = r.Header.Get("Content-Encoding")
		authz := r.Header.Get("Authorization")
		var jwt, k string
		if _, err := fmt.Sscanf(strings.ReplaceAll(authz, ",", " "), "vapid t=%s k=%s", &jwt, &k); err != nil {
			t.Errorf("authorization %q: %v", authz, err)
		}
		if k != vapidPub {
			t.Errorf("k = %q, want the site's public key", k)
		}
		got.claims = verifyVAPID(t, jwt, b64url(t, k))
		w.Header().Set("Location", serverURL+"/message/1")
		w.WriteHeader(status)
		fmt.Fprint(w, "done")
	}))
	defer server.Close()
	serverURL = server.URL
	rt := mtlsRuntime(t)
	sub := subscriptionJS(server.URL+"/push/abc", ua, auth)

	v, err := rt.Eval(fmt.Sprintf(`ddcore.push.send(%s, {title: "Due tomorrow", body: "R$ 137,24"}, {ttl: 60, urgency: "high", topic: "due-1"})`, sub))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Status  int               `json:"status"`
		Body    string            `json:"body"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(v, &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != 201 || res.Body != "done" || res.Headers["Location"] != server.URL+"/message/1" {
		t.Fatalf("result %+v", res)
	}
	if got.plain != `{"title":"Due tomorrow","body":"R$ 137,24"}` {
		t.Fatalf("payload %q", got.plain)
	}
	if got.ttl != "60" || got.urgency != "high" || got.topic != "due-1" || got.encoding != "aes128gcm" {
		t.Fatalf("headers %+v", got)
	}
	exp, _ := got.claims["exp"].(float64)
	if got.claims["aud"] != server.URL || got.claims["sub"] != "mailto:ops@example.com" ||
		time.Until(time.Unix(int64(exp), 0)) > 12*time.Hour || time.Until(time.Unix(int64(exp), 0)) < 11*time.Hour {
		t.Fatalf("claims %v", got.claims)
	}

	// a string goes as is; the defaults are a day of TTL and no Urgency
	status = http.StatusGone
	v, err = rt.Eval(fmt.Sprintf(`ddcore.push.send(%s, "plain text").status`, sub))
	if err != nil {
		t.Fatalf("a 410 must not throw: %v", err)
	}
	if string(v) != "410" || got.plain != "plain text" || got.ttl != "86400" || got.urgency != "" || got.topic != "" {
		t.Fatalf("status %v, payload %q, headers %+v", v, got.plain, got)
	}
}

// Without the keys, send names the variable to set, and publicKey is null.
func TestPushMissingKeys(t *testing.T) {
	rt := mtlsRuntime(t)
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	sub := subscriptionJS("https://push.example.net/x", ua, bytes.Repeat([]byte{1}, 16))
	v, err := rt.Eval(`ddcore.push.publicKey()`)
	if err != nil || string(v) != "null" {
		t.Fatalf("publicKey() = %v (%v), want null", v, err)
	}
	pub, priv, _ := vapidTestKeys(t)
	for _, tc := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{}, "DDCORE_SECRET_VAPID_PUBLIC_KEY"},
		{map[string]string{"DDCORE_SECRET_VAPID_PUBLIC_KEY": pub}, "DDCORE_SECRET_VAPID_PRIVATE_KEY"},
		{map[string]string{"DDCORE_SECRET_VAPID_PUBLIC_KEY": pub, "DDCORE_SECRET_VAPID_PRIVATE_KEY": priv}, "DDCORE_SECRET_VAPID_SUBJECT"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := rt.Eval(fmt.Sprintf(`ddcore.push.send(%s, "hi")`, sub))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error naming %s, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), priv) {
				t.Fatalf("the error echoes the private key: %v", err)
			}
		})
	}
	t.Setenv("DDCORE_SECRET_VAPID_PUBLIC_KEY", pub)
	if v, err := rt.Eval(`ddcore.push.publicKey()`); err != nil || string(v) != `"`+pub+`"` {
		t.Fatalf("publicKey() = %v (%v)", v, err)
	}
}

// Bad options and bad subscriptions are validation errors before anything
// is sent; a push service that cannot be reached is one too.
func TestPushSendErrors(t *testing.T) {
	setVAPID(t)
	rt := mtlsRuntime(t)
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := bytes.Repeat([]byte{1}, 16)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	good := subscriptionJS("https://push.example.net/x", ua, auth)
	for _, tc := range []struct{ name, call, want string }{
		{"urgency", fmt.Sprintf(`(%s, "x", {urgency: "urgent"})`, good), "opts.urgency"},
		{"ttl", fmt.Sprintf(`(%s, "x", {ttl: -1})`, good), "opts.ttl"},
		{"topic", fmt.Sprintf(`(%s, "x", {topic: "has spaces"})`, good), "opts.topic"},
		{"endpoint", fmt.Sprintf(`({endpoint: "nowhere", keys: %s.keys}, "x")`, good), "subscription.endpoint"},
		{"p256dh", `({endpoint: "https://push.example.net/x", keys: {p256dh: "AAAA", auth: "AAAAAAAAAAAAAAAAAAAAAA"}}, "x")`, "subscription.keys.p256dh"},
		{"auth", fmt.Sprintf(`({endpoint: "https://push.example.net/x", keys: {p256dh: %s.keys.p256dh, auth: "AAAA"}}, "x")`, good), "subscription.keys.auth"},
		{"size", fmt.Sprintf(`(%s, "x".repeat(3994))`, good), "3993"},
		{"network", fmt.Sprintf(`(%s, "x")`, subscriptionJS(closed.URL+"/x", ua, auth)), "push:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typ, err := rt.Eval(`(() => { try { ddcore.push.send` + tc.call + `; return "no error" } catch (e) { return e.name + ": " + e.message } })()`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(typ), "ValidationError") || !strings.Contains(string(typ), tc.want) {
				t.Fatalf("want a ValidationError naming %q, got %v", tc.want, typ)
			}
		})
	}
}
