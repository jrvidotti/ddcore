# Tenancy

A site with tenancy keeps several **tenants** in one database: customer organisations that
share the app and the server and never see each other's documents. Each tenant has its own
documents, ids, numbering series, unique values, Singles, users, files, jobs and webhooks.

It is the SaaS shape — one deployment, many customers. To separate companies or branches
*inside* one organisation, where some people see several of them, use `scopes` instead; scopes
also work inside a tenant.

```jsonc
// ddcore.json
{ "tenancy": true }
```

Then `ddcore migrate`. **It cannot be undone**: the migration re-keys every table, and a
binary started without `"tenancy": true` against that database refuses to load. Restart every
server and worker after the migration that turns it on.

## Spaces

Every row of a tenant-owned table carries the **space** it was written in: a tenant's id, or
the empty string — the **platform space**, where every row of a site already was before it
had tenants, and where `Admin` and `Guest` are.

Code always runs in exactly one space:

| Who | Space |
| --- | --- |
| A user | The user's tenant. A user belongs to one tenant, fixed when the account is created |
| `Admin`, and a System Manager whose account is in the platform space (an **operator**) | The platform space, until they *enter* a tenant |
| A background job | The space it was enqueued in |
| A scheduled method, a migration's fixtures, `afterInstall`, `afterMigrate` | The platform space |
| `Guest` | The platform space |

Nothing reads the documents of two spaces at once. The operator works in a tenant by entering
it: the tenant menu in the Desk's user menu, `--tenant` on the command line, the
`X-Tenant` header with an API key, or `ddcore.tenant.run` in server code.

A site with tenancy on and no tenant created behaves like a site without it: everything is in
the platform space.

## What belongs to a tenant

Every DocType, unless it says otherwise:

```ts
export default defineDoctype({
  name: "Country",
  shared: true, // one set of documents for the whole site
  fields: [/* … */],
});
```

A **shared** DocType has no tenant: every space reads the same documents, and only the
platform space writes them. Use it for reference data the operator maintains (countries,
units, tax tables). Rules, checked when the site loads:

- A shared DocType cannot have a `Link` to a tenant-owned one.
- A child DocType follows the DocTypes that use it; one used by both a shared and a
  tenant-owned DocType is refused — declare two.
- A virtual DocType is neither: each of its sources is confined on its own.
- A field cannot be named `tenant` on a tenant-owned DocType. It is a standard column, like
  `owner`: `doc.tenant` is readable, never writable, and never changes.

A shared DocType may be a Single (`isSingle: true, shared: true`): the site then has one such
document, which every space reads and the platform space maintains — settings the operator
keeps for everyone.

The secret of a `Vault` field on a shared DocType lives in the platform space, with its
document. Every space sees the field as configured, and server code in any space reads the
secret by naming it as shared; only the platform space changes it, by saving the document:

```ts
// inside a tenant: the certificate the operator keeps in the shared "Gateway Settings" Single
const pfx = ddcore.vault.get("Gateway Settings:pfx:singleton", { shared: true });
```

Of the Core DocTypes, `Role` and `Site Tenant` are shared; everything else (`User`, `File`,
`Comment`, `Version`, `ToDo`, `API Key`, `User Permission`, `Document Share`, `Webhook`,
`Webhook Delivery`, `Email Delivery`, `Audit Event`, `Error Log`, `Letter Head`) is the
tenant's own.

Per tenant, as a consequence:

- **Ids and `unique`.** Two tenants can both have the customer `Acme` and the order
  `ORD-2026-0001`. `unique` and `uniqueKeys` hold within the tenant.
- **Naming series** count separately.
- **Singles** hold one document per tenant (and one for the platform space), unless the
  Single is shared.
- **`ddcore.vault`** secrets and `Vault` fields, unless the field is on a shared DocType.
  `ddcore.vault.get(name, { shared: true })` reads the platform space's secret from any
  space; see [vault](vault.md).
- **`ddcore.cache`** keys: a value one tenant stored is not returned to another.
- **`ddcore.db.lock`** keys: two tenants locking the same key do not wait for each other. A
  lock that must hold across the whole site is taken from the platform space.
- **Realtime events** reach the sessions of the tenant they happened in. A change to a shared
  DocType reaches everyone.

One thing is site-wide: a **user's e-mail**. Sign-in finds the account by address before it
knows any tenant, so an address registered in one tenant is refused in another ("User already
exists").

## The `Site Tenant` DocType

The tenants are documents of the Core DocType `Site Tenant` (labelled "Tenant" in the Desk). It
exists only on a site with tenancy, and is not called `Tenant` so that an app about leases can
keep that name for its own DocType.

| Field | Meaning |
| --- | --- |
| `slug` | The tenant's id: lowercase letters, digits, `-` and `_`, starting with a letter or digit, at most 63 characters. It cannot change |
| `title` | Its name, shown in the Desk |
| `enabled` | A disabled tenant refuses sign-ins and requests, and its queued jobs wait |

Only the platform space reaches `Site Tenant` documents; inside a tenant every access is refused.
A tenant's users learn their own tenant's id and title from the Desk and nothing else.

Creating a tenant, by any path, runs each app's `onTenantCreate` inside it:

```ts
export default defineApp({
  name: "shop",
  // Fixtures and afterInstall fill the platform space only. This is where an app gives
  // a new tenant the records it cannot start without.
  onTenantCreate() {
    ddcore.newDoc("Price List", { title: "Standard" }).insert();
  },
});
```

## Command line

```bash
ddcore tenant create acme --title "Acme Ltd" --admin boss@acme.example   # invites its first System Manager
ddcore tenant list
ddcore tenant disable acme        # and: enable
ddcore tenant adopt acme          # every row of the platform space moves into the tenant

ddcore --tenant acme eval 'ddcore.db.count("Customer")'
ddcore --tenant acme user add ana@acme.example "Ana" --role "Sales User"
ddcore --tenant acme export Customer
```

`--tenant` goes **before** the command and is for commands that do one thing and exit; a
server, a worker and `mcp` refuse it. `ddcore apikey` and `ddcore user passwd|reset` find the
account's tenant themselves. `ddcore import` (site-to-site) loads into the platform space and
refuses `--tenant`.

The operator's other commands follow the same rule: `ddcore jobs`, `ddcore audit list` and
`ddcore webhooks replay` work in the platform space unless `--tenant` names one. `ddcore
webhooks list`, `ddcore doctor`, `ddcore backup` and the retention sweeps cover the whole site.

`tenant adopt` is the path for a site that had one customer before it had tenancy: turn
tenancy on, migrate, create the tenant, adopt. Everything moves except the `Admin` and `Guest`
accounts and the secrets of `Vault` fields on shared DocTypes, which stay with their documents;
it is one transaction, and a row whose id the tenant already uses stops it.

## Server code

```ts
ddcore.tenant.current();            // "acme", or "" in the platform space
ddcore.tenant.list();               // [{ id, title, enabled }] — platform space only
ddcore.tenant.run("acme", () => {   // platform space only; same transaction
  return ddcore.db.count("Customer");
});
```

Code inside a tenant needs none of this: every call of the document API, and
**`ddcore.db.sql`**, reads and writes that tenant's rows only. A report written in SQL needs
no tenant filter.

A scheduled method runs once, in the platform space. To work in each tenant, fan out:

```ts
export function nightly() {
  for (const t of ddcore.tenant.list()) {
    if (t.enabled) ddcore.tenant.run(t.id, () => closeTheDay());
  }
}
```

or enqueue from inside `run`: the job then runs in that tenant, in a transaction of its own.

**Background jobs** run in the space they were enqueued in, and stay there: a job still runs
with permissions ignored (see `scopes`), but not outside its tenant. `uniqueKey` is per tenant.

**`runAs`** (`ddcore.runAs`, `enqueue`'s `runAs`, a `scheduler` entry's `runAs`; see `scopes`)
names a user of the space the code is working in. Inside a tenant that is one of the tenant's
own users. The platform space cannot run as a tenant's user directly — to it that user does not
exist — so a scheduled method enters the tenant first, and a `{ method, runAs }` scheduler entry
only takes a user of the platform space:

```ts
// refused from the platform space: ana belongs to acme
ddcore.runAs("ana@acme.example", work);

ddcore.tenant.run("acme", () => ddcore.runAs("ana@acme.example", work));                  // now
ddcore.tenant.run("acme", () => ddcore.enqueue("app.services.x.work", {}, { runAs: "ana@acme.example" })); // or as a job
```

**Migration patches** are the exception. A patch runs as the database owner, because it may
run DDL, and so it sees every space: `ctx.sql("UPDATE …")` touches every tenant's rows, which
is usually what a backfill wants. To use the document API on a tenant's documents from a
patch, wrap it in `ddcore.tenant.run`; outside `run`, an id names a row in every tenant that
has one.

**Tests** (`ddcore test`) run as `Admin` in the platform space. `ddcore.test.asUser(user, fn)`
moves to the user's tenant for the duration of `fn`; `ddcore.tenant.run` enters one directly.

## Who administers what

A tenant's **System Manager** administers that tenant: its users and their roles, scopes,
shares, webhooks, API keys, audit events, its jobs, its Error Log. It is not the operator:

| Surface | A tenant's System Manager |
| --- | --- |
| `Site Tenant` documents, other tenants' anything | refused |
| Shared DocTypes (`Role` included) | read only |
| MCP over HTTP (`/mcp`) | refused — it runs code and SQL as the operator |
| `/api/health/report` | refused — it counts the whole site |
| Job administration (`/api/jobs`) | its own tenant's jobs |
| Entering a tenant | refused |

Site configuration — mail transport, storage, branding, language, currency, timezone, sign-in
policy — is the site's and applies to every tenant.

## HTTP

A request works in the tenant of its user. For an operator it works in the platform space, or
in the tenant the session entered (`POST /api/tenant/enter {"tenant": "acme"}`; an empty
tenant leaves), or — with an API key — in the tenant named by `X-Tenant`. The header is
ignored for anyone who is not an operator.

`GET /api/boot` carries `site.tenant`: `{ id, title, platform }`, plus `tenants` for an
operator. It is absent on a site without tenancy.

## How it is enforced

Tenant tables are keyed by `(tenant, id)` and carry a Postgres row-level-security policy:
a row is visible, and writable, only when its `tenant` equals the transaction's. The `tenant`
column's default is that same setting, so every INSERT is stamped by the database — the
engine's, an app's raw SQL, a child row, a Version.

The site's connections run as a role that owns nothing (`ddcore_tenant`, created by
`migrate`), because Postgres does not apply row-level security to a table's owner or to a
superuser. A connection that names no tenant therefore sees the platform space only. The
framework's own work across tenants — a migration, finding an account at sign-in, claiming a
job — uses a separate, small pool that is asked for by name in the code.

**The database role.** `migrate` creates the role and grants it what it needs, on every run.
On a managed Postgres where the site's login role may not create roles, create one yourself,
grant it to the login role, and name it:

```bash
# once, as an administrator of the cluster
CREATE ROLE ddcore_tenant NOLOGIN;
GRANT ddcore_tenant TO <the site's login role>;
```

```bash
DDCORE_TENANT_ROLE=ddcore_tenant   # only needed for another name
```

## Limits

- **Tenants are users, not app authors.** The wall holds against everything a signed-in user
  can do. App code is the operator's: a query that calls `set_config('ddcore.tenant', …)`
  can read another tenant, and nothing in the framework stops an app from doing so on purpose.
- **An e-mail registered in another tenant is refused**, which tells a tenant's administrator
  that the address has an account somewhere on the site.
- **One configuration for all tenants**: mail, storage, branding, language, currency, timezone.
- **Tenants are not chosen by hostname.** Everyone signs in at the same address.
- **`Guest` is in the platform space.** Anonymous pages see no tenant's documents; signed-in
  portals work, since a Website User belongs to a tenant.
- **Single sign-on** signs in accounts that exist. It does not create them (there is no tenant
  to put them in).
- **Files** are checked through their `File` document, which is the tenant's. A *public* file's
  URL is unguessable but answers anyone who has it, as on any site. Storage keys carry no tenant.
- **Backup and restore** are of the whole database. There is no per-tenant backup or quota.
- **MCP and `ddcore eval`** work in the platform space unless `--tenant` is given (`eval`) —
  the MCP server always does.
- A statement run by a migration patch outside `ddcore.tenant.run` sees every tenant.
