# Fieldtypes and properties

| Fieldtype | Postgres column | Notes |
|---|---|---|
| Data | text | `length` limits the input |
| Email | text | an email address; outer spaces are trimmed and the format is validated in the desk and on the server |
| Small Text / Text | text | textarea (2 / 5 rows) |
| Text Editor | text | **rich text**: sanitized HTML, edited with a toolbar. See below |
| Markdown Editor | text | the Markdown source as written, rendered (and sanitized) only for display and print |
| Code | text | monospace, never trimmed; `options` is the language (`"sql"`, lowercase) |
| Int | bigint | |
| Float | double precision | a measurement; `precision` affects display only and the value is never rounded on save |
| Currency | numeric(21,9) | **rounded on save** to the site's precision; shown with the symbol of `ddcore.json:currency`, grouped as the reader's language does |
| Percent | numeric(21,9) | a rate: `precision` affects display only, shown with % |
| Check | boolean | defaults to `false` |
| Rating | bigint | 0 to `options` stars (1–10, default 5); `null` is "not rated", which is not zero |
| Duration | bigint | whole seconds, shown as `1d 2h 30m`; `options: ["hideDays", "hideSeconds"]` hides a unit on screen and never changes the value |
| Color | text | `#rrggbb`, lowercase; `#abc` is expanded and anything else is refused |
| Date | date | value "YYYY-MM-DD"; a **civil date**, never converted between timezones |
| Month | date | value "YYYY-MM-01", shown and edited in the locale's month order |
| Datetime | timestamptz | ISO value; an **instant**, shown in the site's timezone |
| Time | time | "HH:MM:SS"; a civil time, never converted |
| Select | text | `options: ["A", "B"]`, canonical English, validated on the server; `optionColors` gives each value an indicator colour |
| Link | text | `options: "DocType"`; existence validated; index created automatically |
| Dynamic Link | text | `options: "<the field holding the DocType>"` |
| Table | (child table) | `options: "Child DocType"` with `isChild: true`; `gridEditMode: "dialog"` turns off inline editing |
| Attach | text | the file's URL (`/files/..` or `/private/files/..`) |
| Attach Image | text | an Attach restricted to png, jpg, gif or webp, refused at upload as well as on save; SVG is not one of them, because it carries script |
| JSON | jsonb | |
| Password | text | not hashed automatically; never read back through the API, never in Version, never exported. **Not for an integration credential** — see below |
| Vault | — | virtual field backed by the encrypted vault (`ddcore_vault`); never a column in `tab_<doctype>`, never in Version or export; masked in Desk and API. See [vault.md](vault.md) |
| Section Break / Tab Break | — | layout; `label`, `collapsible`, `dependsOn` on a Section |
| HTML | — | `options` is the rendered HTML |

## Rich text

A `Text Editor` value is HTML. The server cleans it on the way in — in the one
place every write passes through — and stores the cleaned markup, so what the
database holds is what the API, print, export and the desk all read.

**What survives:** paragraphs, `h1`–`h4`, bold, italic, underline, strike,
inline code and code blocks, bullet and numbered lists, quotes, horizontal
rules, links and images. **What is removed:** `style`, event handlers,
`script`, `iframe`, `svg`, `form`, and `javascript:`/`data:` URLs. Markup
outside the list is *dropped, not refused*: a paste from a word processor is
not the author's mistake, and the response carries the value that was kept.

- **An image is a file this site serves** (`/files/…` or `/private/files/…`).
  An external URL — `https` included — is removed: a PDF is rendered by headless
  Chrome or Gotenberg against the site's own address, so an outside URL would
  make *that server* fetch an address the document's author chose, and would
  make every reader a hit on somebody else's server. A public file prints; a
  `/private/files/…` one does not, because the renderer holds no session.
- **Links out of the site** get `target="_blank"` and
  `rel="noopener noreferrer nofollow"`, which is what the desk's editor writes,
  so a document survives being opened and saved unchanged.
- **Tables are not allowed in rich text** yet: the editor cannot represent one,
  and keeping what the editor would drop is how data disappears on the next
  save. A `Markdown Editor` does have tables.
- **An emptied editor stores `null`.** Every editor leaves `<p></p>` behind, and
  `reqd` has to see that as empty.

**Values written before this release are plain text**, and they are read as
such: `a < b` keeps its `<` and is shown as a paragraph. The conversion is
written back on the document's next save and is not recorded as a change.
Nothing else converts, and the column does not change, so there is no
migration.

`ddcore.db.sql` writes bypass all of this, as they bypass every other field
rule. The desk sanitizes again before rendering, so a row written that way is
still safe to display, and a custom print template's `b.richText` block is
re-cleaned when it renders.

## Secrets: `ddcore.secret`, `ddcore.vault`, and `Vault` fieldtype

There are three ways secrets are handled:

1. **Site-level secrets (`ddcore.secret`)**:
An integration credential fixed per deployment — an API key, a webhook token, a relay password — does not go in a document. Read it from the environment:

```ts
const key = ddcore.secret("stripe_key");   // DDCORE_SECRET_STRIPE_KEY
if (!key) ddcore.throw(_("Stripe is not configured on this site"));
```

`.env` supplies it in development and the platform supplies it in production. `ddcore doctor` lists the names it found and never the values.

2. **Per-record dynamic credentials (`ddcore.vault` and `Vault` fieldtype)**:
When an app manages credentials per document (e.g. per-customer tokens, OAuth refresh tokens, integration accounts), use the `Vault` fieldtype or `ddcore.vault.*`:

```ts
// In DocType:
{ fieldname: "api_key", fieldtype: "Vault", label: "API Key" }

// In server-side controller:
const token = ddcore.vault.get("Integration Account:api_key:" + doc.id);
```

Secrets in the vault are encrypted with AES-256-GCM using `DDCORE_SECRET_KEY`, stored in `ddcore_vault` outside the document table, never leak through REST API or MCP (redacted to `{ configured: true }`), never enter Version diffs, and a read or write through `ddcore.vault.*` is audited in `Audit Event`. See [vault.md](vault.md).

3. **Person passwords (`Password`)**:
`Password` is for a secret a *person* types and this site stores (plain text at rest, blanked on read through the API, excluded from Version and export). For machine credentials and integration tokens, always use `Vault` or `ddcore.secret`.

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
permlevel, renamedFrom, convert`

- `permlevel`: 0–9, default 0. A field above 0 is read and written only by roles granted that level by a permission row with the same `permlevel`; the server omits it from every response and refuses a change from anyone else. `hidden` and `readOnly` are screen hints and protect nothing. See `field-permissions`.

- `label` and `description` are **catalogue keys**: write them in English. See `i18n`.
- `width`: `"sm"` | `"md"` | `"lg"` | `"full"`. How much of a form line the control takes: `sm`/`md` a quarter, `lg` a half, `full` the whole line — a form has no columns, its shape comes from the widths. Defaults to `sm` for `Date`, `Month`, `Time`, `Int`, `Percent`, `Rating`, `Color`; `md` for `Datetime`, `Float`, `Currency`, `Duration`; `full` for `Text`, `Small Text`, `Text Editor`, `Markdown Editor`, `Code`, `JSON`, `Table`, `HTML`; `lg` for every other type. Fields pack a line greedily, aligned so a half-line field never starts in the middle of a quarter. See `form-api`.
- `default`: a literal value, or `"Today"` for Date/Datetime, `"__user"` for the current user.
- `fetchFrom: "project.assignee"`: copied from the linked document on save. When `readOnly` it always overwrites; otherwise it fills only when empty.
- `dependsOn`, `readOnlyDependsOn`, `mandatoryDependsOn`: a JS expression over `doc` (`"doc.type == 'PJ'"`) or a field name (truthy). Evaluated in the desk **and** on the server.
- `optionColors` (Select): `{ Open: "blue", Overdue: "red" }`, keyed by the canonical value — never by its label.
- `renamedFrom: "old_name"` (or a list, oldest first): the fieldname this field used to have, so `migrate` renames the column instead of adding an empty one beside it. See `migrations`.
- `options` beyond Link, Table and Select: `Rating` takes the number of stars,
  `Code` the language, and `Duration` its display flags. None of them are
  catalogue keys — they are never translated, unlike a Select's options.
- Changing a field from `Text`, `Small Text` or `Data` to `Text Editor`,
  `Markdown Editor` or `Code`, or from `Int` to `Duration` or `Rating`, keeps
  the same column: there is no DDL and no `convert`. `Data` → `Color` or
  `Attach Image` keeps the column too, but a stored value that is not a colour
  or an image URL will fail on that document's next save — and so will every
  other change to that document, since a save casts every field. The same
  applies to a negative `Duration`, while a `Rating` holding a value outside
  its range clamps to `[0, options]` on read. Backfill first, the way `migrations` describes.
- `convert: { from: "Data" }`: authorises a column-type change `migrate` would otherwise refuse, naming the fieldtype the database still holds. No SQL — a conversion a plain cast cannot express goes through expand → backfill → validate → contract.

## DocType properties

```ts
defineDoctype({
  name: "Contract", module: "Sales", label: "Contract", idLabel: "Contract No.",
  idGeneration: { series: "CTR-.YYYY.-.####" } | { field: "code" } | { format: "{index}-{period}" } | { hash: true } | { prompt: true },
  submittable: true, isChild: false, isTree: false, trackChanges: true, allowRename: true, renamedFrom: "Old Name",
  titleField: "id", imageField: "logo", searchFields: ["id", "tax_id"], globalSearch: true, sortField: "modified", sortOrder: "desc", icon: "building-2",
  uniqueKeys: [{ name: "customer_number", fields: ["customer", "number"] }],
  fields: [...],
  permissions: [
    { role: "Manager", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true, report: true, export: true, import: true, share: true, ifOwner: false },
    { role: "Manager", permlevel: 1, read: true, write: true },   // fields declared with permlevel: 1 — see `field-permissions`
  ],
});
```

Series: `.YYYY.`, `.YY.`, `.MM.`, `.DD.`, `.####.` (a zero-padded counter), `.{field}.`. An `id_series` field (Select) lets the user pick the series.

`globalSearch` says whether the desk's global search looks into the DocType; it defaults to on
when there is a `titleField` or `searchFields`, and never applies to a child table or a Single.
See `search`. `share` in a permission row lets the role share one document with another user —
see `sharing`. `import` lets it load rows from a CSV or XLSX file — see `data-import`.

`idLabel` is what the desk calls the document id, a catalogue key like `label`. It heads the
list's id column instead of "ID", and it labels the id when it is asked for (`idGeneration: { prompt }`)
or renamed. It changes display only: filters, `orderBy`, the API and every query still say
`id`. Left out, the column is headed "ID". To hide the column rather than relabel it, see
`idColumn` in `form-api`.

`isTree` makes the documents a hierarchy: the DocType gets a self-referencing Link for the
parent (`parent_<snake(name)>`, or the one `parentField` names) and an `is_group` Check, and
the engine keeps the hierarchy from folding onto itself. See `trees`.

`allowRename` is about renaming a *document*; `renamedFrom` is about renaming the *DocType*,
which moves the table and repoints every stored reference. See `migrations`.

`imageField` names the **Attach Image** (or **Attach**) field that pictures a document, such as
a person's photo or a company's logo. The desk's Cards view shows it on each card, and shows the
title's initials when it is empty (see the Cards view in `form-api`). Unlike `titleField`, it may
sit above permlevel 0: a reader who cannot see it gets the initials. Any other fieldtype is refused
at load.

`titleField`, `imageField`, `sortField`, `searchFields` and `uniqueKeys` name a field by string, so a
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
