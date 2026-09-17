# Administrative Audit Coverage

Sensitive administrative actions across the framework are recorded in a unified `Audit Event` log (`tab_audit_event`): who did what to which target, whether it was allowed or denied, and when. The document API — `Insert`, `Save`, `Delete` and `DBSet`, whether reached through `ddcore.db`, the HTTP API or the Go engine directly — cannot alter or remove a row once written. The log is sanitized against credential leakage, and subject to an operational retention policy that can still purge or, through a migration patch's `ctx.sql`, rewrite rows (see "Retention" and "CLI Tooling" below).

---

## Action Taxonomy

| Action | Target | Description | Detail Payload |
|---|---|---|---|
| `role.assign` | `User` | Role added to a user account. Fires once per role already present when a `User` is inserted, and again for each role added on a later save. | `{"role": "..."}` |
| `role.revoke` | `User` | Role removed from a user account | `{"role": "..."}` |
| `permission.scope_grant` | `User` | User access scope granted (`User Permission` created or changed) | `{"allow": "...", "for_value": "...", "applicable_for": "..."}` |
| `permission.scope_revoke` | `User` | User access scope revoked (`User Permission` deleted or changed) | `{"allow": "...", "for_value": "..."}` |
| `permission.share_grant` | the document | Document shared with a user. `Denied` when the sharer lacks `share`, lacks `write` for a write share, or may not override a scope | `{"user": "...", "read": bool, "write": bool, "share": bool, "override_scope": bool}` |
| `permission.share_update` | the document | Rights of an existing share changed | same as `share_grant` |
| `permission.share_revoke` | the document | Share removed. `Denied` when the caller may not remove it | `{"user": "..."}` |
| `account.enable` | `User` | User account enabled | none |
| `account.disable` | `User` | User account disabled | none |
| `account.invite` | `User` | User invitation sent | `{"fullName": "..."}` |
| `account.resend_invite` | `User` | User invitation resent | — |
| `account.reset_password` | `User` | Password reset initiated | — |
| `account.unlock` | `User` | User account unlocked after lockout | `{"cleared": N}` |
| `account.revoke_sessions`| `User` | All sessions of user revoked | — |
| `workflow.transition` | the document | A workflow action was applied or refused. `Allowed` on success; `Denied` for the wrong role, a self-approval attempt, or a failed condition. | `{"action": "...", "from_state": "...", "to_state": "..."}` |
| `job.cancel` | `Job` | Background job cancelled | `{"method": "...", "queue": "...", "status": "..."}` |
| `job.retry` | `Job` | Failed/cancelled background job retried | `{"newId": N, "method": "...", "queue": "..."}` |
| `job.purge` | `Job` | Finished background jobs purged | `{"done": N, "failed": N, "dryRun": bool, "doneDays": N, "failedDays": N}` |
| `job.admin` | `Job` | Unauthorized job administration attempt | Outcome: `Denied` |
| `vault.read` | `Vault Secret` | Vault secret read | — |
| `vault.write` | `Vault Secret` | Vault secret stored or updated | — |
| `vault.delete` | `Vault Secret` | Vault secret deleted | — |
| `webhook.replay` | `Webhook Delivery` | Outgoing webhook redelivered. `Denied` when the caller is not a System Manager, or is a System Manager with access scopes. | `{"webhook": "...", "previous_status": "..."}` |
| `ops.maintenance_on` | none | Maintenance mode switched on (CLI, MCP) | `{"reason": "..."}` |
| `ops.maintenance_off` | none | Maintenance mode switched off | — |
| `backup.create` | none | `ddcore backup` ran. `Denied` records a failed run | `{"archive": "...", "bytes": N, "files": N, "uploaded": bool, "error": "..."}` |
| `backup.restore` | none | `ddcore restore` restored an archive into this database | `{"archive": "...", "ddcore": "...", "started": "...", "files": N}` |
| `method.<path>` | none | A whitelisted method's `roles` option refused the caller | Outcome: `Denied` |

`permission.scope_grant` and `permission.scope_revoke` also fire from `DBSet`
on `User Permission`, since that is the path `dbSet`/`setValue` use to change
a scope in place.

## App-authored entries

Apps write to the same log through the SDK — the log is append-only, not
engine-only:

```ts
ddcore.audit(action: string, targetDoctype?: string, targetName?: string, detail?: Record<string, any>): void;
ddcore.auditDenied(action: string, targetDoctype?: string, targetName?: string, detail?: Record<string, any>): void;
```

`audit` records an allowed action on the caller's transaction; `auditDenied`
records a refused one directly on the pool. Both redact sensitive detail keys
the same way the engine's own entries are redacted. On the Go side these are
`(*Ctx).Audit` and `(*Ctx).AuditDenied`.

---

## Security & Immutability Guarantees

1. **Immutability:** `Insert`, `Save`, `Delete` and `DBSet` all refuse `Audit Event` with a `PermissionError`, whether the caller goes through `ddcore.db`, the HTTP API (`/api/resource/Audit Event`), or the Go engine directly. That is the only path closed: the retention sweep (`ops.auditRetentionDays`), `ddcore audit purge`, and a migration patch's `ctx.sql` can still remove or rewrite rows (see "Retention Policy" and "CLI Tooling").
2. **Payload Sanitization:** Any detail dictionary is inspected recursively before persistence. Keys matching sensitive patterns (`password`, `secret`, `token`, `key`, `hash`, `credential`, `auth`, `ciphertext`, `nonce`) are replaced with `[REDACTED]`. String values exceeding 500 characters are truncated.
3. **Transaction Safety:**
   - Permitted events (`Allowed`) are recorded on the caller's active database transaction so a rolled-back operation does not leave a false record.
   - Refused attempts (`Denied`, recorded via `c.AuditDenied`) are written immediately on the database connection pool so they survive the transaction rollback that follows.
   - `job.cancel`, `job.retry` and `job.purge` are the exception among allowed events: `RecordAudit` writes them directly on the pool, outside the caller's transaction, because job administration in `jobs_admin.go` does not run inside one.
   - Vault access recorded with no request context in scope (the internal case in `vault.go`, not routed through a `Ctx`) is written on the pool with actor `System`.

---

## Retention Policy

Audit retention is configured in `ddcore.json` under `ops`:

```json
{
  "ops": {
    "auditRetentionDays": 90
  }
}
```

- A value of `0` (or omitted) retains audit logs **forever**. A negative value is refused at boot.
- A positive integer purges audit records older than `N` days via `core.services.audit.sweep`, which runs daily — but only on a site that has `"scheduler": true` in `ddcore.json`. On a site with the scheduler off, nothing purges audit events on its own; use `ddcore audit purge`.

---

## CLI Tooling

### `ddcore audit list`

Inspect audit events with optional filters:

```bash
# List the latest 20 audit events
ddcore audit list

# Filter by action, actor, target, or outcome
ddcore audit list --action role.assign --outcome Allowed
ddcore audit list --actor admin@example.com --since 7d --limit 50
ddcore audit list --target user@example.com

# Output as JSON
ddcore audit list --json
```

`--action` is an exact match, not a prefix. `--target` matches the target
name (`target_name`), not the target doctype.

### `ddcore audit purge`

Purge audit logs older than a retention threshold:

```bash
# Dry run: preview how many events would be purged
ddcore audit purge --days 90 --dry-run

# Execute purge
ddcore audit purge --days 90
```

`--days` defaults to `ops.auditRetentionDays` when omitted. If neither is set
— `--days` is left out and `ops.auditRetentionDays` is `0` or unset — the
command errors instead of purging, because `0` means "keep forever" and a
purge with no window would be a mistake, not a decision.
