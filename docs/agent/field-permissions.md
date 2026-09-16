# Field permissions

Role permissions decide what a user may do with a *document*. Field permissions decide
which *fields* of that document they see and change: a salary on an employee record that
everyone in the team opens, a cost price on an item the sales desk sells, a review only HR
writes.

`hidden` and `readOnly` are screen hints. The server still sends a hidden field's value to
anyone who reads the document, and accepts a change to a read-only field from any API
client. A field that has to stay confidential needs a permission level.

## Declaring levels

A field declares a level from 0 to 9 with `permlevel`. The default is 0. A permission row
with `permlevel: N` grants `read` and/or `write` on that level's fields to its role:

```ts
defineDoctype({
  name: "Employee",
  fields: [
    { fieldname: "full_name", fieldtype: "Data", label: "Full Name", reqd: true },
    { fieldname: "department", fieldtype: "Link", label: "Department", options: "Department" },
    { fieldname: "salary", fieldtype: "Currency", label: "Salary", permlevel: 1 },
    { fieldname: "review", fieldtype: "Small Text", label: "Review", permlevel: 2 },
    { fieldname: "bonuses", fieldtype: "Table", label: "Bonuses", options: "Employee Bonus", permlevel: 1 },
  ],
  permissions: [
    { role: "Employee Manager", read: true, write: true, create: true },
    { role: "HR Manager", read: true, write: true, create: true },
    { role: "HR Manager", permlevel: 1, read: true, write: true },
    { role: "HR Manager", permlevel: 2, read: true },
  ],
});
```

An Employee Manager reads and writes `full_name` and `department`, and never sees
`salary`, `review` or `bonuses`. An HR Manager also reads and writes `salary` and
`bonuses`, and can read `review` without changing it.

The rules:

- **Level 0 is the document.** Whoever may read the document reads its level-0 fields, and
  whoever may write it writes them. Nothing about level 0 changes.
- **A level is never a way in.** A row above level 0 grants fields, not the document: the
  role still needs a level-0 row (or another of the user's roles does). A row above 0 may
  set only `read` and `write`. `create`, `delete`, `submit`, `cancel`, `amend`, `report`,
  `export` and `ifOwner` are refused when the meta loads.
- **`write` implies `read`** at the same level.
- **Standard columns** (`name`, `owner`, `creation`, `modified`, `modified_by`,
  `docstatus`, and a child row's `parent`, `parenttype`, `parentfield` and `idx`) are
  always level 0.
- **Child tables follow the parent.** A child DocType has no permissions of its own, so its
  fields' levels are judged by the parent's rows. A `Table` field with a level restricts the
  whole table: its rows, and adding or removing them. Listing a child DocType directly uses
  the strictest level across every DocType that embeds it.
- **Administrator** and privileged contexts (jobs, migrations and patches,
  `ignorePermissions`, `getAll`, a workflow transition's own field updates) see and write
  every level.

`extendDoctype` may set `permlevel` on another app's field through `set`, and may add a
permission row at a level for a role the owner already grants at level 0. See `extending`.

### What the meta refuses

A field that identifies a document outside its own form cannot be restricted. Everyone who
can find the document sees it, so these must be level 0:

- `titleField`, `searchFields`, `naming.field` and every `{field}` in `naming.format`;
- a workflow's state field;
- a level-0 field whose `fetchFrom` copies a field above level 0. Copying a restricted value
  into an unrestricted field would publish it. Give the destination a level instead.

## Reading

A field the user cannot read is **omitted**. The key is absent, not null, everywhere the
framework hands a document out:

| Path | Behaviour |
| --- | --- |
| `GET /api/resource/{doctype}/{name}`, create, update, submit, cancel, save, amend, workflow actions | Omitted, child rows included |
| A controller method (`run_method`) | The method sees the whole document; the `doc` in the response is redacted |
| Lists, `count`, link search, number cards (`/api/resource/{doctype}`, `ddcore.db.getList`, `ddcore.db.count`) | `*` and a named field leave the column out. A filter, `or_filters`, `order_by`, `group_by` or aggregate (`sum(salary)`) on the field is a `PermissionError`, so a query cannot probe the value |
| Export (HTTP and CLI) | Default columns leave it out; naming it is a `PermissionError`; child columns and attachments held by the field are left out |
| Version history | A change to the field is removed from the diff, on `/api/versions` and on `Version` read through the resource API |
| Print and PDF | The field, its label and its child-table column are left out of standard and custom templates |
| Notifications | A rule's condition and recipients see the whole document; the desk title, message and email arguments are rendered from what **each recipient** may read |
| Webhooks | The payload carries **only level-0 fields**, whoever made the change. A webhook has no reading user |
| `/private/files` and mail attachments | A file whose `attached_to_field` is restricted needs read on that field |
| `GET /api/meta/{doctype}` | Adds `fieldLevels: { read: number[], write: number[] }` for the current user. Field definitions are not secret and stay in the meta |

Password and Vault redaction still applies on top of this for every user, Administrator
included.

## Writing

The check runs on the document as it arrives, **before any hook**. A controller may still
set a restricted field on the user's behalf, for example a computed commission.

1. **What the writer could not see is kept.** A field the user cannot read that arrives
   absent or `null` keeps its stored value, or its default on an insert. The Desk saves the
   whole document it was given, and that document never had the field. Child rows are
   matched by `name`. A new row gets the default for the columns the user cannot read.
2. **Anything else is compared.** A field the user cannot write must equal the stored value,
   or the default on an insert. If it does not, the save fails with `PermissionError` "Not
   permitted to change {0}". In a restricted `Table`, adding, removing or editing a row is a
   change.
3. **An amendment carries its source.** Inserting a document with `amended_from` compares it
   with the cancelled original, so a user without the level can amend without being able to
   see or reset the restricted values.

Uploading into a restricted `Attach` field requires write on that level, and the file is
always stored as private.

A user who cannot write a field cannot clear it either. A **mandatory** field above level 0
is still mandatory for someone who cannot write it, so give it a `default` or set it in a
hook.

## Server code is trusted

A controller, a whitelisted method, a report and a job work on whole documents:
`ddcore.getDoc` returns every field, and `ddcore.db.getAll`, `ddcore.db.getValue` and
`ddcore.db.sql` ignore field levels as they ignore document permissions.
`ddcore.db.getList` and `ddcore.db.count` apply them, the same way they apply role
permissions.

What server code *returns* is the app's responsibility. Before a whitelisted method, a
script report or `ddcore.publish` hands a document to a client, pass it through
`ddcore.redact`:

```ts
export const employeeCard = whitelisted((args: { name: string }) => {
  const doc = ddcore.getDoc("Employee", args.name);
  return ddcore.redact("Employee", doc); // what GET /api/resource would return to this user
});
```

`ddcore.redact(doctype, doc)` returns a copy. The caller's document is untouched. It applies
Password/Vault redaction and the current user's field levels, child rows included.
`doc.dbSet` and `ddcore.db.setValue` write columns directly and check nothing.

## Desk

The Desk reads `fieldLevels` from the meta. A field the user cannot read is removed from
the form, the grid, list columns and filters. A field they cannot write is read-only. These
are conveniences: the server enforces both whatever the screen shows.

## Limitations

- There is no permission editor. Levels are versioned metadata, declared in code.
- A file already uploaded as **public** into a field is reachable by its URL whatever the
  field's level. Uploads into a restricted field are private from the moment the level
  exists, not before.
- Comments are free text. A workflow comment names the state field, which is level 0.
- `ddcore.db.sql`, `ddcore.publish` payloads, `/api/method` results and script report rows
  are not filtered: use `ddcore.redact` or select only what the user may read.
- A list view saved with a now-restricted column drops that column.
