# Fieldtypes and properties

| Fieldtype | Postgres column | Notes |
|---|---|---|
| Data | text | `length` limits the input |
| Email | text | an email address; outer spaces are trimmed and the format is validated in the desk and on the server |
| Small Text / Text / Text Editor | text | textarea (2 / 5 rows) |
| Int | bigint | |
| Float | double precision | a measurement; `precision` affects display only and the value is never rounded on save |
| Currency | numeric(21,9) | **rounded on save** to the site's precision; shown with the symbol of `ddcore.json:currency`, grouped as the reader's language does |
| Percent | numeric(21,9) | a rate: `precision` affects display only, shown with % |
| Check | boolean | defaults to `false` |
| Date | date | value "YYYY-MM-DD"; a **civil date**, never converted between timezones |
| Month | date | value "YYYY-MM-01", shown and edited in the locale's month order |
| Datetime | timestamptz | ISO value; an **instant**, shown in the site's timezone |
| Time | time | "HH:MM:SS"; a civil time, never converted |
| Select | text | `options: ["A", "B"]`, canonical English, validated on the server; `optionColors` gives each value an indicator colour |
| Link | text | `options: "DocType"`; existence validated; index created automatically |
| Dynamic Link | text | `options: "<the field holding the DocType>"` |
| Table | (child table) | `options: "Child DocType"` with `isChild: true`; `gridEditMode: "dialog"` turns off inline editing |
| Attach | text | the file's URL (`/files/..` or `/private/files/..`) |
| JSON | jsonb | |
| Password | text | not hashed automatically; never read back through the API, never in Version, never exported. **Not for an integration credential** — see below |
| Section Break / Column Break / Tab Break | — | layout; `label`, `collapsible`, `dependsOn` on a Section |
| HTML | — | `options` is the rendered HTML |

## Secrets: `ddcore.secret`, not a column

An integration credential — an API key, a webhook token, a relay password —
does not go in a document. Read it from the environment:

```ts
const key = ddcore.secret("stripe_key");   // DDCORE_SECRET_STRIPE_KEY
if (!key) ddcore.throw(_("Stripe is not configured on this site"));
```

`.env` supplies it in development and the platform supplies it in production.
`ddcore doctor` lists the names it found and never the values.

The reason is not fastidiousness. A secret in a column is a secret in every
backup, every replica, every export and every Version diff, and keeping it out
of those is a list of places to remember rather than a property of the system.
A secret in the environment is in none of them because it was never written
down, and rotating it is a redeploy rather than a migration. The
`DDCORE_SECRET_` prefix is the boundary: an app reads its own secrets and
nothing else the process was started with, so `ddcore.secret("DDCORE_DSN")`
returns null.

`Password` is for a secret a *person* types and this site stores — and the
honest statement is that it is still plain text at rest. What the framework
guarantees is that it does not leave: the value is blanked on every read
through the API, never enters a `Version` diff, and never appears in an export.
A controller still sees the real value, because it works on the `Ctx` and never
through that border. If what you hold is a machine credential rather than a
person's password, it belongs in `ddcore.secret`, where none of that has to be
guaranteed one path at a time.

## Money: precision and rounding

A `Currency` is the one numeric type the server rounds **on the way into the
database**, so that what a form shows and what a `sum()` adds are the same
number. How many places it keeps resolves in three steps, most specific first:

1. the field's own `precision`;
2. `ddcore.json:currencyPrecision`;
3. the ISO minor unit of `ddcore.json:currency` — 2 for USD and BRL, 0 for JPY,
   3 for KWD. This is the default, and it is right more often than a number
   written by hand.

`ddcore.json:rounding` is `"commercial"` (half away from zero — the default:
`2.345 → 2.35`, `-2.345 → -2.35`) or `"bankers"` (half to even). The rule is
applied to the number's shortest decimal representation, so `1.005` rounds to
`1.01` even though the double really stored is `1.00499999999999989`. The same
rule runs in `ddcore.utils.roundTo` and in the desk, so a total computed in a
controller or a form script equals the total the server writes.

`Percent` and `Float` are never rounded on save — a rate of `33.333333` and a
measurement of `1.23456789` are both legitimate — and `Int` keeps its own rule
(half away from zero), deliberately unaffected by `rounding`.

What the column will refuse, with the field's name: `NaN`, `±Inf`, and any
value from 10¹² up, which is the limit of `numeric(21,9)`. That column also
still truncates silently past nine decimal places, which only a `Float`-like
use of `Percent` would reach.

## Field properties

`fieldname, fieldtype, label, options, optionColors, reqd, unique, default, readOnly, hidden, fetchFrom, dependsOn,
readOnlyDependsOn, mandatoryDependsOn, allowOnSubmit, inListView, inStandardFilter, searchIndex,
length, precision, description, columns (grid width 1–12), width (`"sm"` | `"md"` | `"lg"` | `"full"`), gridEditMode (`"inline"` default or `"dialog"`), collapsible, bold,
renamedFrom, convert`

- `label` and `description` are **catalogue keys**: write them in English. See `i18n`.
- `width`: `"sm"` | `"md"` | `"lg"` | `"full"`. How much of a form line the control takes: `sm`/`md` a quarter, `lg` a half, `full` the whole line (in a section split by a `Column Break`, the column is already half a line, so `lg` and `full` both fill it). Defaults to `sm` for `Date`, `Month`, `Time`, `Int`, `Percent`; `md` for `Datetime`, `Float`, `Currency`; `full` for `Text`, `Small Text`, `Text Editor`, `JSON`, `Table`, `HTML`; `lg` for every other type. Fields pack a line greedily, aligned so a half-line field never starts in the middle of a quarter. See `form-api`.
- `default`: a literal value, or `"Today"` for Date/Datetime, `"__user"` for the current user.
- `fetchFrom: "project.assignee"`: copied from the linked document on save. When `readOnly` it always overwrites; otherwise it fills only when empty.
- `dependsOn`, `readOnlyDependsOn`, `mandatoryDependsOn`: a JS expression over `doc` (`"doc.type == 'PJ'"`) or a field name (truthy). Evaluated in the desk **and** on the server.
- `optionColors` (Select): `{ Open: "blue", Overdue: "red" }`, keyed by the canonical value — never by its label.
- `renamedFrom: "old_name"` (or a list, oldest first): the fieldname this field used to have, so `migrate` renames the column instead of adding an empty one beside it. See `migrations`.
- `convert: { from: "Data" }`: authorises a column-type change `migrate` would otherwise refuse, naming the fieldtype the database still holds. No SQL — a conversion a plain cast cannot express goes through expand → backfill → validate → contract.

## DocType properties

```ts
defineDoctype({
  name: "Contract", module: "Sales", label: "Contract",
  naming: { series: "CTR-.YYYY.-.####" } | { field: "code" } | { format: "{index}-{period}" } | { hash: true } | { prompt: true },
  submittable: true, isChild: false, trackChanges: true, allowRename: true, renamedFrom: "Old Name",
  titleField: "name", searchFields: ["name", "tax_id"], sortField: "modified", sortOrder: "desc", icon: "building-2",
  uniqueKeys: [{ name: "customer_number", fields: ["customer", "number"] }],
  fields: [...],
  permissions: [{ role: "Manager", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true, report: true, export: true, ifOwner: false }],
});
```

Series: `.YYYY.`, `.YY.`, `.MM.`, `.DD.`, `.####.` (a zero-padded counter), `.{field}.`. A `naming_series` field (Select) lets the user pick the series.

`allowRename` is about renaming a *document*; `renamedFrom` is about renaming the *DocType*,
which moves the table and repoints every stored reference. See `migrations`.

`titleField`, `sortField`, `searchFields` and `uniqueKeys` name a field by string, so a
renamed field has to be changed here too — the meta refuses to load while one of them points
at a field that no longer exists.

`uniqueKeys` declares a business key the **database** enforces: each key becomes a partial
unique index, which is what makes it hold when two requests race. A row is constrained only
when every component has a value — null, or empty on a text column, leaves the row outside
the key, exactly as a `unique` field with no value is left out. Two or more declared fields
(a single field is `unique: true`), never a standard column, and the same fields in another
order are the same key. The `name` is the key's identity: the index is named after it, so
reordering the list or renaming a component neither renames nor rebuilds it. Not available on
a child DocType, and not something `extendDoctype` can add — a business key is part of what
the owning app says the document *is*. See `migrations` for what changing one costs.
