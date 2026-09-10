# Fieldtypes and properties

| Fieldtype | Postgres column | Notes |
|---|---|---|
| Data | text | `length` limits the input |
| Email | text | an email address; outer spaces are trimmed and the format is validated in the desk and on the server |
| Small Text / Text / Text Editor | text | textarea (2 / 5 rows) |
| Int | bigint | |
| Float | double precision | `precision` affects display only |
| Currency | numeric(21,9) | shown with the symbol of `ddcore.json:currency`, grouped as the reader's language does |
| Percent | numeric(21,9) | shown with % |
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
