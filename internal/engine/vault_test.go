package engine

import (
	"crypto/rand"
	"os"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

func TestVaultMasterKey(t *testing.T) {
	orig := os.Getenv("DDCORE_SECRET_KEY")
	defer os.Setenv("DDCORE_SECRET_KEY", orig)

	os.Unsetenv("DDCORE_SECRET_KEY")
	_, err := MasterKey()
	if err == nil {
		t.Fatal("expected error when DDCORE_SECRET_KEY is unset")
	}

	os.Setenv("DDCORE_SECRET_KEY", "   ")
	_, err = MasterKey()
	if err == nil {
		t.Fatal("expected error when DDCORE_SECRET_KEY is blank")
	}

	os.Setenv("DDCORE_SECRET_KEY", "my-super-secret-key-12345")
	key, err := MasterKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d bytes", len(key))
	}

	// Same secret should produce deterministic key
	key2, err := MasterKey()
	if err != nil || string(key) != string(key2) {
		t.Fatalf("expected deterministic key derivation")
	}
}

func TestVaultEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	plaintexts := []string{
		"asaas_token_live_1234567890",
		"",
		"special chars: 🔐 🚀 \n \t \r \x00 unicode",
	}

	for _, pt := range plaintexts {
		ciphertext, nonce, err := EncryptVault(key, pt)
		if err != nil {
			t.Fatalf("encryption failed for %q: %v", pt, err)
		}
		if len(nonce) != 12 {
			t.Fatalf("expected 12-byte nonce, got %d", len(nonce))
		}
		if pt != "" && string(ciphertext) == pt {
			t.Fatalf("ciphertext equals plaintext")
		}

		got, err := DecryptVault(key, ciphertext, nonce)
		if err != nil {
			t.Fatalf("decryption failed for %q: %v", pt, err)
		}
		if got != pt {
			t.Fatalf("decrypted mismatch: got %q, want %q", got, pt)
		}
	}
}

func TestVaultDecryptTampered(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	ciphertext, nonce, err := EncryptVault(key, "sensitive-credential")
	if err != nil {
		t.Fatal(err)
	}

	// Wrong key
	wrongKey := make([]byte, 32)
	rand.Read(wrongKey)
	_, err = DecryptVault(wrongKey, ciphertext, nonce)
	if err == nil {
		t.Fatal("expected error with wrong key")
	}

	// Tampered ciphertext
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[0] ^= 0xff
	_, err = DecryptVault(key, tampered, nonce)
	if err == nil {
		t.Fatal("expected error with tampered ciphertext")
	}

	// Tampered nonce
	tamperedNonce := make([]byte, len(nonce))
	copy(tamperedNonce, nonce)
	tamperedNonce[0] ^= 0xff
	_, err = DecryptVault(key, ciphertext, tamperedNonce)
	if err == nil {
		t.Fatal("expected error with tampered nonce")
	}
}

func TestVaultCRUDAndAuditing(t *testing.T) {
	e := setup(t)
	t.Setenv("DDCORE_SECRET_KEY", "test-master-key-xyz")

	ctx := t.Context()
	c := e.NewCtx(ctx, "admin@example.com")
	c.ReqID = "req-test-123"

	// 1. Initially unset
	val, ok, err := e.VaultGet(c, "asaas:token:PES-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok || val != "" {
		t.Fatalf("expected unset, got ok=%v val=%q", ok, val)
	}

	// 2. Set secret
	err = e.VaultSet(c, "asaas:token:PES-1", "token-secret-1")
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	// 3. Get secret
	val, ok, err = e.VaultGet(c, "asaas:token:PES-1")
	if err != nil || !ok || val != "token-secret-1" {
		t.Fatalf("get mismatch: ok=%v, val=%q, err=%v", ok, val, err)
	}

	// 4. Set another secret
	if err := e.VaultSet(c, "asaas:token:PES-2", "token-secret-2"); err != nil {
		t.Fatal(err)
	}
	if err := e.VaultSet(c, "other:key:1", "other-secret"); err != nil {
		t.Fatal(err)
	}

	// 5. List with prefix
	list, err := e.VaultList(c, "asaas:token:")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 2 || list[0] != "asaas:token:PES-1" || list[1] != "asaas:token:PES-2" {
		t.Fatalf("unexpected list: %v", list)
	}

	// 6. Delete
	if err := e.VaultDel(c, "asaas:token:PES-1"); err != nil {
		t.Fatalf("del failed: %v", err)
	}
	val, ok, err = e.VaultGet(c, "asaas:token:PES-1")
	if err != nil || ok || val != "" {
		t.Fatalf("expected deleted, got ok=%v, val=%q", ok, val)
	}

	// 7. Verify audit log entries
	auditLogs, err := c.GetList("Vault Audit Log", ListArgs{
		Fields:            []string{"name", "secret_name", "action", "user", "request_id"},
		Filters:           map[string]any{"secret_name": "asaas:token:PES-1"},
		OrderBy:           "creation asc",
		IgnorePermissions: true,
	})
	if err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	// We expect: 1 write, 2 reads, 1 delete, 1 read
	if len(auditLogs) < 3 {
		t.Fatalf("expected at least 3 audit log records, got %d", len(auditLogs))
	}
	first := auditLogs[0]
	if db.Str(first["action"]) != "write" || db.Str(first["user"]) != "admin@example.com" || db.Str(first["request_id"]) != "req-test-123" {
		t.Fatalf("unexpected first audit record: %v", first)
	}
}

func TestVaultRefusesWithoutKey(t *testing.T) {
	e := setup(t)
	os.Unsetenv("DDCORE_SECRET_KEY")

	ctx := t.Context()
	c := e.NewCtx(ctx, "admin@example.com")

	if err := e.VaultSet(c, "key:1", "val"); err == nil {
		t.Fatal("expected error on VaultSet without DDCORE_SECRET_KEY")
	}
	if _, _, err := e.VaultGet(c, "key:1"); err == nil {
		t.Fatal("expected error on VaultGet without DDCORE_SECRET_KEY")
	}
}
