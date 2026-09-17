# Design: Document Sharing (SEC-03)

This document explains **why** document sharing is shaped the way it is. The API reference
is [`docs/agent/sharing.md`](../../agent/sharing.md).

## Starting point

- **Access came from roles only.** Every grant covered a whole DocType: role rows,
  `ifOwner`, controller `hasPermission`/`permissionQuery` and workflow `allowEdit`. SEC-01
  scopes narrow the rows and SEC-02 levels narrow the fields. Nothing gave one user one
  document.
- **Assignments (OPS-05) deliberately grant nothing.** An assignee without a role could
  not open what they were assigned.
- **Permission checks were already centralised.** Single-document access funnels through
  `HasPermission`, and every list, count, link search and `exists`/`getValue` goes through
  `GetList` → `permissionFilters`/`scopeFilters`. Attachments, versions, comments, print,
  SSE and notifications ask `GetDoc`.
- **Filters could not OR.** `db.Filter` rendered as an AND of conditions. `orFilters` is a
  caller-level OR of single conditions and cannot express "role branch OR share branch".
- **The audit ledger (PRD-06) was waiting on SEC-03** for sharing events.

## Key Decisions

### 1. One Core DocType, one row per user and document

`Document Share` has `user`, `share_doctype`, `share_name`, the rights `read`/`write`/`share`
and `override_scope`, with a unique key on `(user, share_doctype, share_name)`. Its
reference columns join `coreRefs`, so a rename moves shares and a deletion removes them.

**Rationale:** these are the same plain-Data reference columns ToDo, File and Comment use,
so the rename and delete machinery already exists. One row per pair makes re-sharing an
update, not a second grant to reconcile.

### 2. A share stands in for the role grant inside `HasPermission`

When no level-0 role row grants `read`, `write` or `share`, or an `ifOwner` row fails on
someone else's document, a matching share grants it. The checks that follow are unchanged:
workflow `allowEdit`, the controller's `hasPermission`, and the scope check. With no
document, a doctype-level `read` (or `write`) answers yes when the user holds any such
share on that DocType, so list endpoints and the Desk meta open up. The rows are still
filtered.

**Rationale:** the controller, the workflow and the scopes keep their authority. A share
cannot write past a workflow state or an app's own veto. Every surface that already asks
`GetDoc` gets sharing for free.

### 3. Rights are read, write and share, and write is capped by the sharer

Submit, cancel, delete, amend, report and export stay role-only. A new `share` flag on a
level-0 role row lets a role share. A share with `share` lets its recipient re-share. The
sharer may hand out `write` only while holding `write` on that document.

**Rationale:** sharing has to be opt-in per DocType, and a share must never exceed its
sharer. Lifecycle actions (submit, cancel, amend) carry business meaning that belongs to
roles and workflows. Export and report are bulk surfaces, not "this document".

### 4. Lists OR a share branch into the permission filters

`db.Filter` gains an internal `Any [][]Filter` (groups OR-ed, filters within a group
AND-ed), rendered recursively by `filterSQL` and never produced by `ParseFilters`.
`permissionFilters` builds up to three groups:

- **Role branch:** owner filter, `permissionQuery` and scopes. Included only when a role
  grants read.
- **Plain shares:** `name IN (…)`, `permissionQuery` and scopes.
- **Overriding shares:** `name IN (…)` and `permissionQuery`, without scopes.

`scopeFilters` (used by `getAll`/`getValue`/`exists`) ORs in the overriding names in the same
way.

**Rationale:** one query keeps counts and pagination honest. A post-filter would leave holes
in pages. The OR stays inside the framework's own filters, so a caller's filters and field
checks are untouched.

### 5. Scopes still apply, unless the share overrides them

By default a share is bound by the recipient's User Permission scopes, like any role grant.
`override_scope` lifts the scope for the rights the share grants (read, write) on that one
document. It never lifts delete, submit or cancel, and never reopens the DocTypes closed to
scoped users. Only a System Manager without scope rows, `Administrator` or an elevated
context may set it.

**Rationale:** the product needs both. Scopes are an administrator's isolation boundary, so
by default a colleague must not be able to share across it. Sometimes the administrator
wants exactly one exception, and that exception belongs to someone who is not scoped
themselves. Requiring System Manager, and not merely "unscoped", keeps an ordinary unscoped
user from lifting a boundary somebody else set.

### 6. Field levels stay role-only

A share grants the document at level 0. `FieldAccess` is still computed from roles.

**Rationale:** SEC-02 keeps one authority per question. A share answers "this document",
and levels answer "which fields". Sharing a salary field is a different feature, if it is
ever needed.

### 7. Writes go through a service; audit and cache live on the DocType

`ShareDoc`/`UnshareDoc` check the sharer, then write the row under a raised context. No
role may write `Document Share` through the generic API, and a scoped user is refused the
DocType (`unscopedOnlyDoctypes`). Insert, Save, DBSet and Delete of the DocType record
`permission.share_grant`/`share_update`/`share_revoke` with the shared document as target,
and drop the recipient's share cache after commit. The writing ctx re-reads its own
transaction immediately.

**Rationale:** it mirrors `User Permission`. However a row changes, even by `Administrator`
through the resource API, audit and invalidation happen in one place. Denied attempts are
written on the pool, as for workflows.

### 8. Notifications reuse the assignment path

`NotifyUser` became `notifyUserAs(rule, …)`. A new share inserts a `share` notification in
the recipient's language, best effort inside a savepoint. The notification access check
runs as the recipient on a child ctx that loads shares from the transaction, so the
just-written share is visible.

**Rationale:** the inbox already rechecks access on every listing, so revocation hides the
notification with no extra code.

## Left Out

- Role or group shares, "share with everyone", and expiry.
- Sharing child rows on their own, Singles, and the unscoped-only DocTypes.
- A permission editor and role profiles (the demand-driven backlog SEC-03 item).
- Shares in `ddcore.db.sql`, link titles and background jobs, which already ignore scopes.
- Per-row audit of the cascade when a shared document is deleted.

## Verification Strategy

- **Engine tests (`internal/engine/share_test.go`):**
  - read/write/share semantics, the write cap and the controller veto;
  - lists, counts, link search, notifications, revocation and self-removal;
  - scopes with and without override: `exists`, `getValue`, save and `dbSet` allowed, delete refused;
  - override refused to a non-System Manager and to a scoped re-sharer;
  - scoped administration refused, rename and delete cascade, audit counts.
- **API test (`internal/api/shares_test.go`):**
  - every channel (document, list, versions, comments, print, private file, share list,
    SSE authorizer) closed before a share, open after it, and closed again after the
    revoke, including the cached event authorization;
  - permlevel field omission, strict JSON decoding, notifications and audit.
- **Desk:** vitest for the state class and sidebar helpers; `svelte-check`.
