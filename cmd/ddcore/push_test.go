package main

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"strings"
	"testing"
)

// pushKeysEnv runs `ddcore push keys` and reads its .env lines.
func pushKeysEnv(t *testing.T, args ...string) map[string]string {
	t.Helper()
	var out bytes.Buffer
	if err := pushKeys(&out, args); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(line, "#") {
			env[k] = v
		}
	}
	return env
}

// The pair printed is a P-256 key in the web-push format: a 65-byte
// uncompressed public key that is the 32-byte private key's half.
func TestPushKeysPrintsAVAPIDPair(t *testing.T) {
	env := pushKeysEnv(t, "--subject", "mailto:ops@example.com")
	pub, err := base64.RawURLEncoding.DecodeString(env["DDCORE_SECRET_VAPID_PUBLIC_KEY"])
	if err != nil || len(pub) != 65 {
		t.Fatalf("public key %q (%v)", env["DDCORE_SECRET_VAPID_PUBLIC_KEY"], err)
	}
	priv, err := base64.RawURLEncoding.DecodeString(env["DDCORE_SECRET_VAPID_PRIVATE_KEY"])
	if err != nil || len(priv) != 32 {
		t.Fatalf("private key (%v)", err)
	}
	key, err := ecdh.P256().NewPrivateKey(priv)
	if err != nil || !bytes.Equal(key.PublicKey().Bytes(), pub) {
		t.Fatalf("the keys are not a pair (%v)", err)
	}
	if env["DDCORE_SECRET_VAPID_SUBJECT"] != "mailto:ops@example.com" {
		t.Fatalf("subject %q", env["DDCORE_SECRET_VAPID_SUBJECT"])
	}
	if other := pushKeysEnv(t); other["DDCORE_SECRET_VAPID_PRIVATE_KEY"] == env["DDCORE_SECRET_VAPID_PRIVATE_KEY"] {
		t.Fatal("two runs printed the same key")
	} else if v, ok := other["DDCORE_SECRET_VAPID_SUBJECT"]; !ok || v != "" {
		t.Fatalf("without --subject the variable is printed empty, got %q (%v)", v, ok)
	}
}
