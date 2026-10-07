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

A workspace, and each of its sidebar items, shortcuts and number cards, can name the space it
shows in with `space: "platform" | "tenant"`. Without it, it shows in both, unless what it opens
is a tenant-only DocType (below). See `report-api`.

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
units, tax tables). Inside a tenant the desk shows it read only, with a notice (and, for an
operator, a way back to the platform), because the permissions it receives already leave out
write, create, delete, submit, cancel, amend and import there. Rules, checked when the site loads:

- A shared DocType cannot have a `Link` to a tenant-owned one.
- A child DocType follows the DocTypes that use it; one used by both a shared and a
  tenant-owned DocType is refused — declare two.
- A virtual DocType is neither: each of its sources is confined on its own.
- A field cannot be named `tenant` on a tenant-owned DocType. It is a standard column, like
  `owner`: `doc.tenant` is readable, never writable, and never changes.

A shared DocType may be a Single (`isSingle: true, shared: true`): the site then has one such
document, which every space reads and the platform space maintains — settings the operator
keeps for everyone.

`shared` can change after tenancy is applied, and `ddcore migrate` reshapes the table (and
its child tables) either way. A DocType made shared loses the `tenant` column, the policy and
the `(tenant, id)` key, and its indexes no longer lead with the tenant; its documents are the
ones the platform space held. That is refused, with the tenants and their row counts, while any
row belongs to a tenant: move those rows to the platform space or delete them in a
`beforeSchema` patch ([migrations](migrations.md)), which runs before the plan is made. A
DocType made tenant-owned again gets the column back, with its rows in the platform space.

### Tenant-only DocTypes

A tenant-owned DocType is still reachable from the platform space. The operator works there
on its rows with `tenant = ''`, which no tenant ever sees. For a business DocType (an
employee, an invoice) that is never right. `space: "tenant"` says so:

```ts
export default defineDoctype({
  name: "Employee",
  space: "tenant", // lives inside a tenant: the platform space neither lists, reads nor writes it
  fields: [/* … */],
});

export default defineApp({
  name: "hr",
  title: "HR",
  space: "tenant", // the default of every DocType of the app
});
// …and a DocType of that app that the platform space needs too:
export default defineDoctype({ name: "Holiday", space: "any", fields: [/* … */] });
```

In the platform space, such a DocType:

- is left out of `GET /api/boot`, so the desk has no menu entry, search palette row or Link
  for it. A workspace item, shortcut, number card or grouped link that opens it, or a report
  on it, belongs to the tenants' space unless the item names a `space` of its own (see
  `report-api`). Reports on it are left out of boot too;
- is refused on every read and write (403): `/app/Employee`, `GET /api/meta/Employee`,
  `POST /api/resource/Employee`, MCP `insert_doc`, `ddcore eval` or `exec` without
  `--tenant`, a job enqueued there, a scheduled method that does not fan out, `afterInstall`,
  `afterMigrate`, a test that has not entered a tenant (see **Tests** below for the scratch
  tenant). The message is "Employee lives
  inside a tenant: enter one (on the command line, pass --tenant)". Permissions ignored do not
  change it;
- is left out of `ddcore import run` without `--tenant`, which lists it under the
  exclusions ("lives inside a tenant: load it with --tenant").

Some code still reaches it. Code inside a tenant does, and so does `ddcore.tenant.run`, which
enters one. A job enqueued from inside a tenant runs there. A **migration patch** reaches it
too: it sees every space. `ddcore.db.sql` in the platform space keeps seeing only the platform's
rows, which is none of them.

This holds from the moment tenancy is applied, before any tenant exists. A site with tenancy
and no tenant cannot use such a DocType until a tenant is created.

Rules, checked when the site loads, with tenancy on or off:

- `space` is `"tenant"` or `"any"` on a DocType, and `"tenant"` on an app.
- A shared DocType is in every space: `shared: true` with `space: "tenant"` is refused. An
  app's `space: "tenant"` leaves its shared DocTypes alone.
- A child DocType follows the DocTypes that use it, and a virtual DocType's sources are
  confined on their own: neither declares `space`.
- An app's `fixtures` cannot hold a tenant-only DocType: fixtures fill the platform space.
  Give each tenant those records in `onTenantCreate`.

`space` is the app's own: `extendDoctype` cannot change it.

### Shared DocTypes only server code reaches

A shared DocType is read by every tenant's users who hold a role on it, and a tenant's System
Manager hands out roles. Some platform data needs a different rule: every tenant's code reads
it and adds to it, and no tenant's user may browse it. A paid lookup cache is the typical case:
one lookup serves every tenant, and it holds names, addresses and phones. Declare it like this:

```ts
export default defineDoctype({
  name: "Lookup Cache",
  shared: true,
  tenantAccess: "server", // inside a tenant, only server code reaches it
  idGeneration: { field: "document" },
  fields: [/* … */],
});
```

Inside a tenant, **every client path is refused, whatever the role**, with "Lookup Cache is
kept by the platform: inside a tenant only server code reaches it". That covers:

- `GET /api/boot` (no menu entry, search row or Link);
- the desk, `/api/resource` and `/api/meta`;
- global and link search, and link titles (a tenant document's Link to it shows the id only);
- export, and reports on it;
- uploads to it;
- a tenant's Webhook on it.

A workspace entry that opens it, with no `space` of its own, shows in the platform space only.

**Server code** is a call that ignores permissions: `ddcore.db.getAll`, `db.getValue`,
`db.getSingleValue`, `db.exists`, `ddcore.getDoc(…, { ignorePermissions: true })`,
`db.getList({ ignorePermissions: true })`, jobs, and `onTenantCreate`. Such code reads the
platform's rows, the same ones in every tenant. Its **`insert` and `save` with
`ignorePermissions: true`** write them:

```ts
// services/api.ts, inside a tenant, as the tenant's system user
const hit = ddcore.db.getValue("Lookup Cache", doc, "data");
if (hit) return hit;
const data = provider.lookup(doc); // paid
ddcore.newDoc("Lookup Cache", { document: doc, data }).insert({ ignorePermissions: true });
return data;
```

Such a write runs in the **platform space**, on the caller's transaction. Its controller and
hooks run there, and so does everything it derives:

- the naming series counter and the `Version`;
- the `Audit Event`s;
- the secrets of its `Vault` fields;
- the platform's webhooks and notifications;
- the realtime events, which reach the platform's sessions only.

Its `Version` and audit events carry the tenant as `source_tenant`. The document's `owner` and
`modified_by` are the tenant's user, whose id is the site's. A tenant's code can only add to the
data and refresh it: `delete`, `db.setValue` and rename stay refused inside a tenant.

A `fetchFrom` on a tenant DocType that reads from it is server code too. The app chose to copy
that field, so its value lands in the tenant's document.

The platform space is unchanged: the operator reads and writes as the permissions say.

`tenantAccess` is only allowed on a shared DocType, its one value is `"server"`, and an
extension cannot change it.

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
tenant's own. `Feedback` is a tenant DocType too, but the framework writes every feedback in the
platform space, stamped with its `source_tenant` (see `feedback`).

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
- **`ddcore.db.lock`** and **`ddcore.db.tryLock`** keys: two tenants locking the same key do not
  wait for (or fail on) each other. A lock that must hold across the whole site is taken from the
  platform space.
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
ddcore tenant adopt acme --dry-run  # what would move, and what the tenant already has
ddcore tenant adopt acme          # every row of the platform space moves into the tenant

ddcore --tenant acme eval 'ddcore.db.count("Customer")'
ddcore --tenant acme user add ana@acme.example "Ana" --role "Sales User"
ddcore --tenant acme export Customer
ddcore import run /srv/export --tenant acme   # another site's export, loaded into the tenant
```

`--tenant` goes **before** the command and is for commands that do one thing and exit; a
server, a worker and `mcp` refuse it. `ddcore apikey` and `ddcore user passwd|reset` find the
account's tenant themselves, and so does `ddcore.users.createApiKey` called from the platform space. `ddcore import`
takes `--tenant` on either side of the command (see below).

The operator's other commands follow the same rule: `ddcore jobs`, `ddcore audit list` and
`ddcore webhooks replay` work in the platform space unless `--tenant` names one. `ddcore
webhooks list`, `ddcore doctor`, `ddcore backup` and the retention sweeps cover the whole site.

**Bringing a customer's data into a tenant.** Export it from where it is and load it with
`ddcore import run <dir> --tenant <slug>`: the documents, their ledger, their series and the
import's audit events land in the tenant, and nothing else on the site moves. It is the route
for a new customer arriving with another site's data, and for a site being split. A load into a
tenant leaves out the shared DocTypes (they are loaded once, from the platform space) and the
`Admin` and `Guest` accounts, and refuses an attachment whose url a file of another space
already uses, since storage keys are site-wide. The same export can be loaded into two
tenants; `import status` and `import reconcile` take the same `--tenant` (see `import`).

`tenant adopt` is the path for migrating a whole site that had one customer before it had
tenancy: turn tenancy on, migrate, create the tenant, adopt. Everything in the platform space moves, not just
documents: the Singles saved there, the `ddcore.vault` secrets, the numbering series, the
queued and running jobs, and the `Error Log`, `Audit Event`, `Version` and `Feedback` rows —
including the ones `maintenance on` and `backup` wrote during the cutover, which the tenant's
System Managers can then read. What stays is the `Admin` and `Guest` accounts (with their
roles and API keys), the documents of shared DocTypes and the secrets of their `Vault` fields,
and finished jobs.

It is one transaction. A row whose key the tenant already uses — an id, a `unique` value, a
series prefix, a Single the tenant has saved too — stops it with nothing moved, and the refusal
names each one. Run `--dry-run` first: it prints, table by table, how many rows would move and
every collision with the tenant (up to ten keys per unique index), moves nothing, and exits
non-zero when there is a collision.

The usual collision is a Single, or a vault secret, written in the platform space by something
that ran without `--tenant` — an `eval` that called an integration and cached its session, say —
while the tenant has its own. Decide which one is right: delete the platform one (from
`ddcore eval`, which works in the platform space) or the tenant's, then adopt.

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

**Tests** (`ddcore test`) run as `Admin`. Where they run depends on the app's
`tests.space`:

| `tests.space` | Where each test runs |
| --- | --- |
| `"platform"` | The platform space. This is the default |
| `"tenant"` | Inside the run's scratch tenant. This is the default for an app with `space: "tenant"` |

```ts
export default defineApp({
  name: "training",
  title: "Training",
  tests: { space: "tenant" }, // its tests create hr's tenant-only Employees
});
```

The **scratch tenant** is a tenant the run creates (`ddcore-test-<random>`) in its
rolled-back transaction, so it never reaches the site:

- Every app's `onTenantCreate` is applied to it, as it is to a real tenant.
- `Admin` is entered into it.
- Each test starts from the same state, because the test's writes roll back.
- No real tenant's documents are in sight, and `ddcore.tenant.current()` names it.
- `ddcore.test.asUser(user, fn)` works for `Admin` and for the users a test creates there,
  and `ddcore.getRoles(user)` sees the roles of a user the test just inserted.
- A `Link` to `User` may record `ddcore.session.user`, which is `Admin` (see
  [Who administers what](#who-administers-what)).

From such a test, `ddcore.test.inPlatform(fn)` runs `fn` in the platform space and comes back
afterwards, even when `fn` throws. It is how a tenant test writes a shared DocType, creates a
`Site Tenant`, acts as a platform user or exercises a scheduled fan-out
(`ddcore.tenant.list()` there includes the scratch tenant).

The tests of every other app run in the platform space. There, `ddcore.test.asUser` moves to
the user's tenant for the duration of `fn`, and `ddcore.tenant.run` enters a tenant directly.
Without tenancy, `tests.space` is ignored and `inPlatform` just runs `fn`.

## Who administers what

A tenant's **System Manager** administers that tenant: its users and their roles, scopes,
shares, webhooks, API keys, audit events, its jobs, its Error Log. It is not the operator:

| Surface | A tenant's System Manager |
| --- | --- |
| `Site Tenant` documents, other tenants' anything | refused |
| Shared DocTypes (`Role` included) | read only |
| Shared DocTypes with `tenantAccess: "server"` | refused (its app's server code reads and adds to them) |
| MCP over HTTP (`/mcp`) | refused — it runs code and SQL as the operator |
| `/api/health/report` | refused — it counts the whole site |
| Job administration (`/api/jobs`) | its own tenant's jobs |
| Entering a tenant | refused |

Site configuration — mail transport, storage, branding, language, currency, timezone, sign-in
policy — is the site's and applies to every tenant.

An operator who entered a tenant has no `User` there, and is still who did what. Inside a
tenant a `Link` (or `Dynamic Link`) to `User` therefore also accepts an operator: `Admin`, or a
System Manager of the platform space. So code that records `ddcore.session.user` works for the
operator too, and the desk shows the operator's full name. Any operator is accepted, not only
the one acting, so a tenant's user can save the document later. A platform user who is not an
operator is still refused.

## HTTP

A request works in the tenant of its user. For an operator it works in the platform space, or
in the tenant the session entered (`POST /api/tenant/enter {"tenant": "acme"}`; an empty
tenant leaves), or — with an API key — in the tenant named by `X-Tenant`. The header is
ignored for anyone who is not an operator.

`GET /api/boot` carries `site.tenant`: `{ id, title, platform }`, plus `tenants` for an
operator. It is absent on a site without tenancy.

Desk URLs name their tenant: inside a tenant the desk keeps `?tenant=<id>` on every `/app`
address, so a copied link says where its record lives (ids are only unique within a tenant).
Opening a link whose `tenant` is not the space the session works in shows a notice instead of
the page. An operator may **Enter** that tenant — the session moves, and every open tab with
it — or **Stay** where they are; a tenant's own user is told the link belongs to another
tenant. A link without `tenant`, or one from the platform space, opens as before.

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
- **Tenants are not chosen by hostname.** Everyone signs in at the same address. The one
  place a person picks a tenant is an app's credential provider (`auth`, "Sign-in through an
  app"): its tab lists the tenants whose `enabled()` says so, and that list is public.
- **`Guest` is in the platform space.** Anonymous pages see no tenant's documents; signed-in
  portals work, since a Website User belongs to a tenant.
- **Single sign-on** and **credential providers** sign in accounts that exist. They do not
  create them (single sign-on has no tenant to put them in).
- **Files** are checked through their `File` document, which is the tenant's. A *public* file's
  URL is unguessable but answers anyone who has it, as on any site. Storage keys carry no tenant.
- **Backup and restore** are of the whole database. There is no per-tenant backup or quota.
- **MCP and `ddcore eval`** work in the platform space unless `--tenant` is given (`eval`) —
  the MCP server always does, so it reaches a tenant-only DocType only through `eval` and
  `ddcore.tenant.run`.
- A statement run by a migration patch outside `ddcore.tenant.run` sees every tenant.
