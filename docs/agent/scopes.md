# User access scopes

Role permissions decide *what kind* of document a user may touch. A user access scope
decides *which* documents: company A and not company B, one branch, one customer. Scopes
are data, not code. A System Manager grants them by creating `User Permission`
documents, and the engine enforces them on every read and write path. An app does not
need a `permissionQuery`/`hasPermission` hook to separate companies or units.

A scope only **narrows** access. A user still needs a role permission for the DocType, or
a share of the document (see `sharing`). A `User Permission` never grants access to anything.

Scopes choose documents. To hide *fields* inside a document the user may read, see
`field-permissions`.

## The `User Permission` DocType

`User Permission` is a Core DocType stored in `tab_user_permission`. Only `Administrator`
and a `System Manager` without scope rows of their own may read or change it (see "Scope
administration" under "Enforced surfaces").

| Field | Type | Meaning |
| --- | --- | --- |
| `user` | Link (`User`) | The restricted user |
| `allow` | Data | The DocType that defines the scope, e.g. `Company` |
| `for_value` | Data | The `name` of an allowed document of that DocType |
| `applicable_for` | Data | Optional. When set, the rule applies only to that DocType; when empty, to every DocType |
| `is_default` | Check | Stored for Desk defaults. The framework does not read it yet |

`allow`, `for_value` and `applicable_for` are plain `Data` fields. Nothing validates that
the DocType or the document exists, so a typo gives a scope that matches no document.

```ts
// Restrict ana@example.com to the companies "Alfa" and "Gama".
ddcore.newDoc("User Permission", { user: "ana@example.com", allow: "Company", for_value: "Alfa" }).insert();
ddcore.newDoc("User Permission", { user: "ana@example.com", allow: "Company", for_value: "Gama" }).insert();
```

## How a rule matches a document

A user's rules are grouped by `allow`, after dropping those whose `applicable_for` names a
different DocType.

- **Values of one `allow` are OR-ed.** Two `Company` rules allow either company.
- **Different `allow` DocTypes are AND-ed.** With both `Company` and `Branch` rules, a
  document must satisfy both.
- **The scope DocType itself** is filtered on `name`. A user scoped to `Company: Alfa` lists
  and opens only the `Alfa` company.
- **A DocType that references the scope** is filtered on *every* `Link` field whose
  `options` is the `allow` DocType. With two such fields, both must hold allowed values.
- **Empty is out of scope.** A document whose restricted link is empty is hidden and
  rejected, so a user scoped to company A cannot see unassigned records.
- **A DocType with no link to the `allow` DocType** is not restricted by that rule.
- **Child tables**: writes and direct reads also check the `Link` fields of every child
  row, under the parent's `applicable_for`.
- **Dynamic Link**: a list query, a direct read or write, and a child row all restrict a
  `Dynamic Link` field only when its selector field holds the `allow` DocType. Rows pointing
  at other DocTypes pass.

## Who is unrestricted

- `Administrator`.
- Any user without `User Permission` rows. This is the default: scopes are opt-in per user.
- Background jobs, including jobs a scoped user enqueued (see "Background jobs" below).
- Framework-internal operations that raise the whole context to ignore permissions, such
  as the mail queue, attachment export and renames. The framework's own duplicate-name,
  rename and link checks also look names up without a scope, so saving a document that
  links to an out-of-scope name does not report the link as missing.

A `System Manager` **with** scope rows is scoped like anyone else. That includes history
and comment listings, which a System Manager can otherwise list unpinned, and `User
Permission` itself, which such a user cannot read or change.

## What app code sees

Scopes are applied below the SDK, so app code cannot opt out:

| Call | Role permissions | Scope |
| --- | --- | --- |
| `ddcore.db.getList`, `count` | enforced | enforced |
| `ddcore.db.getAll`, `getList({ ignorePermissions: true })`, `getValue`, `exists` (by name and by filters) | skipped | **enforced** |
| `ddcore.getDoc` | enforced | enforced (Link fields, Dynamic Link fields, child rows) |
| `insert` / `save` / `delete` / `submit` / `cancel`, with or without `ignorePermissions` | per option | **enforced** |
| `ddcore.db.setValue`, `doc.dbSet` | skipped | **enforced**: refused if the stored document, or the stored document with the new values, is out of scope |
| `ddcore.db.sql` | not applied | **not applied** |

`Webhook`, `Webhook Delivery`, `User Permission` and `Document Share` are closed to a user with access scopes
on every one of these calls except `ddcore.db.sql`: lists and `getAll` return no rows,
`getValue` returns nothing, `exists` returns `null`, and `insert`, `save`, `delete`,
`setValue` and `dbSet` are refused, with or without `ignorePermissions`.

A report that uses `ddcore.db.getList` inherits the scope. A report or service that uses
`ddcore.db.sql` must filter by scope itself.

A controller's `hasPermission` and `permissionQuery` still run. Their result is AND-ed
with the scope.

### Background jobs

A job runs as the user who enqueued it: `ddcore.session.user` and the actor of any audit
event it records are that user. Every job, however, runs with permissions ignored for its
whole context, so no role permission and no access scope applies inside it, even when a
scoped user enqueued it. A method a scoped user can enqueue therefore runs unscoped: app
code that enqueues work on behalf of a scoped user must filter by that user's scope itself,
for example by passing the in-scope names as arguments after reading them with
`ddcore.db.getList` in the request. Scheduled methods (`scheduler` in `defineApp`) run as
`Administrator` and are also unscoped.

## Enforced surfaces

| Surface | Behaviour |
| --- | --- |
| Lists, counts, link search | Scope filters are added to the query |
| Direct read (`GET /api/resource/...`) | `403` for an out-of-scope document |
| Insert | Refused if the new document is out of scope |
| Update | Refused if the stored document *or* the new values are out of scope, so a record cannot be moved into another company |
| Delete, submit, cancel, amend | Refused for out-of-scope documents |
| Export (`/api/export`) | Only in-scope documents are exported. `ddcore export` runs as `Administrator`, and is therefore unscoped, unless `--user` is passed |
| Files | An attached file is readable only if its document is; an unattached file stays with its owner and System Manager |
| Versions and comments | Refused unless the referenced document is readable |
| Realtime events (SSE) | Document events are delivered only to users who can read the document |
| Notifications | Recipient filtering, listing, counting, read-state changes and the email-send recheck all recheck access, so a scope change stops a new occurrence and hides or blocks an existing one |
| Scope administration | A scoped user is refused every permission on `User Permission`, including their own rows, and app code running as that user cannot reach them with `ignorePermissions`, `setValue` or `dbSet`. Scope administration belongs to a `System Manager` without scope rows, or `Administrator` |
| Document shares | A share of an out-of-scope document stays refused, unless it was given with **Override security scope** by an unscoped System Manager: that share lifts the scope for the rights it grants (read, write), never for delete, submit or cancel. A scoped user is refused `Document Share` itself (see `sharing`) |
| Webhooks | A scoped user is refused every permission on `Webhook` and `Webhook Delivery`, including replay, and app code running as that user cannot reach them with `ignorePermissions`: webhook administration is for unscoped users. The user's own document writes still queue and send deliveries (see `webhooks`) |

## Caching

Each user's rules are cached in the request context and in the engine cache. Creating,
updating or deleting a `User Permission` clears that user's cache, and their event
authorization cache, after the transaction commits. Direct SQL writes to
`tab_user_permission` bypass this, so manage scopes through the document API.

## Audit

Scope administration is recorded in `Audit Event` (see `audit`):

- `permission.scope_grant`: a `User Permission` was created. Target is the `User`, detail is `allow`, `for_value` and `applicable_for`.
- `permission.scope_revoke`: a `User Permission` was deleted. Detail is `allow` and `for_value`.

An update that changes `user`, `allow`, `for_value` or `applicable_for`, including
`dbSet`, is recorded as a revoke of the old rule followed by a grant of the new one.

## Limitations

- `ddcore.db.sql` ignores scopes.
- `is_default` is not used to prefill forms.
- Scope rules are per user. There are no scope groups or role-based scopes, and no Desk editor
  beyond the generic `User Permission` form.
- Background jobs run with permissions ignored, so a job a scoped user enqueued is unscoped.
- No automated test yet covers a report running under a scoped user.
- Link validation (checking that a Link field's value names an existing document) looks
  the name up without a scope, so it does not report an out-of-scope name as missing. Combined
  with direct access and `dbSet` returning `PermissionError` for an out-of-scope name but
  `NotFound` for one that does not exist at all, a scoped user who tries both can tell the two
  cases apart — a limited way to learn that an out-of-scope document exists.
- `setValue`/`dbSet` on a user's own `User` record is scope-checked like any other document. A
  scoped user whose own `User` record is itself out of scope (for example an app-added Link on
  `User` left empty, or an `allow: User` rule with no matching `for_value`) cannot change their
  language or profile through `core/services/profile.ts` or `core/services/i18n.ts`, which write
  through `setValue` — the same refusal `save` would give.
