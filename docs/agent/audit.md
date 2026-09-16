# Administrative Audit Coverage

All sensitive administrative actions across the framework are recorded in a unified `Audit Event` log (`tab_audit_event`). The audit log is tamper-resistant, immutable via standard document APIs, sanitized against sensitive credential leakage, and configurable with operational retention policies.

---

## Action Taxonomy

| Action | Target | Description | Detail Payload |
|---|---|---|---|
| `role.assign` | `User` | Role added to user account | `{"role": "..."}` |
| `role.revoke` | `User` | Role removed from user account | `{"role": "..."}` |
| `permission.scope_grant` | `User` | User access scope granted (`User Permission` created or changed) | `{"allow": "...", "for_value": "...", "applicable_for": "..."}` |
| `permission.scope_revoke` | `User` | User access scope revoked (`User Permission` deleted or changed) | `{"allow": "...", "for_value": "..."}` |
| `account.enable` | `User` | User account enabled | `{"enabled": true}` |
| `account.disable` | `User` | User account disabled | `{"enabled": false}` |
| `account.invite` | `User` | User invitation sent | `{"fullName": "..."}` |
| `account.resend_invite` | `User` | User invitation resent | — |
| `account.reset_password` | `User` | Password reset initiated | — |
| `account.unlock` | `User` | User account unlocked after lockout | `{"cleared": N}` |
| `account.revoke_sessions`| `User` | All sessions of user revoked | — |
| `job.cancel` | `Job` | Background job cancelled | `{"method": "...", "queue": "...", "status": "..."}` |
| `job.retry` | `Job` | Failed/cancelled background job retried | `{"newId": N, "method": "...", "queue": "..."}` |
| `job.purge` | `Job` | Finished background jobs purged | `{"done": N, "failed": N, "dryRun": bool}` |
| `job.admin` | `Job` | Unauthorized job administration attempt | Outcome: `Denied` |
| `vault.read` | `Vault Secret` | Vault secret read | — |
| `vault.write` | `Vault Secret` | Vault secret stored or updated | — |
| `vault.delete` | `Vault Secret` | Vault secret deleted | — |
| `webhook.replay` | `Webhook Delivery` | Outgoing webhook redelivered | `{"webhook": "..."}` |
| `method.<path>` | — | Unauthorized attempt to call whitelisted method | Outcome: `Denied` |

---

## Security & Immutability Guarantees

1. **Document API Immutability:** Direct mutations on `Audit Event` via `ddcore.db`, HTTP API (`/api/resource/Audit Event`), or Go engine methods (`Insert`, `Save`, `Delete`) are strictly blocked with `PermissionError`. Only internal engine operations write audit entries.
2. **Payload Sanitization:** Any detail dictionary is inspected recursively before persistence. Keys matching sensitive patterns (`password`, `secret`, `token`, `key`, `hash`, `credential`, `auth`, `ciphertext`, `nonce`) are replaced with `[REDACTED]`. String values exceeding 500 characters are safely truncated.
3. **Transaction Safety:**
   - Permitted events (`Allowed`) are recorded on the caller's active database transaction so rolled-back operations do not leave false records.
   - Refused attempts (`Denied`, recorded via `c.AuditDenied`) are written immediately on the database connection pool so they survive transaction rollback.

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

- A value of `0` (or omitted) retains audit logs **forever**.
- A positive integer purges audit records older than `N` days via the scheduled daily sweep (`core.services.audit.sweep`).

---

## CLI Tooling

### `ddcore audit list`

Inspect audit events with optional filters:

```bash
# List the latest 20 audit events
ddcore audit list

# Filter by action, actor, or outcome
ddcore audit list --action role.assign --outcome Allowed
ddcore audit list --actor admin@example.com --since 7d --limit 50

# Output as JSON
ddcore audit list --json
```

### `ddcore audit purge`

Purge audit logs older than a retention threshold:

```bash
# Dry run: preview how many events would be purged
ddcore audit purge --days 90 --dry-run

# Execute purge
ddcore audit purge --days 90
```
