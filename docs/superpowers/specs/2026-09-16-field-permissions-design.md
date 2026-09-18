# Design: Field Permissions (SEC-02)

This document explains **why** field permissions are shaped the way they are. The API
reference is [`docs/agent/field-permissions.md`](../../agent/field-permissions.md).

## Starting point

- Permissions were whole-DocType: `PermDef` rows per role, `ifOwner`, controller hooks,
  workflow `allowEdit`, and SEC-01 scopes narrowing rows. Nothing was field-level.
- `hidden` and `readOnly` are screen hints; the server sent and accepted both.
- Password/Vault redaction (`RedactDoc`) was applied by hand at a few HTTP borders. It
  missed lists, `amend` and `run_method` responses, and child rows in Version diffs.
- The OPS-01 print spec already assumed a `permlevel > 0` omission.

## Key Decisions

### 1. Frappe-style `permlevel`

A field declares `permlevel: 0–9`; a `PermDef` row with `permlevel: N` grants `read`/`write`
on that level's fields. A row above 0 may grant nothing else, and it never grants the
document itself: `HasPermission`, `readIsOwnerOnly` and `permissionFilters` ignore such
rows.

**Rationale:** levels group fields, so a dozen confidential fields cost one row per role
rather than a role list per field. They compose with `extendDoctype`: an extension adds a
row, and `set` can move a field. They match what Frappe ports and the print spec already
expect.

### 2. Level 0 is the document; levels are role-only

`FieldAccess` is a bitmask computed from the user's roles and the DocType's rows. Level 0
is always granted. Whether the user reaches the document at all stays with the existing
document checks. Child DocTypes use the parent's rows, and a child listed directly uses the
intersection over every embedding parent. Admin and `IgnorePermissions` contexts
get full access.

**Rationale:** it keeps one authority per question. Document access remains where SEC-01,
hooks and workflows already enforce it, and the field check needs no document, so it is
cheap and cacheable.

### 3. Omit on the way out, at the framework's borders

`RedactDoc` removes unreadable keys, children included, which covers every document
response. Lists enforce inside `GetList`: `*` narrows to readable columns, a named column
is dropped, and a filter, sort, grouping or aggregate on one is a `PermissionError`.
Export, Version diffs (filtered at read time, since one diff serves every reader), print,
notification rendering (per recipient), webhooks (level 0 only) and file access
(`attached_to_field`) each apply the same access.

**Rationale:** refusing a filter instead of ignoring it closes the oracle and makes a
misbuilt screen fail loudly. Filtering Version on read avoids per-reader copies. A webhook
has no reader, so level 0 is the only safe default.

### 4. Server code is trusted; `ddcore.redact` is the opt-in border

`getDoc`, `getAll`, `getValue` and `sql` return raw values, as they already ignore
document permissions. `getList` and `count` apply levels, as they apply role permissions.
Whitelisted results, script report rows and `publish` payloads are the app's
responsibility, and `ddcore.redact(doctype, doc)` gives it the API's view.

**Rationale:** controllers compute with confidential values (totals, commissions). Guessing
at free-form report columns would be heuristic and silently incomplete.

### 5. Writes: restore what was unseen, then compare

Before any hook, a field the user cannot read that arrives absent or `null` takes the
stored value, or the default on insert. Then every field the user cannot write must equal
that base. A restricted `Table` must be unchanged row for row. An insert with
`amended_from` uses the cancelled source as its base.

**Rationale:** the Desk saves the whole document it received, and that document never had
the field, so without the restore every save would wipe it. Checking before hooks lets a
controller set restricted values, matching `checkReadOnlyDependsOn`.

### 6. Meta validation keeps identifying fields at level 0

`titleField`, `searchFields`, `naming.field` and format placeholders, the workflow state
field, and a level-0 `fetchFrom` destination of a restricted source are refused.

**Rationale:** these values surface in link titles, search, names and comments to anyone
who can find the document, so restricting them would be a promise the framework cannot keep.

### 7. Desk: shape the meta once

`/api/meta` adds `fieldLevels`. `getMeta` removes unreadable fields from the DocType and
its children and marks unwritable ones `readOnly`. Forms, grids, lists and filters follow
without their own checks.

## Left Out

- A permission editor and role profiles (SEC-03 backlog).
- Filtering app-returned payloads automatically; auditing field-level reads.
- Per-webhook field selection or a "run as" user.
- Retroactively privatising public files already stored in a newly restricted field.
