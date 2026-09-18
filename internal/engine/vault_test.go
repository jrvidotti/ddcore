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

	// 7. Verify audit event entries
	auditLogs, err := c.GetList("Audit Event", ListArgs{
		Fields:            []string{"name", "action", "outcome", "actor", "target_doctype", "target_name", "request_id"},
		Filters:           map[string]any{"target_doctype": "Vault Secret", "target_name": "asaas:token:PES-1"},
		OrderBy:           "creation asc",
		IgnorePermissions: true,
	})
	if err != nil {
		t.Fatalf("failed to query audit logs: %v", err)
	}
	// We expect: 1 write, 2 reads, 1 delete, 1 read
	if len(auditLogs) < 3 {
		t.Fatalf("expected at least 3 audit event records, got %d", len(auditLogs))
	}
	first := auditLogs[0]
	if db.Str(first["action"]) != "vault.write" || db.Str(first["outcome"]) != "Allowed" ||
		db.Str(first["actor"]) != "admin@example.com" || db.Str(first["target_doctype"]) != "Vault Secret" ||
		db.Str(first["target_name"]) != "asaas:token:PES-1" || db.Str(first["request_id"]) != "req-test-123" {
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

func TestVaultJSRuntime(t *testing.T) {
	e := setup(t)
	t.Setenv("DDCORE_SECRET_KEY", "test-master-key-xyz")
	ctx := t.Context()

	code := `
ddcore.vault.set("customer:token:42", "token-from-js");
const v1 = ddcore.vault.get("customer:token:42");
if (v1 !== "token-from-js") throw new Error("expected token-from-js, got " + v1);

const list = ddcore.vault.list("customer:token:");
if (!Array.isArray(list) || list.length !== 1 || list[0] !== "customer:token:42") {
    throw new Error("unexpected list: " + JSON.stringify(list));
}

ddcore.vault.del("customer:token:42");
const v2 = ddcore.vault.get("customer:token:42");
if (v2 !== null) throw new Error("expected null after del, got " + v2);
"ok";
`
	out, _, err := e.Eval(ctx, code, true)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	if string(out) != `"ok"` {
		t.Fatalf("expected ok, got %s", out)
	}
}

func TestVaultAbsorptionPatch(t *testing.T) {
	e := setup(t)
	ctx := t.Context()

	// 1. Create temporary tab_vault_audit_log table to simulate pre-migration state
	_, err := e.DB.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tab_vault_audit_log (
		name text primary key,
		owner text,
		creation timestamptz default now(),
		modified timestamptz default now(),
		modified_by text,
		docstatus smallint default 0,
		secret_name text,
		action text,
		"user" text,
		ip text,
		request_id text
	)`)
	if err != nil {
		t.Fatal(err)
	}
	defer e.DB.Pool.Exec(ctx, "DROP TABLE IF EXISTS tab_vault_audit_log")

	// 2. Insert legacy rows
	_, err = e.DB.Pool.Exec(ctx, `INSERT INTO tab_vault_audit_log
		(name, owner, action, "user", secret_name, ip, request_id) VALUES
		('val-1', 'admin@example.com', 'write', 'admin@example.com', 'stripe:key:1', '127.0.0.1', 'req-legacy-1'),
		('val-2', 'admin@example.com', 'read', 'admin@example.com', 'stripe:key:1', '127.0.0.1', 'req-legacy-2')`)
	if err != nil {
		t.Fatal(err)
	}
	// Clear patch record so Migrate runs it against the legacy table
	_, _ = e.DB.Pool.Exec(ctx, "DELETE FROM ddcore_patch WHERE name = '0001_absorb_vault_audit_log'")

	// 3. Execute migration (which executes beforeSchema patches including 0001_absorb_vault_audit_log)
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	// 4. Verify tab_vault_audit_log no longer exists
	var tableExists bool
	err = e.DB.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'tab_vault_audit_log')`).Scan(&tableExists)
	if err != nil || tableExists {
		t.Fatalf("expected tab_vault_audit_log to be dropped, exists=%v, err=%v", tableExists, err)
	}

	// 5. Verify rows migrated into tab_audit_event
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT name, action, outcome, actor, target_doctype, target_name, request_id
		FROM tab_audit_event WHERE name IN ('val-1', 'val-2') ORDER BY name ASC`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 migrated rows, got %d", len(rows))
	}
	if db.Str(rows[0]["action"]) != "vault.write" || db.Str(rows[0]["target_name"]) != "stripe:key:1" || db.Str(rows[0]["request_id"]) != "req-legacy-1" {
		t.Fatalf("unexpected migrated row 0: %+v", rows[0])
	}
	if db.Str(rows[1]["action"]) != "vault.read" || db.Str(rows[1]["outcome"]) != "Allowed" {
		t.Fatalf("unexpected migrated row 1: %+v", rows[1])
	}
}
