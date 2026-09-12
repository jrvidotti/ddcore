# Secret Vault (`ddcore.vault` and `Vault` fieldtype) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a secure, encrypted secret vault (`ddcore.vault`) and a virtual `Vault` fieldtype for documents, so credentials belonging to rows (like API tokens per customer) are encrypted at rest with `DDCORE_SECRET_KEY`, audited on access, and never stored as columns in the database.

**Architecture:** 
- The vault persists encrypted records in `ddcore_vault` (`name text PRIMARY KEY, ciphertext bytea, nonce bytea, created, updated`) using AES-256-GCM with a master key from `DDCORE_SECRET_KEY`.
- Access and mutations (`read`, `write`, `delete`) are audited in the `Vault Audit Log` DocType (`tab_vault_audit_log`).
- The server runtime exposes `ddcore.vault.set()`, `get()`, `del()`, and `list()`.
- The `Vault` fieldtype is a virtual field (no column in `tab_<doctype>`); `Engine.save` updates/clears the vault entry using a derived key (`doctype:fieldname:name` or template in `options`), and reads return `{ configured: true }` without ever exposing the real secret over HTTP, MCP, `Version`, or export.
- Desk's `Control.svelte` provides a UI showing configured status, change, and clear actions.

**Tech Stack:** Go (1.24+), PostgreSQL, Svelte 5 (Desk), TypeScript / Goja.

**Spec:** `docs/superpowers/specs/2026-09-12-secret-vault-design.md`

## Global Constraints
- `DDCORE_SECRET_KEY` is read from environment; key derived via SHA-256 to produce 32 bytes for AES-256-GCM.
- Values must never be returned over HTTP, MCP, exports, or `Version` diffs.
- English is canonical for code, comments, doctype labels, and log messages; translations in `translations/<lang>.csv`.
- `make check` and `make test` must pass clean.

---

### Task 1: Schema Migration & Master Key Crypto Core

**Files:**
- Modify: `internal/db/schema.go:13-63`
- Create: `internal/engine/vault.go`
- Create: `internal/engine/vault_test.go`

**Interfaces:**
- Produces:
  - `MasterKey() ([]byte, error)`
  - `EncryptVault(key []byte, plaintext string) (ciphertext, nonce []byte, err error)`
  - `DecryptVault(key, ciphertext, nonce []byte) (string, error)`

- [ ] **Step 1: Write failing crypto unit tests**
Create `internal/engine/vault_test.go` with tests:
- `TestVaultEncryptDecrypt`: Encrypts a string and decrypts it back with the same key; verifies that ciphertext does not match plaintext and nonce is 12 bytes.
- `TestVaultDecryptTampered`: Decrypting with wrong key or corrupted ciphertext returns authentication error.
- `TestVaultMasterKey`: Derives key from `DDCORE_SECRET_KEY`; returns error when env is unset or empty.

- [ ] **Step 2: Run test to verify it fails**
Run: `go test ./internal/engine -run TestVaultEncryptDecrypt`
Expected: Compilation failure (functions not defined).

- [ ] **Step 3: Add `ddcore_vault` table to `InternalSchema` and implement crypto in `internal/engine/vault.go`**
In `internal/db/schema.go`:
Add `CREATE TABLE IF NOT EXISTS ddcore_vault (name text PRIMARY KEY, ciphertext bytea NOT NULL, nonce bytea NOT NULL, created timestamptz NOT NULL DEFAULT now(), updated timestamptz NOT NULL DEFAULT now());` to `InternalSchema`.
In `internal/engine/vault.go`:
Implement `MasterKey()`, `EncryptVault()`, and `DecryptVault()`.

- [ ] **Step 4: Run tests to verify they pass**
Run: `go test ./internal/engine -run TestVault`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add internal/db/schema.go internal/engine/vault.go internal/engine/vault_test.go
git commit -m "feat(vault): add ddcore_vault schema and AES-256-GCM encryption"
```

---

### Task 2: Core Vault Engine CRUD & `Vault Audit Log` DocType

**Files:**
- Create: `core/doctypes/vault_audit_log/vault_audit_log.doctype.ts`
- Modify: `internal/engine/vault.go`
- Modify: `internal/engine/vault_test.go`

**Interfaces:**
- Produces:
  - `(e *Engine) VaultSet(c *Ctx, name, value string) error`
  - `(e *Engine) VaultGet(c *Ctx, name string) (string, bool, error)`
  - `(e *Engine) VaultDel(c *Ctx, name string) error`
  - `(e *Engine) VaultList(c *Ctx, prefix string) ([]string, error)`
  - `(e *Engine) VaultStatus(ctx context.Context) (configured bool, count int, names []string, err error)`

- [ ] **Step 1: Create `Vault Audit Log` DocType**
Create `core/doctypes/vault_audit_log/vault_audit_log.doctype.ts` with fields `secret_name` (Data, inListView, searchIndex), `action` (Select: "read", "write", "delete", inListView), `user` (Link: "User", inListView), `ip` (Data), `request_id` (Data). Permissions: System Manager read/export/delete.

- [ ] **Step 2: Write failing unit tests for Vault CRUD and audit logging**
In `internal/engine/vault_test.go`:
- `TestVaultCRUD`: Set a secret, Get it, Del it, Get returns not found. List with prefix returns matching names and never values.
- `TestVaultAuditLogging`: Verify that Set writes `write` audit record, Get writes `read` audit record, and Del writes `delete` record with correct user/action.
- `TestVaultRefusesWithoutKey`: If `DDCORE_SECRET_KEY` is not set, Set/Get returns validation error mentioning `DDCORE_SECRET_KEY`.

- [ ] **Step 3: Implement Engine Vault CRUD and Auditing**
In `internal/engine/vault.go`:
Implement `VaultSet`, `VaultGet`, `VaultDel`, `VaultList`, and `VaultStatus`. Every op audits to `tab_vault_audit_log` via `c.Q()` or pool if no transaction.

- [ ] **Step 4: Run tests to verify they pass**
Run: `go test ./internal/engine -run TestVault`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add core/doctypes/vault_audit_log/ internal/engine/vault.go internal/engine/vault_test.go
git commit -m "feat(vault): implement engine CRUD methods and audit logging"
```

---

### Task 3: JS Host Bridge & SDK Typings

**Files:**
- Modify: `internal/engine/host.go`
- Modify: `internal/js/prelude.js`
- Modify: `packages/sdk/src/index.ts`
- Modify: `internal/engine/vault_test.go`

**Interfaces:**
- Produces:
  - `ddcore.vault.set(name: string, value: string): void`
  - `ddcore.vault.get(name: string): string | null`
  - `ddcore.vault.del(name: string): void`
  - `ddcore.vault.list(prefix?: string): string[]`

- [ ] **Step 1: Write failing JS integration test**
In `internal/engine/vault_test.go`:
Write `TestVaultJSRuntime`: sets up JS runtime with engine, executes TS/JS script calling `ddcore.vault.set("test:key", "s3cret")`, `ddcore.vault.get("test:key")`, `ddcore.vault.list("test:")`, and `ddcore.vault.del("test:key")`.

- [ ] **Step 2: Run test to verify it fails**
Run: `go test ./internal/engine -run TestVaultJSRuntime`
Expected: FAIL (unknown host operation or undefined ddcore.vault).

- [ ] **Step 3: Implement host operations and prelude exposure**
- In `internal/engine/host.go`: add handlers for `"vault.set"`, `"vault.get"`, `"vault.del"`, `"vault.list"`.
- In `internal/js/prelude.js`: bind `ddcore.vault: { set, get, del, list }`.
- In `packages/sdk/src/index.ts`: declare `vault` on `DDCoreAPI`.

- [ ] **Step 4: Run tests to verify they pass**
Run: `go test ./internal/engine -run TestVaultJSRuntime`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add internal/engine/host.go internal/js/prelude.js packages/sdk/src/index.ts internal/engine/vault_test.go
git commit -m "feat(vault): expose ddcore.vault in server JS runtime and SDK typings"
```

---

### Task 4: CLI Diagnostics (`ddcore doctor`)

**Files:**
- Modify: `cmd/ddcore/doctor.go`
- Modify: `cmd/ddcore/doctor_test.go` (or acceptance test)

- [ ] **Step 1: Write test for doctor vault reporting**
Add test in `cmd/ddcore/doctor_test.go` asserting that doctor reports vault configuration status, secret count, and names list (without values).

- [ ] **Step 2: Implement Vault section in `cmd/ddcore/doctor.go`**
Add `Vault` to `Report` struct in `doctor.go`. Populate it using `e.VaultStatus()`. If `DDCORE_SECRET_KEY` is not set and entries exist, add a warning or report unconfigured state.

- [ ] **Step 3: Run test to verify it passes**
Run: `go test ./cmd/ddcore/...`
Expected: PASS

- [ ] **Step 4: Commit**
```bash
git add cmd/ddcore/doctor.go cmd/ddcore/doctor_test.go
git commit -m "feat(vault): report vault status in ddcore doctor"
```

---

### Task 5: Meta & DB Virtual Field Support for `Vault`

**Files:**
- Modify: `internal/meta/meta.go`
- Modify: `internal/db/schema.go`
- Modify: `internal/meta/meta_test.go`

- [ ] **Step 1: Write test asserting `Vault` is a valid fieldtype and creates no SQL columns**
Add test in `internal/meta/meta_test.go` and `internal/db/schema_test.go` verifying that a DocType with fieldtype `"Vault"` passes validation, but `CreateTableSQL` / column planning produces no column in `tab_<doctype>`.

- [ ] **Step 2: Implement `Vault` in Meta and DDL**
- In `internal/meta/meta.go`: add `"Vault"` to `ValidFieldTypes`.
- In `internal/db/schema.go`: ensure `ColumnDef` ignores `Vault` fieldtype (treating it as virtual, similar to `Section Break` / `HTML`).

- [ ] **Step 3: Run tests to verify they pass**
Run: `go test ./internal/meta/... ./internal/db/...`
Expected: PASS

- [ ] **Step 4: Commit**
```bash
git add internal/meta/meta.go internal/db/schema.go internal/meta/meta_test.go internal/db/schema_test.go
git commit -m "feat(meta): register Vault as a virtual fieldtype without DDL columns"
```

---

### Task 6: Document Lifecycle, Key Derivation & Redaction for `Vault` Fields

**Files:**
- Modify: `internal/engine/doc.go`
- Modify: `internal/engine/secrets.go`
- Modify: `internal/engine/export.go`
- Create: `internal/engine/vault_doc_test.go`

- [ ] **Step 1: Write failing integration test for document lifecycle with Vault field**
In `internal/engine/vault_doc_test.go`:
- Define a DocType with a `Vault` field (with default key and custom template options).
- Insert doc with secret value -> verify no column in `tab_<doctype>`, secret present in `ddcore_vault`.
- Get doc -> field is redacted to `{ configured: true }`.
- Save doc without modifying field -> secret in vault unchanged.
- Save doc with new string -> secret in vault updated.
- Save doc with `{ clear: true }` -> secret deleted from vault.
- Delete doc -> secret deleted from vault.
- Check `Version` diff -> no vault field diff recorded.
- Check `Export` -> no vault field exported.

- [ ] **Step 2: Run test to verify it fails**
Run: `go test ./internal/engine -run TestVaultDocLifecycle`
Expected: FAIL.

- [ ] **Step 3: Implement Vault field handling in document lifecycle and redaction**
- In `internal/engine/doc.go`:
  - Intercept `Vault` fields before INSERT/UPDATE.
  - After insert/update commits/executes, call `VaultSet` with derived key (`doctype:fieldname:name` or evaluated template from `field.OptionsString()`). If `{ clear: true }`, call `VaultDel`.
  - In `Delete`: clean up vault entries for document.
- In `internal/engine/secrets.go`:
  - In `RedactDoc`: replace `Vault` field value with `{ configured: true }` if key exists in vault, or `nil`.
- In `internal/engine/export.go`:
  - Exclude `Vault` fields from exports.

- [ ] **Step 4: Run tests to verify they pass**
Run: `go test ./internal/engine -run TestVaultDocLifecycle`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add internal/engine/doc.go internal/engine/secrets.go internal/engine/export.go internal/engine/vault_doc_test.go
git commit -m "feat(engine): document lifecycle, key derivation, and redaction for Vault fields"
```

---

### Task 7: Desk UI Control for `Vault` Fieldtype

**Files:**
- Modify: `desk/src/lib/controls/Control.svelte`
- Modify: `desk/src/lib/field-width.test.ts`
- Modify: `desk/src/lib/controls/control-vault.test.ts` (new test)

- [ ] **Step 1: Add unit test for Vault control**
Create `desk/src/lib/controls/control-vault.test.ts` to test:
- Shows badge/placeholder when `value?.configured` is true.
- "Change" button enables input to enter new credential.
- "Clear" button marks value as `{ clear: true }`.
- Empty input when not configured.

- [ ] **Step 2: Implement Vault control in `Control.svelte`**
Add `{:else if ft === "Vault"}` block in `Control.svelte` with:
- Configured state: displays badge `Configured` / masked indicator with "Change" and "Clear" buttons.
- Input state: password input for setting new value.
- Clear state: button to undo clear.

- [ ] **Step 3: Run desk tests and type check**
Run: `cd desk && npm run check && npm test`
Expected: PASS

- [ ] **Step 4: Commit**
```bash
git add desk/src/lib/controls/Control.svelte desk/src/lib/field-width.test.ts desk/src/lib/controls/control-vault.test.ts
git commit -m "feat(desk): add UI control for Vault fieldtype"
```

---

### Task 8: Documentation, i18n & Full Verification

**Files:**
- Create: `docs/agent/vault.md`
- Modify: `docs/agent/fieldtypes.md`
- Modify: `translations/pt-BR.csv`

- [ ] **Step 1: Write documentation**
- Create `docs/agent/vault.md` documenting `ddcore.vault.set`, `get`, `del`, `list`, `DDCORE_SECRET_KEY`, and the `Vault` fieldtype.
- Update `docs/agent/fieldtypes.md` with `Vault` field row and details.

- [ ] **Step 2: Run i18n extraction and verification**
Run: `./bin/ddcore i18n extract --all --lang pt-BR`
Verify that all new labels (`Vault Audit Log`, `Secret Name`, etc.) are translated.

- [ ] **Step 3: Run full verification suite**
Run: `make check && make test`
Expected: ALL PASS.

- [ ] **Step 4: Commit**
```bash
git add docs/agent/vault.md docs/agent/fieldtypes.md translations/
git commit -m "docs: add vault documentation and update translation catalogs"
```
