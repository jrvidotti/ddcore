# Secret Vault

ddcore provides a dedicated, encrypted vault for credentials and secrets.

There are two kinds of secrets in an application:
1. **Site-level credentials** (e.g. Stripe webhook secret, SMTP password): fixed per deployment, configured via environment variables and accessed with `ddcore.secret("stripe_key")` (reads `DDCORE_SECRET_STRIPE_KEY`).
2. **Per-record dynamic credentials** (e.g. multi-tenant API tokens, customer credentials, OAuth refresh tokens): stored encrypted in the vault and linked to documents via the `Vault` fieldtype or managed directly via `ddcore.vault.*`.

---

## 1. Master Encryption Key

The vault uses **AES-256-GCM** authenticated encryption. The encryption key is derived from the `DDCORE_SECRET_KEY` environment variable:

```bash
# In production: provide a 32-byte (64 hex characters or 32 raw bytes) key
export DDCORE_SECRET_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
```

If `DDCORE_SECRET_KEY` is not set, vault operations fail immediately with a configuration error, preventing plaintext storage.

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
    { fieldname: "custom_token", fieldtype: "Vault", label: "Custom Token", options: "custom:token:{name}" },
  ],
});
```

### Key properties of `Vault` fields:
1. **Virtual field (no column):** `Vault` fields never generate a column in `tab_<doctype>`. They are persisted exclusively in the encrypted `ddcore_vault` table.
2. **Key derivation:**
   - Default: `${DocType}:${fieldname}:${doc.name}` (e.g. `Integration Account:api_key:ACC-00001`).
   - Custom template in `options`: interpolates document fields using `{field}` placeholders (e.g. `options: "custom:token:{name}"`).
3. **Lifecycle on save:**
   - On `insert` or `save`, if a string value is passed, it is encrypted and saved in the vault under the derived key.
   - If `{ clear: true }` is passed, the secret is removed from the vault.
   - If `{ configured: true }`, `null`, or unchanged, the current vault secret is preserved.
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

Every read, write, and delete operation on the vault is automatically audited in the `Vault Audit Log` DocType:
- `secret_name`: Name of the vault key.
- `action`: `read`, `write`, or `delete`.
- `user`: User email who triggered the action.
- `ip`: Client IP address.
- `request_id`: Request correlation ID.

System Managers can review audit logs in the Desk under the **Core** module.

---

## 6. Health & Diagnostics (`ddcore doctor`)

Run `ddcore doctor` to inspect the vault status:
```bash
ddcore doctor
```

The report indicates whether `DDCORE_SECRET_KEY` is configured, the total count of stored secrets, and their key names (values are never displayed).
