package engine

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

func TestVaultDocLifecycle(t *testing.T) {
	extra := map[string]string{
		"doctypes/integration_account/integration_account.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Integration Account",
  idGeneration: { series: "ACC-.###" },
  trackChanges: true,
  fields: [
    { fieldname: "account_name", fieldtype: "Data", label: "Account Name", reqd: true },
    { fieldname: "api_key", fieldtype: "Vault", label: "API Key" },
    { fieldname: "custom_token", fieldtype: "Vault", label: "Custom Token", options: "custom:token:{id}" },
  ],
  permissions: [{ role: "All", read: true, write: true, create: true, delete: true }],
});`,
	}

	e := setupWith(t, extra)
	t.Setenv("DDCORE_SECRET_KEY", "test-master-key-xyz")
	ctx := t.Context()
	c := e.NewCtx(ctx, "admin@example.com")

	// 1. Insert document with Vault fields
	doc, err := c.NewDoc("Integration Account", Doc{
		"account_name": "Asaas Account",
		"api_key":      "sk_live_123456",
		"custom_token": "token_custom_789",
	})
	if err != nil {
		t.Fatalf("failed to create doc: %v", err)
	}

	saved, err := c.Insert(doc, SaveOpts{})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	docName := saved.ID()
	if !strings.HasPrefix(docName, "ACC-") {
		t.Fatalf("expected ACC- name prefix, got %q", docName)
	}

	// 2. Verify NO columns exist in tab_integration_account
	rows, err := db.Select(c.Ctx, c.Q(), "SELECT * FROM tab_integration_account WHERE id = $1", docName)
	if err != nil || len(rows) == 0 {
		t.Fatalf("select row failed: len=%d, err=%v", len(rows), err)
	}
	row := rows[0]
	if _, ok := row["api_key"]; ok {
		t.Fatalf("tab_integration_account must NOT contain column api_key: %v", row)
	}
	if _, ok := row["custom_token"]; ok {
		t.Fatalf("tab_integration_account must NOT contain column custom_token: %v", row)
	}

	// 3. Verify secrets exist in ddcore_vault with correct keys
	defaultKey := "Integration Account:api_key:" + docName
	customKey := "custom:token:" + docName

	val1, ok, err := e.VaultGet(c, defaultKey)
	if err != nil || !ok || val1 != "sk_live_123456" {
		t.Fatalf("expected vault secret for %q, got ok=%v val=%q err=%v", defaultKey, ok, val1, err)
	}

	val2, ok, err := e.VaultGet(c, customKey)
	if err != nil || !ok || val2 != "token_custom_789" {
		t.Fatalf("expected vault secret for %q, got ok=%v val=%q err=%v", customKey, ok, val2, err)
	}

	// 4. RedactDoc redacts Vault fields to { configured: true }
	redacted := c.RedactDoc("Integration Account", saved)
	cfg1, ok1 := redacted["api_key"].(map[string]any)
	if !ok1 || cfg1["configured"] != true {
		t.Fatalf("expected api_key redacted to {configured: true}, got %#v", redacted["api_key"])
	}
	cfg2, ok2 := redacted["custom_token"].(map[string]any)
	if !ok2 || cfg2["configured"] != true {
		t.Fatalf("expected custom_token redacted to {configured: true}, got %#v", redacted["custom_token"])
	}

	// 5. Save with unchanged vault fields does not wipe secrets
	saved["account_name"] = "Renamed Account"
	saved["api_key"] = map[string]any{"configured": true}
	saved["custom_token"] = nil
	updated, err := c.Save(saved, SaveOpts{})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	val1After, ok, _ := e.VaultGet(c, defaultKey)
	if !ok || val1After != "sk_live_123456" {
		t.Fatalf("secret was modified or cleared unexpectedly: %q", val1After)
	}

	// 6. Check Version diff does not contain Vault fields
	versions, err := c.GetList("Version", ListArgs{
		Fields:            []string{"data"},
		Filters:           map[string]any{"doc_id": docName},
		IgnorePermissions: true,
	})
	if err != nil {
		t.Fatalf("get versions failed: %v", err)
	}
	for _, v := range versions {
		dataStr := db.Str(v["data"])
		if strings.Contains(dataStr, "api_key") || strings.Contains(dataStr, "custom_token") {
			t.Fatalf("Version diff leaked vault fields: %s", dataStr)
		}
	}

	// 7. Update api_key with new string value
	updated["api_key"] = "new_secret_key_999"
	updated, err = c.Save(updated, SaveOpts{})
	if err != nil {
		t.Fatalf("save new key failed: %v", err)
	}
	val1New, ok, _ := e.VaultGet(c, defaultKey)
	if !ok || val1New != "new_secret_key_999" {
		t.Fatalf("secret was not updated: %q", val1New)
	}

	// 8. Clear custom_token via { clear: true }
	updated["custom_token"] = map[string]any{"clear": true}
	updated, err = c.Save(updated, SaveOpts{})
	if err != nil {
		t.Fatalf("save clear failed: %v", err)
	}
	val2Cleared, ok, _ := e.VaultGet(c, customKey)
	if ok || val2Cleared != "" {
		t.Fatalf("expected custom_token deleted from vault, got %q", val2Cleared)
	}

	// 9. Delete document cleans up remaining vault secrets
	if err := c.Delete("Integration Account", docName, true, false); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	val1Deleted, ok, _ := e.VaultGet(c, defaultKey)
	if ok || val1Deleted != "" {
		t.Fatalf("expected api_key deleted from vault on doc delete, got %q", val1Deleted)
	}
}

// A rename must carry a default-shaped vault key ("<DocType>:<field>:<name>")
// to the new name, or the secret is orphaned under a name the document no
// longer answers to.
func TestVaultRenameCarriesDefaultShapedSecret(t *testing.T) {
	extra := map[string]string{
		"doctypes/integration_account/integration_account.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Integration Account",
  idGeneration: { field: "account_name" },
  allowRename: true,
  fields: [
    { fieldname: "account_name", fieldtype: "Data", label: "Account Name", reqd: true },
    { fieldname: "api_key", fieldtype: "Vault", label: "API Key" },
  ],
  permissions: [{ role: "All", read: true, write: true, create: true, delete: true }],
});`,
	}

	e := setupWith(t, extra)
	t.Setenv("DDCORE_SECRET_KEY", "test-master-key-xyz")
	ctx := t.Context()
	c := e.NewCtx(ctx, "admin@example.com")

	doc, err := c.NewDoc("Integration Account", Doc{
		"account_name": "old-name",
		"api_key":      "sk_live_123456",
	})
	if err != nil {
		t.Fatalf("failed to create doc: %v", err)
	}
	saved, err := c.Insert(doc, SaveOpts{})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	docName := saved.ID()

	oldKey := "Integration Account:api_key:" + docName
	if _, ok, _ := e.VaultGet(c, oldKey); !ok {
		t.Fatalf("expected secret under %q before rename", oldKey)
	}

	newName, err := c.Rename("Integration Account", docName, "new-name")
	if err != nil {
		t.Fatalf("rename failed: %v", err)
	}

	newKey := "Integration Account:api_key:" + newName
	val, ok, err := e.VaultGet(c, newKey)
	if err != nil || !ok || val != "sk_live_123456" {
		t.Fatalf("expected secret readable under %q after rename, got ok=%v val=%q err=%v", newKey, ok, val, err)
	}
	if _, ok, _ := e.VaultGet(c, oldKey); ok {
		t.Fatalf("expected secret no longer readable under old key %q after rename", oldKey)
	}
}
