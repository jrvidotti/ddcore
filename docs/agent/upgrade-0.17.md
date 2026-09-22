# Upgrading an app to 0.17: the `name` → `id` key

In 0.17 the document key is `id` instead of `name` — the column, the SDK, the REST API, the MCP
tools, the desk — and the vocabulary around it follows (`naming` → `idGeneration`, `nameLabel`
→ `idLabel`, …). There is no alias. This page is the checklist for moving an app across; what
`migrate` does to the database is in `migrations` ("The 0.17 key rename").

The database side is automatic: the first `ddcore migrate` on 0.17 renames the column on every
table, before any patch runs. The work is in the app's **source**, and the compiler will not
find most of it — read "What fails loudly, and what does not" before trusting a green build.

## The steps

1. **Back up** the site (`ddcore backup`). The rename is one transaction and a failure leaves
   the database as it was, but a 0.16 binary cannot open a database 0.17 has migrated: rolling
   back the binary means restoring the backup.
2. **Upgrade the binary**, then run `ddcore types`: it refreshes `.ddcore/sdk` and
   `.ddcore/types.d.ts`, where `BaseDoc` now has `id`. Never edit those files by hand.
3. **Rename in the source**, with the table below and the searches under "Finding every
   occurrence". Patches included — the ones already run as well as the pending ones (see
   "Patches").
4. **Type-check** the app (`tsc --noEmit` against its `tsconfig.json`). It catches the renamed
   DocType and list-view options, and nothing about `doc.name`.
5. **Preview**: `ddcore migrate --dry-run`. It shows the plan as it will be after the key rename;
   an `ADD COLUMN "id"` or a refusal on a `name` column means something below was missed.
6. **Migrate and test**: `ddcore migrate`, then `ddcore test --app <app>`. Tests are where the
   silent failures surface, so a DocType with no test deserves one now.
7. **Raise the range** in `defineApp`: `ddcore: ">=0.17.0 <…"`. A 0.17 binary warns at start-up
   about every app whose range still reaches below 0.17.0 — that warning is the reminder, and it
   stops once the range says the app has been through this page.

## What to rename

### DocType definitions

| 0.16 | 0.17 |
|---|---|
| `naming: { series: "INV-.####" }` | `idGeneration: { series: "INV-.####" }` |
| a `naming_series` Select field | an `id_series` field |
| `nameLabel: "Contract No."` | `idLabel: "Contract No."` |
| `titleField: "name"`, `searchFields: ["name", …]` | `titleField: "id"`, `searchFields: ["id", …]` |
| Vault key template `"asaas:token:{name}"` | `"asaas:token:{id}"` |
| `renamedFrom: "name"` on a field | not allowed: the key is `id`, and migrate moved it |

`idGeneration` keeps the same sub-keys (`series`, `field`, `hash`, `prompt`, `format`).
Vault keys already stored are values and do not move: a template that produced
`asaas:token:ACME` under `{name}` produces the same key under `{id}`.

### Server code (controllers, services, reports, patches, tests)

| 0.16 | 0.17 |
|---|---|
| `doc.name`, `row.name`, `this.name` | `doc.id`, `row.id`, `this.id` |
| `ddcore.getDoc("Task", name)` | unchanged call, the argument is the id |
| `filters: { name: x }`, `["name", "in", ids]` | `filters: { id: x }`, `["id", "in", ids]` |
| `fields: ["name", …]`, `"count(name) as n"` | `fields: ["id", …]`, `"count(id) as n"` |
| `orderBy: "name asc"` | `orderBy: "id asc"` |
| `ddcore.hasPermission(dt, "read", { name, owner })` | `{ id, owner }` |
| `sendMail({ reference: { doctype, name } })` | `reference: { doctype, id }` |
| `webhooks.emit(…, { reference: { doctype, name } })` | `reference: { doctype, id }` |
| `ddcore.rename(dt, oldName, newName)` | same call; the arguments are ids |
| `newDoc("Task", { name: "T-1", … })` (a prompted key) | `{ id: "T-1", … }` |
| SQL: `WHERE name = $1`, `t.name`, `ORDER BY name` | `WHERE id = $1`, `t.id`, `ORDER BY id` |

The core's reference fields changed too; filters, reports and SQL that name them must follow:

| DocType | 0.16 | 0.17 |
|---|---|---|
| Comment, ToDo, Email Delivery, Webhook Delivery | `reference_name` | `reference_id` |
| Document Share | `share_name` | `share_id` |
| File | `attached_to_name` | `attached_to_id` |
| Audit Event | `target_name` | `target_id` |
| Version | `docname` | `doc_id` |

### Desk code (form scripts, list views, desk includes)

| 0.16 | 0.17 |
|---|---|
| `frm.doc.name`, child `row.name` | `frm.doc.id`, `row.id` |
| `defineListView(dt, { nameColumn: false })` | `{ idColumn: false }` |
| `ddcore.db.getValue(dt, { name: x }, …)` | `{ id: x }`, or pass the id string |
| a link built as `` `/app/${ws}/${dt}/${doc.name}` `` | `` `…/${doc.id}` `` |
| `ToDoDoc.name`, `DocShare.name`/`share_name`, `DeskNotification.name`/`reference_name`, `GlobalSearchHit.name` | `id`, `share_id`, `reference_id`, `id` |

### Anything outside the app that talks to the site

| 0.16 | 0.17 |
|---|---|
| `POST /api/assignments/assign` `{ doctype, name, … }`; `complete`/`revoke` `{ name }` | `{ doctype, id, … }`; `{ id }` |
| `POST /api/shares/add` and `/remove` `{ doctype, name, user, … }` | `{ doctype, id, user, … }` |
| `POST /api/workflow/apply` `{ doctype, name, action }` | `{ doctype, id, action }` |
| `GET /api/workflow/actions?doctype=X&name=Y` | `?doctype=X&id=Y` |
| `POST /api/resource/{dt}/{id}/rename` `{ "name": new }` | `{ "id": new }` |
| `GET /api/search/link-titles?doctype=X&names=a,b` | `?ids=a,b` |
| upload form field `docname` | `doc_id` |
| a response's `name` (documents, search hits, letterheads, API keys, health errors) | `id` |
| webhook receiver reading `data.name` | `data.id` |
| realtime `doc_update` payload `name` | `id` |
| MCP `get_doc`/`update_doc`/`delete_doc`/`submit_doc`/`cancel_doc`/`call_method` `name` | `id` |
| a consumer of `ddcore export` reading the `name` column | the `id` column |

The URL paths keep their shape — `/api/resource/{doctype}/{id}` is the same URL it was — so a
client that only builds URLs from ids needs nothing. What changes is every JSON body and every
response field above. A webhook receiver is somebody else's code: tell its owner before the
upgrade, or accept both `data.id` and `data.name` there for a while.

## What fails loudly, and what does not

`BaseDoc` carries an index signature, so `doc.name` type-checks on 0.17 exactly as `doc.id`
does. The compiler is no help for the most common case. What each old spelling does on 0.17:

| Old spelling | On 0.17 |
|---|---|
| `fields: ["name"]`, `orderBy: "name asc"`, `filters: { name: … }` | **error**: unknown field — unless the DocType declares a field called `name`, in which case it silently reads that field instead |
| SQL naming the `name` column | **error**: column does not exist |
| `renamedFrom: "name"`, a Vault template with `{name}` | **error at load**, with the reason |
| `nameColumn` in `defineListView`, `naming`/`nameLabel` in `defineDoctype` | **type error** under `tsc`; at run time **silently ignored** — a DocType whose `naming` is dropped gets random hash ids |
| `doc.name`, `row.name`, `frm.doc.name`, a response's `.name` | **silent**: `undefined` — a cache key ending in `undefined`, a link to `/…/undefined`, a comparison that never matches |
| assignment and share bodies with `name` | **error**: unknown field in the JSON |
| a rename body `{ "name": … }` | **silent no-op**: the document keeps its id |
| `?names=` on link titles, `data.name` in a webhook receiver | **silent**: empty result, `undefined` |

So: run `tsc`, run the tests, and search the source — all three.

## Finding every occurrence

From the app's directory. Each hit is either the key (rename it) or a genuine name — of a
DocType, report, workspace, template, workflow, app or file — which stays. The first search is
noisy on purpose.

```bash
# the key read or written as a property, a filter, a field or in SQL
grep -rnE '\.name\b|\bname:|"name"|\bname\s*(=|<>|!=|IN|ASC|DESC)|count\(name\)' --include='*.ts' .

# DocType vocabulary
grep -rnE 'naming|nameLabel|nameColumn|naming_series|\{name\}' --include='*.ts' .

# the core's reference fields
grep -rnE 'reference_name|share_name|attached_to_name|target_name|docname' --include='*.ts' .
```

Leave `.ddcore/` out of the review: `ddcore types` rewrites it. Search the app's translations
too if a message quoted a field by its column name.

## Patches

The rename runs before every patch, so a patch sees `id` whether it is `beforeSchema` or
`afterSchema`. A patch that has **not run yet** on some site must say `id`, or it fails there
with a column that does not exist. A patch that **has already run** everywhere is never run
again, but a new site installs from the source, and anyone reading it later reads the source:
change it too, rather than leave a file that describes a schema that no longer exists.

Do not write a patch to rename `name` to `id` yourself — `migrate` does it, and a DocType
table the app renames by hand is simply found already moved.

## A field called `name`

`name` is an ordinary fieldname from 0.17 on, and a DocType may declare one — a person's name, a
product's name. It is not the key and never becomes it: the key is `id`, and `idGeneration`
decides it (`idGeneration: { field: "name" }` makes the field's value the id, as any field can).
Declaring it has one side effect worth knowing: on that DocType, an old `filters: { name: … }`
stops failing and silently filters on the field.

## Example

Before, on 0.16:

```ts
export default defineDoctype({
  name: "Invoice",
  naming: { series: "INV-.####" },
  nameLabel: "Invoice No.",
  titleField: "customer",
  fields: [/* … */],
});

export default defineController<Invoice>({
  validate(doc) {
    const dup = ddcore.db.exists("Invoice", { external_ref: doc.external_ref, name: ["!=", doc.name] });
    if (dup) ddcore.throw(_("Already imported as {0}", [dup]));
  },
  onSubmit(doc) {
    ddcore.sendMail({ template: "invoice", to: doc.email, reference: { doctype: "Invoice", name: doc.name } });
  },
});
```

After, on 0.17:

```ts
export default defineDoctype({
  name: "Invoice",
  idGeneration: { series: "INV-.####" },
  idLabel: "Invoice No.",
  titleField: "customer",
  fields: [/* … */],
});

export default defineController<Invoice>({
  validate(doc) {
    const dup = ddcore.db.exists("Invoice", { external_ref: doc.external_ref, id: ["!=", doc.id] });
    if (dup) ddcore.throw(_("Already imported as {0}", [dup]));
  },
  onSubmit(doc) {
    ddcore.sendMail({ template: "invoice", to: doc.email, reference: { doctype: "Invoice", id: doc.id } });
  },
});
```

`name: "Invoice"` stays: it is the DocType's name, not a document's key.
