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
| Password | text | not hashed automatically |
| Section Break / Column Break / Tab Break | — | layout; `label`, `collapsible`, `dependsOn` on a Section |
| HTML | — | `options` is the rendered HTML |

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
length, precision, description, columns (grid width 1–12), gridEditMode (`"inline"` default or `"dialog"`), collapsible, bold`

- `label` and `description` are **catalogue keys**: write them in English. See `i18n`.
- `default`: a literal value, or `"Today"` for Date/Datetime, `"__user"` for the current user.
- `fetchFrom: "project.assignee"`: copied from the linked document on save. When `readOnly` it always overwrites; otherwise it fills only when empty.
- `dependsOn`, `readOnlyDependsOn`, `mandatoryDependsOn`: a JS expression over `doc` (`"doc.type == 'PJ'"`) or a field name (truthy). Evaluated in the desk **and** on the server.
- `optionColors` (Select): `{ Open: "blue", Overdue: "red" }`, keyed by the canonical value — never by its label.

## DocType properties

```ts
defineDoctype({
  name: "Contract", module: "Sales", label: "Contract",
  naming: { series: "CTR-.YYYY.-.####" } | { field: "code" } | { format: "{index}-{period}" } | { hash: true } | { prompt: true },
  submittable: true, isChild: false, trackChanges: true, allowRename: true,
  titleField: "name", searchFields: ["name", "tax_id"], sortField: "modified", sortOrder: "desc", icon: "building-2",
  fields: [...],
  permissions: [{ role: "Manager", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true, report: true, export: true, ifOwner: false }],
});
```

Series: `.YYYY.`, `.YY.`, `.MM.`, `.DD.`, `.####.` (a zero-padded counter), `.{field}.`. A `naming_series` field (Select) lets the user pick the series.
