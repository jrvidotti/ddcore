package engine

import (
	"crypto/rand"
	"os"
	"testing"
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
