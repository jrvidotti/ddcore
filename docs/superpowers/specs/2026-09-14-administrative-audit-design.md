# Design: administrative audit coverage (PRD-06)

Record of the design built on 2026-09-14. The contract for app authors and operators
is [`docs/agent/audit.md`](../../agent/audit.md); this document preserves **why** each
piece is designed the way it is.

## Starting point

Stage 1 of the roadmap introduced the minimum audit capability required by outgoing
webhooks (OPS-06): the `Audit Event` DocType, recording webhook replays (allowed and
denied). Meanwhile, Secret Vault had its own separate log DocType (`Vault Audit Log`).
Other sensitive administrative actions—modifying user roles, inviting or unlocking
accounts, revoking sessions, resetting passwords, and retrying, cancelling, or
purging background jobs—either left disparate traces in `Version` or `ddcore_job`, or
left no audit trail at all.

PRD-06 establishes unified administrative audit coverage across framework services:
1. Absorbs `Vault Audit Log` completely into `Audit Event`.
2. Instruments core administrative and authorization chokepoints.
3. Guarantees that secrets and sensitive payloads are never written to audit records.
4. Protects audit records from deletion or tampering via Desk/API.
5. Provides audit inspection and retention commands through the `ddcore` CLI.

## Decisions

### 1. Unified `Audit Event` as the single audit ledger
`Vault Audit Log` is completely replaced by `Audit Event`. Maintaining separate tables
for vault operations and other administrative events fragments audit trails and
complicates compliance inspection.
- The `Vault Audit Log` DocType is removed from `core/doctypes/vault_audit_log`.
- A migration patch (`core/patches/0001_absorb_vault_audit_log.ts`) migrates existing
  rows from `tab_vault_audit_log` into `tab_audit_event` and drops the old table.
- Operations in `internal/engine/vault.go` (`VaultGet`, `VaultSet`, `VaultDel`)
  call `c.Audit` with actions `vault.read`, `vault.write`, and `vault.delete`.
- Internal system reads (`vaultRead` used by webhooks to sign attempts) continue to skip
  auditing to prevent burying administrative reads under worker retry noise.

### 2. Standardized action taxonomy
Actions follow a hierarchical `<domain>.<verb>` format:
- **Vault**: `vault.read`, `vault.write`, `vault.delete`
- **Roles & Permissions**: `role.assign`, `role.revoke`
- **Account Administration**: `account.invite`, `account.resend_invite`, `account.reset_password`, `account.unlock`, `account.revoke_sessions`, `account.enable`, `account.disable`
- **Job Administration**: `job.cancel`, `job.retry`, `job.purge`
- **Deliveries**: `webhook.replay` (established in OPS-06)

Each event captures:
- `action`: the standardized action string
- `outcome`: `Allowed` or `Denied`
- `actor`: user email/identity (or `"System"`)
- `target_doctype`: target document type (e.g. `User`, `Vault Secret`, `Job`, `Webhook Delivery`)
- `target_name`: target document or entity identifier
- `ip`: client IP address from context
- `request_id`: request correlation ID
- `detail`: JSON map of identifiers and metadata (strictly sanitized)

### 3. Transactional duality (allowed on tx, denied on pool)
- **Allowed actions** are written on the caller's transaction: if the operation fails or
  rolls back, the action did not happen. A committed audit record for a rolled-back
  mutation would be a false statement.
- **Denied actions** (`c.AuditDenied`) are written directly to the database connection pool:
  when permission is refused, the caller's transaction rolls back with a `PermissionError`,
  but the refusal itself is the exact evidence an investigation needs to discover.

### 4. Automatic instrumentation at lifecycle chokepoints
- **User Roles (`Has Role`)**: In the `User` save lifecycle (`core/doctypes/user/user.controller.ts`
  or `internal/engine`), differences between incoming roles and the existing roles from
  `getDocBeforeSave()` are calculated. For each added role, emit `role.assign`; for each
  removed role, emit `role.revoke`, attributing the change to the acting user.
- **Account Operations**: In `core/services/users.ts`, `invite`, `resendInvite`,
  `sendPasswordReset`, `unlockUser`, and `revokeUserSessions` emit corresponding
  `account.*` audit events.
- **Job Administration**: In `internal/engine/jobs_admin.go` and `internal/api/jobs.go`,
  `CancelJob`, `RetryJob`, and `PurgeJobs` record audit entries. Unauthorized attempts
  trigger `AuditDenied`.

### 5. Sensitive payload redaction and data protection
`Audit Event` records must never leak secrets, passwords, or restricted payloads:
- `writeAudit` runs a deep sanitization pass on `detail`:
  - Any key matching sensitive patterns (`*password*`, `*secret*`, `*token*`, `*key*`,
    `*hash*`, `*credential*`, `*auth*`, `ciphertext`, `nonce`) has its value replaced
    with `"[REDACTED]"`.
  - Raw request bodies, entire documents, and long string values (> 500 characters)
    are truncated or omitted.
- Immutability: `Audit Event` DocType grants only `read: true` to `System Manager`.
  The engine rejects any attempt to insert, update, or delete `Audit Event` records
  via REST, RPC, or SDK APIs.

### 6. Retention and CLI tooling
- Audit events are retained indefinitely by default.
- An optional retention window can be configured in `ddcore.json` under `ops.auditRetentionDays`
  (default `0` = keep forever). If greater than zero, `ddcore doctor` / retention sweep
  can purge expired records.
- CLI subcommand `ddcore audit`:
  - `ddcore audit list`: lists recent audit events in tabular format with filters
    (`--action`, `--actor`, `--target`, `--outcome`, `--limit`).
  - `ddcore audit purge`: explicitly purges audit events older than `--days` (with optional `--dry-run`).
