# Document sharing

Role permissions decide what a user may do with *every* document of a DocType. A share
gives one user access to *one* document: a colleague who reviews a single contract, an
approver from another team who needs to edit one order. Shares are data, created at run
time, and the engine enforces them on the same paths as role permissions.

A share can grant three rights:

| Right | Grants |
| --- | --- |
| `read` | Opening, listing, counting, link search, attachments, comments, versions, print and realtime events for that document. Always granted |
| `write` | Saving the document (and `dbSet`/`setValue`). Implies `read` |
| `share` | Sharing the document with someone else, and removing its shares. Implies `read` |

Submit, cancel, delete, amend, report and export are never granted by a share. They stay
with role permissions. A user whose role already allows export exports every row they can
read, so the documents shared with them are included.

## Who may share

The sharer needs the **`share` right** on the document, either from a role permission
row or from a share of their own:

```ts
permissions: [
  { role: "Sales User", read: true, write: true, create: true, share: true },
],
```

`share` is a level-0 right: a row above level 0 may not grant it. No DocType is shareable
until a role gets `share: true`. `Administrator` and elevated contexts
(`ignorePermissions`, jobs) can always share.

Other checks when a share is granted:

- **The sharer can read the document.** The sharer can give `write` only while holding
  `write` on that document, so a read-only sharer can hand out read access and nothing
  more.
- **The recipient** must be an existing, enabled user other than the sharer,
  `Administrator` or `Guest`.
- **Some documents cannot be shared:** child rows, Singles, `Document Share`, `Audit Event`,
  `User Permission`, `Webhook` and `Webhook Delivery`.
- **Updating a share:** sharing again with the same user replaces that user's rights (one
  row per user and document).

To remove a share you need the `share` right on the document, unless you are the
recipient, who can always drop their own share.

## How a share interacts with the rest

| Mechanism | With a share |
| --- | --- |
| Role permissions | A share stands in for a missing role grant (including an `ifOwner` grant on someone else's document) for `read`/`write`/`share` only |
| Controller `hasPermission` / `permissionQuery` | Still apply. A controller that vetoes a write vetoes a shared write, and a list only includes a shared document that also passes `permissionQuery`. `hasPermission` is also asked for `ptype: "share"` |
| Workflow `allowEdit` | Still applies to a shared write |
| Docstatus | Unchanged: a shared `write` on a submitted document is limited to `allowOnSubmit` fields |
| Field permissions (`permlevel`) | A share grants the document at **level 0 only**. Fields above level 0 still require a role row for that level, so a share-only user never sees or changes them |
| User access scopes (`User Permission`) | A share is **still bound by the recipient's scopes**, unless it was given with **Override security scope** (below) |
| Child DocTypes | Rows follow their parent document's share. A child DocType cannot be listed through a share |

### Override security scope

A share given with `overrideScope` reaches the recipient even when the document is outside
their `User Permission` scopes. It lifts the scope for exactly the rights the share grants:

- **Read override:** the document opens, lists, counts and shows up in `db.exists`/`getValue`.
- **Write override:** the document can also be saved and `dbSet`.
- **Still refused:** delete, submit, cancel and amend stay refused by the scope, and
  `Document Share`, `User Permission`, `Webhook` and `Webhook Delivery` stay closed to a
  scoped user. An override lifts `read`, `write` and `share`, and nothing else.

Only a **System Manager without scope rows of their own**, `Administrator`, or an elevated
context may give an override. A refused attempt is recorded as a `Denied`
`permission.share_grant`. A scoped recipient holding the `share` right may re-share the
document, but never with an override.

## The `Document Share` DocType

Each share is a `Document Share` row in `tab_document_share`, unique per
`(user, share_doctype, share_name)`.

| Field | Type | Meaning |
| --- | --- | --- |
| `user` | Link (`User`) | The recipient |
| `share_doctype` | Data | The shared document's DocType |
| `share_name` | Data | The shared document's name |
| `read`, `write`, `share` | Check | The rights granted |
| `override_scope` | Check | Whether the share lifts the recipient's scopes |

A System Manager can read and export the rows. No role may write them through the generic
resource API: use the endpoints or `ddcore.share.*`, which check the sharer. A user with
access scopes is refused the DocType entirely.

> **Warning.** `ignorePermissions` lifts that refusal for app code, not for scoped users.
> Code running with permissions ignored — a job, a migration, an explicit
> `ignorePermissions` — can `insert`, `save` or `dbSet` a `Document Share` row directly and
> so grant any right, override included, without a single sharer check. The audit events
> and the cache invalidation below still fire, so such a grant is recorded, but nothing
> validates it. Share through `ddcore.share.*` unless you mean to bypass the checks.

A rename moves a document's shares, and deleting a document deletes them.

## HTTP API

| Method | Path | Body / result |
| --- | --- | --- |
| `GET` | `/api/shares/{doctype}/{name}` | `{ shares, canShare, canOverrideScope }`. The caller must read the document; without the `share` right, `shares` holds only the caller's own share |
| `POST` | `/api/shares/add` | `{ doctype, name, user, read, write, share, overrideScope }` → the `Document Share` row |
| `POST` | `/api/shares/remove` | `{ doctype, name, user }` → `{ ok: true }` |

`canOverrideScope` is computed only when `canShare` is true, and is `false` otherwise: a
caller who cannot share never sees it as `true`.

| Status | When |
| --- | --- |
| `401` | `Guest`, or no session |
| `403` | The caller lacks `share` on the document, lacks `write` for a write share, or may not give an override |
| `404` | The document does not exist, or `remove` names a share that does not exist |
| `417` | Sharing with yourself, with `Administrator` or `Guest`, or with a missing or disabled user; a DocType that cannot be shared; a missing `doctype`, `name` or `user`; invalid JSON, an unknown body field, or a body over 4096 bytes |

Order matters: the `share` right is checked **before** the recipient is validated, so a
caller who cannot share learns nothing about the user they named.

## Server SDK

```ts
// add(doctype, name, user, rights?) → DocShare. Rights are the last argument.
ddcore.share.add("Sales Order", "SO-0001", "ana@example.com", { write: true });
ddcore.share.list("Sales Order", "SO-0001"); // DocShares: { shares, canShare, canOverrideScope }
ddcore.share.remove("Sales Order", "SO-0001", "ana@example.com"); // void
```

These calls are synchronous and use the current user as the sharer, with the same checks as
the endpoints, decoding included: a key that is not a right — `override_scope` for
`overrideScope`, say — is refused with `417` rather than dropped, so a share never quietly
grants less than the caller asked for.

Inside a job (`ignorePermissions`), the checks on the sharer's rights are skipped, including
the override one; the recipient is still validated. A job runs as the user who enqueued it,
and a document cannot be shared with the sharer, so `ddcore.share.add` in a job can never
share with that user — share with them from the request instead, or enqueue under another
user.

## Desk

- **Form sidebar:** the form sidebar shows **Shared With** when the user can share the document or holds a
  share on it. Each row lists the rights and an override marker.
- **Share button:** it opens a dialog with a user picker, *Can Write*, *Can Share* and, for a user
  who may give one, *Override Security Scope*.
- **Desk SDK:** `ddcore.shares.forDoc / add / remove` expose the same endpoints to desk
  scripts. They are asynchronous, and `add` takes the recipient inside an object
  (`ShareArgs`), unlike the server SDK's positional `user`:

```ts
import { ddcore } from "@ddcore/desk-sdk";

// Who a document is shared with: DocSharesInfo { shares, canShare, canOverrideScope }
const info = await ddcore.shares.forDoc("Sales Order", "SO-0001");

// Grant or update a share → DocShare
await ddcore.shares.add("Sales Order", "SO-0001", { user: "ana@example.com", write: true });

// Revoke → { ok: true }
await ddcore.shares.remove("Sales Order", "SO-0001", "ana@example.com");
```

## Notifications

A new share sends the recipient an inbox notification ("Shared with you: …") in their
language. Changing an existing share's rights does not notify again. The notification is
rechecked like every other: once the share is revoked, it stops being listed and counted.

## Caching

A user's shares are cached in the request context and in the engine cache. Every change
through `Document Share` (insert, save, `dbSet`, delete, and the rename or delete of a shared
document) clears the recipient's cache, and their realtime-event authorization cache, after
commit. The writing transaction sees its own change immediately. Direct SQL writes to
`tab_document_share` bypass this.

## Audit

Recorded in `Audit Event` with the shared document as the target (see `audit`):

- `permission.share_grant`: a share was created. Detail is `user`, `read`, `write`, `share` and `override_scope`.
  `Denied` when the sharer lacked the `share` right, lacked `write` for a write share, or could not override.
- `permission.share_update`: the rights of an existing share changed. Same detail.
- `permission.share_revoke`: a share was removed. Detail is `user`. `Denied` when the caller could not remove it.

The cascade that deletes a deleted document's shares is not audited row by row.

## Limitations

- Shares are per user: no role or group shares, no "share with everyone", no expiry.
- `ddcore.db.sql` and link titles (`/api/search/link-titles`) ignore shares, as they ignore scopes.
- Background jobs run with permissions ignored, so shares play no part there.
- A user's shares on one DocType become an `IN` list in every list query on it. The list is
  meant to stay small; there is no pagination of shares.
- There is no permission editor or role profile (a separate backlog item).
