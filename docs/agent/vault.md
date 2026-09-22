# Secret Vault

ddcore provides a dedicated, encrypted vault for credentials and secrets.

There are two kinds of secrets in an application:
1. **Site-level credentials** (e.g. Stripe webhook secret, SMTP password): fixed per deployment, configured via environment variables and accessed with `ddcore.secret("stripe_key")` (reads `DDCORE_SECRET_STRIPE_KEY`).
2. **Per-record dynamic credentials** (e.g. multi-tenant API tokens, customer credentials, OAuth refresh tokens): stored encrypted in the vault and linked to documents via the `Vault` fieldtype or managed directly via `ddcore.vault.*`.

---

## 1. Master Encryption Key

The vault uses **AES-256-GCM** authenticated encryption. The encryption key is derived from the `DDCORE_SECRET_KEY` environment variable by trimming it and hashing it with SHA-256 — any non-empty string works, hex is not decoded, and there is no length requirement:

```bash
export DDCORE_SECRET_KEY="a long, random value provisioned once per deployment"
```

If `DDCORE_SECRET_KEY` is not set, vault operations fail immediately with a configuration error, preventing plaintext storage.

`ddcore.secret("key")` cannot read this variable: `Engine.Secret` refuses any
name that maps to `DDCORE_SECRET_KEY`, so the master key is unreachable from
app code even though it shares the same environment prefix as an app secret.

---

## 2. Server-side API (`ddcore.vault`)

Server-side controllers, services, and background jobs have direct synchronous access to the vault:

```ts
// Store or update a secret
ddcore.vault.set("asaas:token:ACC-00042", "sk_live_abcdef123456");

// Retrieve and decrypt a secret (returns string | null)
const token = ddcore.vault.get("asaas:token:ACC-00042");
if (!token) {
  ddcore.throw(_("Integration token not configured"));
}

// Delete a secret
ddcore.vault.del("asaas:token:ACC-00042");

// List secret names matching a prefix (never returns values)
const keys = ddcore.vault.list("asaas:token:"); // ["asaas:token:ACC-00042", ...]
```

---

## 3. Fieldtype `Vault`

DocTypes can declare fields with `fieldtype: "Vault"`:

```ts
import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Integration Account",
  fields: [
    { fieldname: "account_name", fieldtype: "Data", label: "Account Name", reqd: true },
    { fieldname: "api_key", fieldtype: "Vault", label: "API Key", reqd: true },
    // Custom key naming using doc fields interpolation:
    { fieldname: "custom_token", fieldtype: "Vault", label: "Custom Token", options: "custom:token:{id}" },
  ],
});
```

### Key properties of `Vault` fields:
1. **Virtual field (no column):** `Vault` fields never generate a column in `tab_<doctype>`. They are persisted exclusively in the encrypted `ddcore_vault` table.
2. **Key derivation:**
   - Default: `${DocType}:${fieldname}:${doc.id}` (e.g. `Integration Account:api_key:ACC-00001`). Renaming the document re-keys this shape automatically.
   - Custom template in `options`: interpolates document fields using `{field}` placeholders, `{id}` for the document id (e.g. `options: "custom:token:{id}"`). `{name}` is refused unless the DocType declares a field named `name`. A rename does not re-key a custom template — see Limitations.
3. **Lifecycle on save:** a `Vault` field accepts one of a few shapes:
   - A non-empty string encrypts and saves it under the derived key.
   - An empty string, `null`, `{ configured: true }` (what a read gives back), or
     the field simply not appearing leaves the current vault secret untouched.
   - `{ value: "…" }` or `{ secret: "…" }` sets it, same as a plain string.
   - `{ clear: true }` removes it from the vault.
   - On document `delete`, associated vault entries are cleaned up automatically.
4. **Border security:**
   - Reading documents via REST API, Desk, or MCP never returns the plaintext secret. The field is redacted to `{ "configured": true }` if present, or `null` if not configured.
   - Excluded from `Version` diffs (audit history diffs will never leak secrets).
   - Excluded from data exports.

---

## 4. Desk UI Control

The Desk provides a dedicated control for `Vault` fields:
- When configured: shows a green `Configured` badge with **Change** (to enter a new secret) and **Clear** buttons.
- When entering a new secret: displays a masked password input.
- When marked for clearing: displays an indicator with an **Undo** button.

---

## 5. Audit Logging

`ddcore.vault.set`, `ddcore.vault.get`, and `ddcore.vault.del` are audited in the `Audit Event` DocType (PRD-06):
- `action`: `vault.write`, `vault.read`, or `vault.delete`.
- `target_doctype`: `Vault Secret`.
- `target_id`: the vault key.
- `actor`: User email who triggered the action (or `System`).
- `outcome`: `Allowed` (or `Denied`).
- `ip`: Client IP address.
- `request_id`: Request correlation ID.

Two paths are not audited:
- `ddcore.vault.list` never touches the audit log — it never returns a value,
  only key names.
- Framework code reading a key it owns on its own schedule, such as signing
  each webhook delivery attempt, reads the vault without recording an entry.
  An audit row per retry would bury the reads a person made under the ones a
  worker made. Anything an app or a person asks for through `ddcore.vault.get`
  still goes through the audited path.

Writing the audit entry itself is best-effort: if it fails, the vault
operation still succeeds and the failure is silently discarded.

System Managers can review audit events in the Desk at `/app/Audit Event` or via `ddcore audit list`.

---

## 6. Health & Diagnostics (`ddcore doctor`)

Run `ddcore doctor` to inspect the vault status:
```bash
ddcore doctor
```

The report indicates whether `DDCORE_SECRET_KEY` is configured, the total count of stored secrets, and their key names — values are never displayed, but the names themselves are printed, and a key can leak a detail (a document id, a tenant) worth keeping out of a report pasted into an issue.

---

## 7. Limitations

- **No key rotation.** A different `DDCORE_SECRET_KEY` makes every existing secret fail to decrypt; there is no re-encryption path. Doctor only warns when the key is missing, not when it has changed.
- **Backups are useless alone.** A database backup carries the ciphertext but not the key; restoring it without the same `DDCORE_SECRET_KEY`, provisioned separately, leaves every secret undecryptable.
- **No permission check inside `ddcore.vault.*`.** Server code — a controller, a service, a job — is trusted with any key it names; the boundary is that this API only exists on the server, never in desk-sdk.
- **`ddcore doctor` prints vault key names**, not values (see above).
- **Some renames still orphan a secret.** A default-shaped key (`<DocType>:<fieldname>:<id>`) is re-keyed on rename. A custom key template in `options`, and removing a child row during an update, are not: the secret stays in `ddcore_vault` under a key nothing reads anymore.
