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
| Autocomplete | text | free text with suggestions: `options` lists them (a list, or one per line) and never limits what is stored; outer spaces are trimmed. See below |
| Barcode | text | `options` is the symbology: `"Code128"` (the default), `"EAN-13"` or `"QR"`; the value is validated for it, and an EAN-13 typed with 12 digits gets its check digit. The form previews the code and can scan it with the camera. See below |
| Signature | text | a signature drawn on the form, stored as a PNG data URL (`data:image/png;base64,…`, as Frappe stores it); at most 64 KiB and 2000×1000 pixels. Never unique, indexed, a standard filter, in the list, or a title, search, sort or key field. See below |
| Geolocation | jsonb | points, lines and polygons drawn on a map, stored as a GeoJSON FeatureCollection, `[longitude, latitude]`; coordinates rounded to 7 decimals, no altitude or properties; at most 500 shapes and 64 KiB. Never unique, indexed, a standard filter, or a title, search, sort or key field. See below |
| Link | text | `options: "DocType"`; existence validated; index created automatically. A Link to a virtual DocType stores `"<Source>:<id>"` (see `virtual-doctypes`) |
| Dynamic Link | text | `options: "<the field holding the DocType>"`; the DocType and the document are validated on save. When that field is a `Data`, the desk shows it as a list of the DocTypes the user can see, and changing it clears the link |
| Table | (child table) | `options: "Child DocType"` with `isChild: true`; `gridEditMode: "dialog"` turns off inline editing; `gridSort`, `gridSortable`, `gridExport`, `gridSelect`, `gridFilters`, `gridSearch` add a default order, header sorting, CSV/XLSX export, row selection, preset filters and a search box; `gridIndex: false` hides the `#` column. See "Form grids" below |
| Table MultiSelect | (child table) | several links to one DocType, edited as pills: `options` is a child DocType with exactly one Link field. See below |
| Attach | text | the file's URL (`/files/..` or `/private/files/..`); the desk shows an icon that opens the file and shows its original name, size and type on hover — `showFileName: true` also shows the name beside it |
| Attach Image | text | an Attach restricted to png, jpg, gif or webp, refused at upload as well as on save; SVG is not one of them, because it carries script; the thumbnail stands in for the icon, and `showFileName` works the same |
| JSON | jsonb | |
| Password | text | not hashed automatically; never read back through the API, never in Version, never exported. **Not for an integration credential** — see below |
| Vault | — | virtual field backed by the encrypted vault (`ddcore_vault`); never a column in `tab_<doctype>`, never in Version or export; masked in Desk and API. See [vault.md](vault.md) |
| Section Break / Tab Break | — | layout; `label`, `collapsible`, `dependsOn` on a Section. A section with no `label` holding only a Table or Report has no card (see "Form grids") |
| HTML | — | `options` is the rendered HTML |
| Report | — | `options` is a `defineReport` name, run for this document through `reportFilters` and shown as a read-only grid. See "Form grids" below |

`File` is not in this table: it is a field of `ddcore.ui.Dialog` only, which hands the chosen file
to the script without storing it. See "`ddcore` in the desk" in [form-api](form-api.md).

## Form grids

A `Table` and a `Report` field are grids, and four properties shape both:

```ts
{ fieldname: "students", fieldtype: "Table", options: "Course Student", label: "Students",
  gridSort: { field: "employee_name", order: "asc" }, gridSortable: true, gridExport: true, gridSelect: true },
```

- `gridSort: { field, order? }`: the order the grid shows its rows in. On a Table, `field` is a
  child field (or `idx`) and is checked at load; on a Report it is a report column. `order` is
  `"asc"` (the default) or `"desc"`.
- `gridSortable: true`: clicking a column header sorts by it: ascending, descending, then back to
  `gridSort`. A Link sorts by its title, a Select by its label, and empty cells go last.
- `gridExport: true`: CSV and XLSX buttons, shown only to a user with the `export` permission
  on the DocType (a Report checks its `refDoctype`). They export the selected rows if there are
  any, otherwise every row, in the order on screen, with Link titles and Select labels.
- `gridSelect: true`: a checkbox per row and a "select all". On a Table it adds "Delete
  selected", unless the grid is read-only or `cannotDeleteRows` is set.
- `gridIndex: false` (Table only): hides the `#` column. Worth it on a grid sorted by something
  other than `idx`, where the stored position reads as noise.
- `gridFilters: [{ label, filters, default? }]`: preset toggles above the grid. A toggle that is
  on shows only the rows matching its `filters`, written like a list's: tuples
  (`[["in_class", "=", 1]]`, `[["grade", ">=", 7]]`) or an object (`{ in_class: 1 }`). The
  operators are the list's except the tree ones; a Check compares as true/false, so `1`, `true`
  and an empty value read as `0` agree. They run in the browser on the loaded rows, so a
  `computed` column filters like any other. Toggles that are on combine with AND, and
  `default: true` turns one on when the form opens. Selection, "select all", export (and a
  Report's totals) follow the rows on screen. `label` is a catalogue key. On a Table the fields
  named are checked at load; on a Report they are report columns.

  ```ts
  gridFilters: [
    { label: "Only students in the class", filters: [["in_class", "=", 1]], default: true },
    { label: "Students who still need the course", filters: { needs_course: 1 } },
  ],
  ```
- `gridSearch: ["field", ...]`: a search box above the grid, before the `gridFilters` toggles,
  showing only the rows where the columns named contain the text typed. Case- and
  accent-insensitive (`sao` finds `São`); with several words, each must be found in one of the
  columns. A Link matches by its id and by its title, a Select by its value and by its label; a
  Check or a Signature has no text to find. On a Table the fields may be `hidden` or off the grid
  (they are checked at load); on a Report they are report columns. It combines with the active
  `gridFilters` (AND), and selection, "select all", export (and a Report's totals) follow the rows
  on screen.

  ```ts
  { fieldname: "attendance", fieldtype: "Table", options: "Training Class Attendance",
    gridSearch: ["employee", "unit"],   // a Link each: typing a name or a unit finds the row
    gridFilters: [{ label: "Only students in the class", filters: [["in_class", "=", 1]] }] }
  ```

**Sorting, filtering and searching are display only.** A child row's `idx`, which is the order the document stores and
`ddcore.db` reads, stays what the user saved, and the `#` column keeps showing it (unless `gridIndex: false` hides it).

**A grid alone in its section.** A section with no heading whose only visible field is a Table
or a Report is drawn without the section card: the grid's own card is the frame. That covers
the section a `Tab Break` opens. Add `hideLabel: true` to the grid and a tab shows just the grid,
without repeating the tab's name above it:

```ts
{ fieldname: "students_tab", fieldtype: "Tab Break", label: "Students" },
{ fieldname: "attendance", fieldtype: "Table", options: "Training Class Attendance", label: "Students", hideLabel: true },
```

A form script can make a Table's or a Report's cells act on a click (`grids.<field>.onCellClick`), react to a
change of one child field (`grids.<table>.onChange`) and change a row with `frm.setRowValue`; see [form-api.md](form-api.md#grids-row-changes-and-cell-clicks).

### Computed columns

`computed: true` declares a field with no column: never stored, always read-only. The
controller's `onLoad` sets its value each time the document is loaded for a form, a print, a
save response or a method response. It works on the parent and on child rows, which is how a
grid shows values that live in other documents:

```ts
// course_student.doctype.ts
{ fieldname: "grade", fieldtype: "Float", label: "Grade", computed: true, inListView: true },

// course.controller.ts
onLoad(doc) {
  for (const s of doc.students || []) s.grade = ddcore.db.getValue("Exam Result", { course: doc.id, employee: s.employee }, "grade");
},
```

Only the computed fields survive `onLoad`: anything else it changes is discarded, and nothing
is written. A computed field cannot be `reqd`, `unique` or `fetchFrom`, cannot be a Table,
Password or Vault, and cannot be filtered, sorted or grouped on in a list. `ddcore.db.getDoc`
and `doc.reload()` in server code do not run `onLoad`.

### Report fields

A `Report` field runs a Script Report for the document it sits in, as a grid. Use it for rows
the document does not hold: attendance and grades per student, say, from other DocTypes.

```ts
{ fieldname: "attendance", fieldtype: "Report", label: "Attendance", options: "Course Attendance",
  reportFilters: { course: "id" }, gridSort: { field: "employee_name" }, gridSortable: true, gridExport: true },
```

- `options` is the report's `name`; a report that does not exist fails the load.
- `reportFilters` maps each report filter to the field of this document (or `id`) whose value
  it takes.
- The report's own `roles` and its `refDoctype`'s `report` permission still decide who sees
  the grid; a user without them sees the error in its place.
- It runs when the form loads, after a save or a reload, and on `frm.refreshField(fieldname)`.
  A new document shows "Save the document first".
- Its rows are read-only; `gridSelect` only chooses what to export. The totals row goes along
  with a full export, not with a selection.
- A form script acts on a row with `grids.<field>.onCellClick.<column>`: the column's non-empty
  cells become buttons and the handler receives the row as the report returned it; see
  [form-api.md](form-api.md#grids-row-changes-and-cell-clicks).

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

## Autocomplete

A text field that offers values as the user types and accepts any other: a
city, a colour name, a tag an app wants to keep consistent without making a
DocType for it. The list filters ignoring case and accents, the arrow keys and
Enter pick, and whatever is in the box when it loses focus is the value.

```ts
{ fieldname: "city", fieldtype: "Autocomplete", label: "City", options: ["São Paulo", "Rio de Janeiro"] }
```

- **The suggestions are text, not labels.** What the list shows is what is
  stored, so they are never translated and never validated on the server.
- **Suggestions that depend on the document** come from a form script, which
  replaces them with `frm.setDfProperty("city", "options", list)` — in
  `refresh`, or in the `onChange` of the field they depend on. A dialog's field
  takes `dlg.setDfProperty` the same way. `frm.setDfProperty` addresses the
  form's own fields only, so the suggestions of a **child-table column** are
  the static ones its DocType declares.
- **A standard filter** on an Autocomplete offers the same suggestions and, like a Data one,
  matches the exact value.

## Barcode

Text that is also a code a scanner reads: a product's GTIN, a stock label, a
link printed as a QR code. The form shows the code under the text box as it is
typed, and a USB scanner works as a keyboard — it types the code and presses
Enter.

```ts
{ fieldname: "gtin", fieldtype: "Barcode", label: "GTIN", options: "EAN-13" }
```

- **Symbologies:** `Code128` (the default) takes printable ASCII — letters,
  digits, spaces and symbols, no accents — up to 80 characters. `EAN-13` takes
  12 or 13 digits: with 12 the server appends the check digit, with 13 it
  verifies it. `QR` takes any text up to 1000 bytes (error correction M). The
  server trims outer spaces, and a value its symbology refuses fails the save
  naming the field. Any other `options` fails `migrate`.
- **The image** comes from `GET /api/barcode?symbology=&value=`, an SVG any
  signed-in user can ask for, Website Users included — a portal page shows the
  same preview. It reads nothing from the database.
- **The camera:** a Scan button appears where the browser has `BarcodeDetector`
  (Chrome and Edge on desktop, Chrome on Android) and the site is served over
  HTTPS or from `localhost`. Elsewhere the field is typed or filled by a USB
  scanner.
- **Print:** the standard layout draws the code with its label; a template uses
  `b.barcode(value, symbology, title)`. See `print`.
- `Data` → `Barcode` keeps the column. The first save of each document trims
  and completes EAN-13 check digits without recording a Version; a stored value
  the symbology refuses fails that save, so check the data first.

## Signature

A signature drawn with a finger, a pen or the mouse: a delivery received, a
form acknowledged.

```ts
{ fieldname: "received_by", fieldtype: "Signature", label: "Received by" }
```

- **Storage:** a PNG data URL in a text column, `data:image/png;base64,…`,
  which is what Frappe stores — an imported Frappe value keeps working. The
  server checks every value on save: that exact prefix, base64 that decodes,
  a real PNG, at most 64 KiB decoded and 2000×1000 pixels. Anything else — a
  JPEG, an SVG, a URL — fails the save naming the field. An empty value is
  stored as null.
- **The form** shows a pad; each stroke that ends commits the pad cropped to
  its strokes, and Clear empties it. A stored signature shows as an image with
  a "Sign again" button, which opens an empty pad — the stored image is kept
  until a new stroke or Clear replaces it. Read-only, only the image shows.
- **Too large to index or list.** A value is tens of KiB, so `migrate` refuses
  `unique`, `searchIndex`, `inStandardFilter` and `inListView` on it, and
  refuses it as the DocType's `titleField`, `sortField`, in `searchFields`,
  `linkSubtitle`, `linkOrderBy` or a `uniqueKeys` entry, and as a Table's `gridSort.field`.
  On a **child** DocType `inListView` is allowed: the grid column says
  "Signed", and clicking it opens the row dialog, where the pad is — a grid
  cell never edits a signature inline.
- **History:** a Version records `sha256:` and 12 hex digits of each side
  instead of the image, so the timeline says Signed, Removed or Changed.
- **Elsewhere** it reads as "Signed": list cells, grid exports and child-table
  cells in print. Data Import does not take it (it is signed on the form);
  `/api/export` carries the data URL, as a faithful copy.
- **Print:** the standard layout draws it as an image, at most 25 mm tall; a
  template uses `b.signature(dataUrl, title)`. See `print`.

## Geolocation

Places drawn on a map: a customer's address pinned, a delivery route, the area
a team covers.

```ts
{ fieldname: "area", fieldtype: "Geolocation", label: "Delivery area" }
```

- **Storage:** a GeoJSON FeatureCollection in a `jsonb` column. The server
  accepts a FeatureCollection, a single Feature, or a bare `Point`,
  `MultiPoint`, `LineString` or `Polygon` (wrapped in a collection), as an
  object or as JSON text, and stores one canonical shape:

  ```json
  { "type": "FeatureCollection", "features": [
    { "type": "Feature", "geometry": { "type": "Point", "coordinates": [-46.6333, -23.5505] }, "properties": {} }
  ] }
  ```

  Positions are **`[longitude, latitude]`** — GeoJSON's order, the reverse of
  what people write. Coordinates must be finite numbers, the longitude within
  ±180 and the latitude within ±90, and are rounded to 7 decimals (about
  1 cm). An altitude and every `properties` member are dropped, so a Frappe
  circle arrives as its centre point. A polygon's ring is closed if it is not;
  a ring needs at least 3 corners and a line 2 points. `MultiLineString`,
  `MultiPolygon`, `GeometryCollection` and a feature without a geometry fail
  the save naming the field, as do more than 500 features or more than 64 KiB.
  An empty collection is stored as null.
- **Canonical, so stable:** what the server stores is what it reads back, so a
  document opened and saved untouched records no Version, and a read-only or
  submitted Geolocation compares equal to itself.
- **In a hook** the value is an object: `doc.area?.features.length`. The
  generated types give it `GeoFeatureCollection | null`, exported by
  `@ddcore/sdk` with `GeoFeature`, `GeoGeometry`, `GeoPoint` and the rest.
- **The form** shows a map with Point, Line, Polygon and Delete tools. A click
  adds a point, or a vertex of the line or polygon being drawn; Finish or a
  double-click ends it, and every finished shape is saved to the field at
  once. Delete removes the shape clicked. "Use my location" appears where the
  browser can ask for it (HTTPS or `localhost`). The map fits the value when
  it opens; read-only, the tools are hidden. There is no vertex editing: to
  change a shape, delete it and draw it again. The default width is `full`,
  and a grid opens the row dialog for it instead of editing the cell.
- **Elsewhere** it reads as a summary: a single point as `lat, lon` (five
  decimals), anything else as what it holds — "2 points, 1 polygon". Lists,
  grid exports, history and print all say it that way; `/api/export` carries
  the GeoJSON.
- **Print draws no map.** The standard layout prints the summary as a
  key/value. A map would need its tiles fetched by the PDF renderer, from a
  third-party server, while it runs — a server-side request to wherever the
  tile URL points, which is not something print should make.
- **Data Import** takes GeoJSON (a cell starting with `{`) or a point typed
  latitude first: `-23.5505; -46.6333` always, `-23.5505, -46.6333` only when
  the file's decimal separator is `.`. See `data-import`.
- **Too large to index.** As for a Signature, `migrate` refuses `unique`,
  `searchIndex` and `inStandardFilter`, and refuses it as `titleField`,
  `sortField`, in `searchFields`, `linkSubtitle`, `linkOrderBy`, a `uniqueKeys` entry or a
  Table's `gridSort.field`. `inListView` is allowed: the column shows the
  summary.
- **Tiles** come from `DDCORE_MAP_TILE_URL` (a Leaflet URL template with
  `{z}`, `{x}`, `{y}`) with the credit in `DDCORE_MAP_ATTRIBUTION`, both in
  `.env`. The browser fetches them; the server never does. The default is
  OpenStreetMap's own tile server with "© OpenStreetMap contributors", which
  is fine for development and a small internal site but **not for
  production**: its [tile usage policy](https://operations.osmfoundation.org/policies/tiles/)
  rules out heavy use. Point a production site at a provider of its own
  (a paid service, or tiles you host) and give its attribution.

## Table MultiSelect

A document that points at *several* documents of one DocType — tags, categories,
the regions a price applies to — holds them as child rows, one value per row, and
the desk edits them as pills with a search to add more. It is Frappe's field of
the same name, stored the same way, so its data comes across from Frappe
unchanged.

```ts
// the child: exactly one Link, which is the value
defineDoctype({ name: "Task Tag", module: "Projects", isChild: true, fields: [
  { fieldname: "category", fieldtype: "Link", label: "Category", options: "Task Category", reqd: true },
]});

// on the parent
{ fieldname: "tags", fieldtype: "Table MultiSelect", label: "Tags", options: "Task Tag" }
```

- **The child's shape is checked at load.** It must be `isChild` and have exactly
  one Link field. Any other field it declares must be optional or have a
  `default`, because the control has no way to fill it.
- **The value is rows**, exactly as a Table's: `[{ id, category, idx, … }]` in
  every response, in the generated types, in export and in print templates.
- **A write may send the ids instead:** `"tags": ["Shipping", "Support"]` over
  REST, MCP or `ddcore.db`. A value the document already held keeps its row and
  its id, so sending the same list again changes nothing and records no Version.
- **Every value is a Link.** It must exist, and a user with User Permission
  scopes on the target can only choose values inside them. **An empty value is
  refused, and so is the same value chosen twice.**
- **`reqd`** means at least one value.
- **Filtering** works the way it does on any child table, by naming the child's
  field: `filters: [["Task Tag.category", "=", "Support"]]`.
- **Changing a field between `Table` and `Table MultiSelect`** needs no
  migration, because the rows are the same rows. The child only has to meet the
  shape above.

Not supported: a Table MultiSelect is not a list column or a Kanban/Calendar
field. It cannot be loaded from a spreadsheet by Data Import, although
`ddcore import` does load it. It cannot appear on a portal page.

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

On a site with tenancy, a `Vault` field of a `shared` DocType keeps its secret in the platform space; read it from any space with `ddcore.vault.get(key, { shared: true })`. See [tenancy.md](tenancy.md).

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
readOnlyDependsOn, mandatoryDependsOn, allowOnSubmit, setOnlyOnce, noCopy, inListView, inStandardFilter, searchIndex,
length, precision, description, columns (grid width 1–12), width (`"sm"` | `"md"` | `"lg"` | `"full"`), gridEditMode (`"inline"` default or `"dialog"`),
gridSort, gridSortable, gridExport, gridSelect, gridFilters, gridSearch (Table / Report), gridIndex (Table), reportFilters (Report), computed, showFileName (Attach / Attach Image), collapsible, bold, hideLabel,
permlevel, renamedFrom, convert`

- `permlevel`: 0–9, default 0. A field above 0 is read and written only by roles granted that level by a permission row with the same `permlevel`; the server omits it from every response and refuses a change from anyone else. `hidden` and `readOnly` are screen hints and protect nothing. See `field-permissions`.

- `setOnlyOnce`: the value cannot change once the document exists — a tenant key, say. The server refuses a different value (clearing it included) on every write path: `save()`, REST, `ddcore.db.setValue` / `doc.dbSet`, Data Import; nothing lifts it, not `ignorePermissions` nor a workflow's `updateFields`. An empty value may be filled once. On a child DocType it holds a saved row's value; a new row takes any. The desk shows the field read-only once it holds a saved value. Not for a layout, `Table` or `computed` field. To repair a value, use `ctx.sql` in a migration patch.

- `noCopy`: **Duplicate** in the desk's form menu leaves the value behind, and the copy takes the field's default — a reference the provider issued, a status the server writes. Duplicate never copies a `unique` field (the copy could not be saved with it) nor a `readOnly` one other than a `fetchFrom`, so those need no `noCopy`. On a `Table`, the copy starts with no rows; a child DocType's own fields take it too. The desk honours it; the server does not see a duplicate as anything but an insert, so a controller that must refuse a copied value still checks it in `validate`.

- `label` and `description` are **catalogue keys**: write them in English. See `i18n`.
- `hideLabel: true`: the form doesn't show the label (screen readers still read it). It still names the field in a grid's export, its row dialog and error messages, so keep a real `label`. Any field except a Section Break, Tab Break or HTML — a Report takes it, like a Table.
- `width`: `"sm"` | `"md"` | `"lg"` | `"full"`. How much of a form line the control takes: `sm`/`md` a quarter, `lg` a half, `full` the whole line — a form has no columns, its shape comes from the widths. Defaults to `sm` for `Date`, `Month`, `Time`, `Int`, `Percent`, `Rating`, `Color`; `md` for `Datetime`, `Float`, `Currency`, `Duration`; `full` for `Text`, `Small Text`, `Text Editor`, `Markdown Editor`, `Code`, `JSON`, `Geolocation`, `Table`, `HTML`, `Report`; `lg` for every other type, `Table MultiSelect` included. Fields pack a line greedily, aligned so a half-line field never starts in the middle of a quarter. See `form-api`.
- `default`: a literal value, or `"Today"` for Date/Datetime, `"__user"` for the current user.
- `fetchFrom: "project.assignee"`: copied from the linked document on save. When `readOnly` it always overwrites; otherwise it fills only when empty.
- `dependsOn`, `readOnlyDependsOn`, `mandatoryDependsOn`: a JS expression over `doc` (`"doc.type == 'PJ'"`) or a field name (truthy). Evaluated in the desk **and** on the server. The desk judges the document on screen. For `readOnlyDependsOn` the server refuses a change only when the expression holds on the stored document **and** on the one being saved, so a save that unlocks a field (`status` back to `"Open"`) may edit it too, and one that locks it keeps the edits made before.
- `optionColors` (Select): `{ Open: "blue", Overdue: "red" }`, keyed by the canonical value — never by its label.
- `renamedFrom: "old_name"` (or a list, oldest first): the fieldname this field used to have, so `migrate` renames the column instead of adding an empty one beside it. See `migrations`.
- `options` beyond Link, Table, Table MultiSelect and Select: `Rating` takes the number of stars,
  `Code` the language, `Duration` its display flags, `Autocomplete` its
  suggestions and `Barcode` its symbology (`Signature` and `Geolocation` take none). None of them are catalogue keys — they are never translated,
  unlike a Select's options.
- Changing a field from `Text`, `Small Text` or `Data` to `Text Editor`,
  `Markdown Editor` or `Code`, or from `Int` to `Duration` or `Rating`, keeps
  the same column: there is no DDL and no `convert`. So does `Data` →
  `Autocomplete`, whose first save trims outer spaces without recording a
  Version, and `Data` → `Barcode`, whose first save also completes an EAN-13's
  check digit — but a value the symbology refuses fails that save. `Data` → `Color` or
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
  titleField: "id", imageField: "logo", searchFields: ["id", "tax_id"], linkSubtitle: ["tax_id"], globalSearch: true, sortField: "modified", sortOrder: "desc", icon: "building-2",
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

`trackChanges` writes a Version, with the diff, each time a save changes the document. The
history outlives the document: a delete keeps its Versions and adds a final one holding the
document as it was. See "What a delete leaves behind" in `controller-api`.

`allowRename` is about renaming a *document*; `renamedFrom` is about renaming the *DocType*,
which moves the table and repoints every stored reference. See `migrations`.

`imageField` names the **Attach Image** (or **Attach**) field that pictures a document, such as
a person's photo or a company's logo. The desk's Cards view shows it on each card, and shows the
title's initials when it is empty (see the Cards view in `form-api`). Unlike `titleField`, it may
sit above permlevel 0: a reader who cannot see it gets the initials. Any other fieldtype is refused
at load.

`linkSubtitle` picks what a **Link** dropdown shows under each option's title: the listed
fields, in order, joined with ` · `. Left out, the line shows the id and every `searchFields`
value, so a DocType with random ids shows the random id. `linkSubtitle: ["cpf"]` shows the CPF
alone, and `"id"` can be listed to keep the id. The fields are shown, not searched: to find a
row by typing its value, list the field in `searchFields` too. Like `searchFields`, they must be
permlevel 0. An app can set it on another app's DocType with `extendDoctype`.

A **Link** dropdown, and the picker of a **Table MultiSelect**, lists its options by **title**,
A to Z, whatever the target's `sortField` — the list's order and a picker's are different things.
When something is typed, the options whose id or title equals it come first, then those starting
with it, then the rest of the matches in that same order. Text compares ignoring case and accents,
so "Álamo" sits between "Abeto" and "Bosque". A DocType without a `titleField` falls back to its
`sortField`, and a `translateId` DocType orders by the translated id. `linkOrderBy` sets the order
instead, as `"field [asc|desc], ..."`:

```ts
defineDoctype({ name: "Unit", titleField: "unit_name", sortField: "code", sortOrder: "asc",
  linkOrderBy: "code asc", ... });     // the list and the dropdown both by code
```

Its fields must exist, sit at permlevel 0 and be small enough to sort by, like `sortField`'s.
An app can set it on another app's DocType — `User`, say — with `extendDoctype`.

A Link field whose value is empty shows a **+** inside the input when the user may create the
target DocType (not in a portal). It opens a dialog asking for the target's title field, its
prompted id and every required field a person fills in (it leaves out hidden, read-only and
`fetchFrom` fields), with the text typed so far in the title. **Create** inserts the document
through the ordinary API, so the controller and validations run, and puts it in the field. When
the target has a required child table, the **+** opens the full form in a new tab instead.

`titleField`, `imageField`, `sortField`, `searchFields`, `linkSubtitle`, `linkOrderBy` and `uniqueKeys` name a field by string, so a
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
