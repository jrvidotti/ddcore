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
name that maps to `DDCORE_SECRET_KEY` (or to `DDCORE_SECRET_KEY_PREVIOUS`, below),
so the master key is unreachable from app code even though it shares the same
environment prefix as an app secret.

### Rotating the master key

When the key leaks — an environment dump, a former operator, a compromised CI or
hosting account — replace it without losing a secret and without a window in which
the server cannot read one:

1. Deploy with the new key in `DDCORE_SECRET_KEY` and the old one in
   `DDCORE_SECRET_KEY_PREVIOUS`. A read that does not open under the current key is
   retried under the previous one; every write uses the current one. Nothing visible
   changes.
2. Run `ddcore vault rekey` (or the MCP tool `vault_rekey`). It re-encrypts under the
   current key every secret not yet written with it, in every tenant, in batches of
   100 (`--batch N`), each batch one transaction. `--dry-run` only counts.
3. Run `ddcore doctor`. Once no secret is under the previous key it says so; remove
   `DDCORE_SECRET_KEY_PREVIOUS` and deploy again.

Every row of `ddcore_vault` records which key encrypted it in `key_id`: a 16-hex-digit
fingerprint of the derived key (`HMAC-SHA256(key, "ddcore-vault-key-id")`, first 8
bytes), from which neither key can be recovered. A row written before fingerprints
existed has none and is taken as encrypted under whichever key opens it; the first
`rekey` after upgrading stamps it, previous key set or not.

A secret that opens under neither key stops the re-key, named (with its tenant on a
site with tenancy) and never shown; the batches already committed stay moved. Delete
it or set it again — in its own space: `ddcore --tenant <slug> eval
'ddcore.vault.del("<name>")' --commit` — or provide the key it was written with, then
run `rekey` again. `--dry-run` lists every such secret first.

The re-key is recorded as one `vault.rekey` Audit Event in the platform space, whose
`detail` has counts only — moved, under the current key, under the previous key,
unreadable — never a name or a value.

With several replicas, finish step 1 on every one before step 2: a replica still
running with the old key alone writes under it, and the doctor would show those rows
under the previous key again (run `rekey` once more). Backups taken before step 2 hold
secrets under the old key: keep it with them, and after restoring one, set it as
`DDCORE_SECRET_KEY_PREVIOUS` and re-key the restored copy the same way.

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

### With tenancy

On a site with [tenancy](tenancy.md) each space has its own secrets: the same name is a
different secret in each tenant and in the platform space, and the calls above reach only the
secrets of the space the code runs in.

`{ shared: true }` names the platform space's secret instead, from whatever space the code
runs in — a credential the operator keeps once for the whole site:

```ts
const pfx = ddcore.vault.get("Gateway Settings:pfx:singleton", { shared: true });
```

- `get(name, { shared: true })` works in every space. It does not fall back to the space's own
  secret of that name, nor the other way around.
- `set(name, value, { shared: true })` and `del(name, { shared: true })` are refused inside a
  tenant (403); in the platform space they are the plain calls.
- Without tenancy the option changes nothing.

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
5. **On a `shared` DocType (tenancy):** the secret is kept in the platform space, like the
   document. Every space sees the field as `{ "configured": true }`, and reads the value with
   `ddcore.vault.get(key, { shared: true })`; a plain `ddcore.vault.get(key)` inside a tenant
   looks in the tenant's own secrets and returns `null`. Only the platform space saves the
   document, so only it sets or clears the secret. Adopting the platform's rows into a tenant
   (`ddcore tenant adopt`) leaves these secrets where they are.

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
- `detail`: `{ "shared": true }` when a secret of the platform space was read with
  `{ shared: true }`. With tenancy the event is recorded in the space the read came from: a
  tenant's read of a shared secret is in that tenant's audit log
  (`ddcore audit list --tenant <id>`), not in the platform's.

`ddcore vault rekey` records one `vault.rekey` event per run instead, with counts and no
names (see [Rotating the master key](#rotating-the-master-key)).

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

System Managers can review audit events in the Desk at `/app/AuditEvent` or via `ddcore audit list`.

---

## 6. Health & Diagnostics (`ddcore doctor`)

Run `ddcore doctor` to inspect the vault status:
```bash
ddcore doctor
```

The report indicates whether `DDCORE_SECRET_KEY` is configured, the total count of stored secrets, and their key names — values are never displayed, but the names themselves are printed, and a key can leak a detail (a document id, a tenant) worth keeping out of a report pasted into an issue.

It also opens every secret in memory, in every tenant, and counts them by the key that
opens it. During a rotation it prints that count and warns while secrets remain under
`DDCORE_SECRET_KEY_PREVIOUS` (run `ddcore vault rekey`), then that the previous key can
be removed once none do. Secrets that open under neither key are a warning too, with
their names, whether or not a rotation is under way.

---

## 7. Limitations

- **A rotation is two keys, not one.** Changing `DDCORE_SECRET_KEY` without setting the old one as `DDCORE_SECRET_KEY_PREVIOUS` makes every existing secret fail to decrypt; rotate as in [Rotating the master key](#rotating-the-master-key).
- **Backups are useless alone.** A database backup carries the ciphertext but not the key; restoring it without the key it was taken under (current or previous), provisioned separately, leaves every secret undecryptable.
- **No permission check inside `ddcore.vault.*`.** Server code — a controller, a service, a job — is trusted with any key it names; the boundary is that this API only exists on the server, never in desk-sdk.
- **`ddcore doctor` prints vault key names**, not values (see above).
- **Some renames still orphan a secret.** A default-shaped key (`<DocType>:<fieldname>:<id>`) is re-keyed on rename. A custom key template in `options`, and removing a child row during an update, are not: the secret stays in `ddcore_vault` under a key nothing reads anymore.
