# Administrative Audit Coverage (PRD-06) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide comprehensive, tamper-proof administrative audit coverage (PRD-06) across ddcore by absorbing `Vault Audit Log` into `Audit Event`, instrumenting role and account mutations, auditing job administration, redacting sensitive payloads, and introducing `ddcore audit` CLI commands.

**Architecture:** Extend the existing `Audit Event` DocType and engine facility (`internal/engine/audit.go`) to act as the single administrative audit ledger. Write allowed actions on caller transactions and denied actions on the database pool. Enforce payload sanitization to strip credentials, passwords, and secrets. Absorb legacy `Vault Audit Log` via a schema patch and add CLI inspection and retention commands.

**Tech Stack:** Go 1.24, PostgreSQL 17, TypeScript / SvelteKit desk, `@ddcore/sdk`.

**Spec:** [`docs/superpowers/specs/2026-09-14-administrative-audit-design.md`](../specs/2026-09-14-administrative-audit-design.md)

## Global Constraints

- Synchronous TypeScript on server: no `await` in server services or controllers.
- Transactional duality: allowed audits commit with caller transaction; denied audits write to connection pool.
- Sensitive redaction: passwords, tokens, secrets, encryption keys, and credentials must never appear in `detail`.
- Immutability: `Audit Event` records cannot be created, updated, or deleted via REST, RPC, or SDK APIs.
- Translations: all user-facing strings must be English canonical and translated in `translations/pt-BR.csv`.
- Verification: all changes must pass `make test` and `make check`.

---

### Task 1: Absorb Vault Audit Log & Schema Migration

**Files:**
- Create: `core/patches/0001_absorb_vault_audit_log.ts`
- Modify: `core/embed.go`
- Delete: `core/doctypes/vault_audit_log/vault_audit_log.doctype.ts`
- Modify: `internal/engine/vault.go`
- Modify: `internal/engine/vault_test.go`
- Modify: `internal/engine/webhooks.go`
- Modify: `internal/js/notifications.go`
- Modify: `internal/engine/notification_load_test.go`
- Modify: `docs/agent/vault.md`

**Interfaces:**
- Consumes: `c.Audit(action, targetDoctype, targetName, detail)`
- Produces: `tab_audit_event` rows for `vault.read`, `vault.write`, `vault.delete`

- [ ] **Step 1: Write test expecting vault operations to produce `Audit Event` rows**

In `internal/engine/vault_test.go`, update test cases so that after `VaultSet`, `VaultGet`, and `VaultDel`, `Audit Event` rows are asserted (with `action` in `vault.write`, `vault.read`, `vault.delete`, `target_doctype = "Vault Secret"`, `target_name = secretName`, and `outcome = "Allowed"`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestVault ./internal/engine`
Expected: FAIL (looking for `Audit Event` rows while code currently writes `tab_vault_audit_log`).

- [ ] **Step 3: Create migration patch `0001_absorb_vault_audit_log.ts` and update `core/embed.go`**

Create `core/patches/0001_absorb_vault_audit_log.ts`:
```ts
import { definePatch } from "@ddcore/sdk";

export default definePatch({
  phase: "beforeSchema",
  description: "Absorb Vault Audit Log into Audit Event and drop tab_vault_audit_log",
  execute() {
    ddcore.db.sql(`
      DO $$
      BEGIN
        IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'tab_vault_audit_log') THEN
          INSERT INTO tab_audit_event (name, owner, creation, modified, modified_by, docstatus, action, outcome, actor, target_doctype, target_name, ip, request_id, detail)
          SELECT name, owner, creation, modified, modified_by, docstatus, 'vault.' || action, 'Allowed', "user", 'Vault Secret', secret_name, ip, request_id, NULL
          FROM tab_vault_audit_log
          ON CONFLICT (name) DO NOTHING;
          DROP TABLE tab_vault_audit_log;
        END IF;
      END $$;
    `);
  },
});
```
Update `core/embed.go` to include `patches`:
```go
//go:embed ddcore.app.ts doctypes mail patches services translations
var FS embed.FS
```

- [ ] **Step 4: Update `internal/engine/vault.go` and remove `Vault Audit Log` DocType**

In `internal/engine/vault.go`:
Replace `e.recordVaultAudit` with direct calls to `c.Audit("vault."+action, "Vault Secret", secretName, nil)`.
Delete `core/doctypes/vault_audit_log/vault_audit_log.doctype.ts`.
Remove references to `"Vault Audit Log"` in `internal/engine/webhooks.go`, `internal/js/notifications.go`, `internal/engine/notification_load_test.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run TestVault ./internal/engine`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add core/ internal/ docs/
git commit -m "feat(audit): absorb Vault Audit Log into Audit Event (PRD-06)"
```

---

### Task 2: Core Audit Engine Enhancements (Sanitization & Querying & Immutability)

**Files:**
- Modify: `internal/engine/audit.go`
- Modify: `internal/engine/insert.go`
- Modify: `internal/engine/save.go`
- Modify: `internal/engine/delete.go`
- Create: `internal/engine/audit_test.go`

**Interfaces:**
- Consumes: `db.Querier`, `Ctx`
- Produces:
  - `SanitizeAuditDetail(detail map[string]any) map[string]any`
  - `(e *Engine) ListAuditEvents(ctx context.Context, f AuditFilter) ([]map[string]any, error)`
  - `(e *Engine) CountAuditEvents(ctx context.Context, f AuditFilter) (int64, error)`
  - `(e *Engine) PurgeAuditEvents(ctx context.Context, days int, dryRun bool) (int, error)`

- [ ] **Step 1: Write unit tests for audit detail sanitization and immutability**

In `internal/engine/audit_test.go`:
- Test that keys containing `password`, `token`, `secret`, `hash`, `key`, `credential` are redacted to `"[REDACTED]"`.
- Test that strings longer than 500 characters are truncated.
- Test that direct `c.Insert`, `c.Save`, `c.Delete` on `Audit Event` return `PermissionError`.
- Test `ListAuditEvents`, `CountAuditEvents`, and `PurgeAuditEvents`.

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestAudit ./internal/engine`
Expected: FAIL (sanitizer and query functions not yet defined).

- [ ] **Step 3: Implement sanitization, query filters, and immutability guards**

In `internal/engine/audit.go`:
- Implement recursive `sanitizeAuditMap` redacting sensitive keys and truncating oversized strings before JSON serialization.
- Define `AuditFilter` struct (`Action`, `Actor`, `TargetDocType`, `TargetName`, `Outcome`, `Since`, `Until`, `Limit`, `Start`).
- Implement `ListAuditEvents`, `CountAuditEvents`, `PurgeAuditEvents`.
- In `internal/engine/insert.go`, `save.go`, `delete.go`: ensure operations on `"Audit Event"` fail with `cerr.Permission("Audit Event records are immutable and cannot be modified")` unless internal bypass flag is active.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestAudit ./internal/engine`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/
git commit -m "feat(audit): add payload sanitization, querying, and immutability guards (PRD-06)"
```

---

### Task 3: Instrument Role & Permission Mutations

**Files:**
- Modify: `core/doctypes/user/user.controller.ts`
- Modify: `internal/engine/audit_test.go`

**Interfaces:**
- Consumes: `c.Audit("role.assign" | "role.revoke", "User", username, detail)`
- Produces: Audit records when user roles change or accounts are enabled/disabled.

- [ ] **Step 1: Write test for role change auditing**

In `internal/engine/audit_test.go`:
- Test creating a user, adding roles `["Accounts User"]`, saving, and verifying `role.assign` audit event exists.
- Test removing a role and verifying `role.revoke` audit event exists with target user.
- Test changing user `enabled` from true to false and verifying `account.disable` audit event exists.

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestAudit_RoleChanges ./internal/engine`
Expected: FAIL

- [ ] **Step 3: Implement role diffing and auditing in `user.controller.ts` / engine**

In `core/doctypes/user/user.controller.ts` (or lifecycle handler):
- In `onUpdate`: compare `doc.roles` with `doc.getDocBeforeSave()?.roles`.
- For added roles, call audit with `role.assign`.
- For removed roles, call audit with `role.revoke`.
- When `doc.enabled` changes, audit `account.enable` or `account.disable`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestAudit_RoleChanges ./internal/engine`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/doctypes/user/ internal/engine/
git commit -m "feat(audit): audit role assignments, revocations, and account status changes (PRD-06)"
```

---

### Task 4: Instrument Account Administration & Denials

**Files:**
- Modify: `core/services/users.ts`
- Modify: `internal/engine/auth_test.go`

**Interfaces:**
- Consumes: `(ddcore as any).audit(action, targetDocType, targetName, detail)`
- Produces: Audit records for `account.invite`, `account.resend_invite`, `account.reset_password`, `account.unlock`, `account.revoke_sessions`.

- [ ] **Step 1: Write test verifying administrative user operations leave audit events**

In `internal/engine/auth_test.go` (or `users_test.go`):
- Call `users.invite`, `users.sendPasswordReset`, `users.unlockUser`, `users.revokeUserSessions`.
- Verify audit rows with `outcome = "Allowed"` and expected action names.
- Verify unauthorized calls produce `AuditDenied`.

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestAudit_AccountAdmin ./internal/engine`
Expected: FAIL

- [ ] **Step 3: Implement auditing in `core/services/users.ts`**

In `core/services/users.ts`:
- Expose `ddcore.audit(action, targetDocType, targetName, detail)` via JS bridge if not already exposed.
- Add audit calls in `invite`, `resendInvite`, `sendPasswordReset`, `revokeUserSessions`, `unlockUser`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestAudit_AccountAdmin ./internal/engine`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/services/users.ts internal/
git commit -m "feat(audit): audit administrative user service actions (PRD-06)"
```

---

### Task 5: Instrument Background Job Administration

**Files:**
- Modify: `internal/engine/jobs_admin.go`
- Modify: `internal/api/jobs.go`
- Modify: `internal/engine/jobs_admin_test.go`

**Interfaces:**
- Consumes: `c.Audit("job.cancel" | "job.retry" | "job.purge", "Job", id, detail)`
- Produces: Audit rows for job administration and `AuditDenied` on unauthorized attempts.

- [ ] **Step 1: Write test for job admin auditing**

In `internal/engine/jobs_admin_test.go`:
- Test `CancelJob` records `job.cancel` with job method and queue.
- Test `RetryJob` records `job.retry` with old and new job ID.
- Test `PurgeJobs` records `job.purge`.
- Test unauthorized job admin call records `AuditDenied`.

- [ ] **Step 2: Run test to verify failure**

Run: `go test -run TestAudit_JobAdmin ./internal/engine`
Expected: FAIL

- [ ] **Step 3: Implement auditing in `jobs_admin.go` and `internal/api/jobs.go`**

In `internal/engine/jobs_admin.go`:
- In `CancelJob`, `RetryJob`, `PurgeJobs`: record audit entries on `c` (or engine pool if without Ctx).
- In `internal/api/jobs.go`: in `requireJobAdmin`, on failure record `c.AuditDenied("job.admin", "Job", urlParam(r, "id"), nil)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestAudit_JobAdmin ./internal/engine`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/jobs_admin.go internal/api/jobs.go internal/engine/jobs_admin_test.go
git commit -m "feat(audit): audit background job administrative actions (PRD-06)"
```

---

### Task 6: Configuration, Retention, and CLI Tooling (`ddcore audit`)

**Files:**
- Modify: `internal/config/ops.go`
- Modify: `internal/engine/ops.go`
- Create: `cmd/ddcore/audit.go`
- Modify: `cmd/ddcore/main.go`
- Create: `docs/agent/audit.md`
- Modify: `docs/agent/cli.md`
- Modify: `ROADMAP.md`

**Interfaces:**
- Produces: CLI commands `ddcore audit list` and `ddcore audit purge`.

- [ ] **Step 1: Add `AuditRetentionDays` to ops config**

In `internal/config/ops.go`:
- Add `AuditRetentionDays() int` reading `ops.auditRetentionDays` (default 0).

- [ ] **Step 2: Implement CLI subcommand `ddcore audit`**

Create `cmd/ddcore/audit.go`:
- `list`: `--action`, `--actor`, `--target`, `--outcome`, `--limit` with tabular output.
- `purge`: `--days`, `--dry-run` calling `PurgeAuditEvents`.
Register `audit` in `cmd/ddcore/main.go`.

- [ ] **Step 3: Add documentation in `docs/agent/audit.md` and update `docs/agent/cli.md` and `ROADMAP.md`**

Document audit taxonomy, immutability, redaction guarantees, CLI usage, and update PRD-06 status in `ROADMAP.md`.

- [ ] **Step 4: Commit**

```bash
git add cmd/ddcore/ internal/config/ docs/ ROADMAP.md
git commit -m "feat(cli): add ddcore audit list and purge commands (PRD-06)"
```

---

### Task 7: Full Verification (`make test`, `make check`, i18n)

**Files:**
- Modify: `core/translations/pt-BR.csv`

- [ ] **Step 1: Extract translations and run `make check`**

Run:
```bash
./bin/ddcore i18n extract --all --lang pt-BR
make check
```
Expected: PASS with 0 untranslated strings and clean typecheck.

- [ ] **Step 2: Run full test suite**

Run: `make test`
Expected: PASS (Go tests, acceptance tests, desk tests).

- [ ] **Step 3: Commit translations and final cleanup**

```bash
git add core/translations/
git commit -m "chore(i18n): update translations for audit coverage"
```
