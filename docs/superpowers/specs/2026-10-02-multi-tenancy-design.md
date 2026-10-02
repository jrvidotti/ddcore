# Design: Row-level multi-tenancy (PRD-08)

Record of the design agreed on 2026-10-02. The contract for app authors and operators is
[`docs/agent/tenancy.md`](../../agent/tenancy.md); this document keeps **why** each piece is
the way it is.

## Starting point

Until now a ddcore instance held one tenant, and PRD-08 deferred anything else. The request
is a SaaS shape: several customer organisations in **one database**, none of which may learn
that the others exist. Decisions taken with the owner of the request:

| Question | Decision |
| --- | --- |
| Shape | A tenant per row in a shared database; not multisite, not instance provisioning |
| Users | A user belongs to exactly one tenant, fixed. E-mail stays unique site-wide |
| Guarantee | Engine enforcement backed by Postgres row-level security |
| Coverage | Every app DocType is tenant-owned unless it declares `shared: true` |
| Operator | Every context is confined to one space, Admin's included; Admin *enters* a tenant |
| Uniqueness | Ids, `unique` fields, naming series and Singles are per tenant |

User access scopes (SEC-01) are untouched. They narrow what a user sees *inside* a tenant
and are opt-in per user; tenancy is the wall *between* tenants and is not opt-in.

## Key decisions

### 1. An opt-in site switch, with no way back

`"tenancy": true` in `ddcore.json`. Off, nothing changes: no column, no policy, the same
schema plan (held by a golden test). On, migrate rewrites keys, and the old statements can no
longer address rows, so a marker in the database makes a binary started with the switch off
refuse to load.

### 2. Spaces, and one space per context

A tenant id is a slug. The empty string is the **platform space**: where every row of an
existing site already lives, and where Admin and Guest are. A site with tenancy on and no
tenant therefore behaves as it did.

Every `Ctx` works in exactly one space. No engine path reads documents of two spaces at
once, which is what makes isolation provable: there is no "all tenants" mode whose every
by-id statement would have to disambiguate. The operator enters a tenant (Desk switcher,
`--tenant`, `ddcore.tenant.run`) to work in it.

### 3. `tenant` is a standard column, not a field

Injected as a `meta.Field` (the `isTree` precedent) it would be written by `columnValues`
from whatever the client sent, trip `setOnlyOnce` on every save that omits it, be validated
as a Link against a DocType the tenant may not read, be alterable by app extensions, and show
up in typings, exports, import mappings and translations of every DocType.

As a standard column, like `owner`, the engine never writes it. Its default,
`coalesce(current_setting('ddcore.tenant', true), '')`, makes Postgres stamp every INSERT —
the engine's, raw SQL, Version rows, child rows — and no UPDATE the engine issues names it,
so it is immutable without a rule.

### 4. Row-level security that fails closed

Each tenant table has one policy: `tenant = coalesce(current_setting('ddcore.tenant', true), '')`
for both `USING` and `WITH CHECK`.

The table owner bypasses RLS, and so does a superuser, which the dev database's login role
is. Rather than run as owner and confine selected transactions, the pool's `AfterConnect`
runs `SET ROLE` to a confined NOLOGIN role, so **every** statement, including the many the
framework issues directly on the pool, is confined to the platform space unless a transaction
names a tenant. Work that must cross spaces (migrate, login lookup, job claim, mail and
webhook workers) elevates explicitly with `SET LOCAL ROLE NONE`. A call site somebody forgot
is then a failing test, not a silent leak.

`SET LOCAL ddcore.tenant` is a utility statement on purpose: export sets its isolation level
as the first statement of its transaction, and a `SELECT set_config(...)` before it would
take a snapshot.

The facts this rests on are pinned by `TestTenancySpike` in `internal/db`.

### 5. Composite keys

With `PRIMARY KEY (id)`, two tenants could not both have a customer "Acme" or an order
`PED-2026-0001`; the refusal would reveal the other's record; `ON CONFLICT (id)` could land
on a row the writer cannot see; and a Single could hold one row for the whole site. Tenant
tables therefore use `PRIMARY KEY (tenant, id)`, and unique indexes lead with `tenant`.

`tab_user` keeps `PRIMARY KEY (id)`: login looks a user up by e-mail before any tenant is
known, and sessions, API keys, tokens and identity links are all keyed by user. The cost is
that a tenant administrator who invites an e-mail registered elsewhere is told it exists.

Consequence, kept as a rule: **no document statement runs elevated without `AND tenant = $n`.**

### 6. Shared DocTypes

`shared: true` keeps a DocType out of all this: no column, global ids, readable from every
space, writable only from the platform space. It may not link to a tenant-owned DocType.
`Role` is shared. `Tenant` is shared and refused to every context outside the platform space.

### 7. Jobs, events, caches

A job records the space it was enqueued in and runs confined to it. It still runs with
permissions ignored, but RLS holds, which closes for tenants the gap scopes document for
jobs. Scheduled methods run once, in the platform space, and fan out themselves.

Realtime events and their subscribers carry a tenant. App cache keys are namespaced by space.

### 8. A tenant's System Manager is not the operator

Role checks that meant "site administrator" (MCP over HTTP, jobs administration, the health
report, site import, webhook replay) additionally require the platform space, or are
filtered by tenant.

## Threat model and limits

Tenants are **users**, never app authors. `ddcore.db.sql` is read-only and single-statement,
but a query can call `set_config('ddcore.tenant', ...)`; app code is the operator's and is
trusted not to.

Out of scope for this version: per-tenant branding, mail and timezone; choosing the tenant by
hostname; anonymous portals per tenant; per-tenant backup, export and quotas; storage keys
prefixed by tenant; SSO auto-provisioning of users (there is no tenant to place them in).
