# Form scripts (desk)

The file `doctypes/<snake>/<snake>.form.ts`, compiled by the server and loaded when the form opens.

```ts
import { defineForm, ddcore } from "@ddcore/desk-sdk";
import type { Entry } from "../../.ddcore/types";

defineForm<Entry>("Entry", {
  setup(frm) { frm.setQuery("category", () => ({ filters: { kind: frm.doc.kind } })); },
  refresh(frm) {
    frm.setDfProperty("settlements", "cannotAddRows", true);
    if (frm.isNew) return;
    frm.addIndicator(__("Balance: {0}", [ddcore.format.currency(frm.doc.balance)]), "red");
    frm.addButton(__("Record a payment"), () => settle(frm), __("Actions"));
    frm.setInnerGroupAsPrimary(__("Actions"));
  },
  onChange: { kind(frm) { frm.setValue("category", null); } },
  validate(frm) { /* return false to stop the save */ },
  afterSave(frm) {},
});
```

## `frm`

`doc`, `doctype`, `meta`, `isNew`, `isDirty`, `docstatus`, `perm`, `getValue`, `setValue(field | {..}, value)`, `field(field)`,
`setDfProperty(field, prop, value)` (`hidden`, `readOnly`, `reqd`, `label`, `options`, `width`, `cannotAddRows`, `cannotDeleteRows`,
`gridSort`, `gridSortable`, `gridExport`, `gridSelect`, `reportFilters`), `refreshField(field)` (re-runs a Report field's report),
`setQuery(field, () => ({ filters }))`, `toggleDisplay/toggleReqd/toggleEnable`, `addButton(label, fn, group)`, `removeButton`,
`setPrimaryAction(label, fn)`, `setInnerGroupAsPrimary(group)`, `addIndicator(label, colour)`, `addChild(table, values)`, `removeChild(table, idx)`,
`addFieldButton(field, { label, icon, onClick, key })`, `removeFieldButton(field, key?)`,
`trigger(field)`, `save()`, `submit()`, `cancel()`, `reload()`, `discardChanges()`,
`call(method, args, { reload })` → calls the controller's `methods.<method>` and reloads the doc.

`isNew` is always `false` on a Single: before its first save the form already holds the declared
defaults, which are the settings in effect.

### Field width and layout

A form is laid out by sizing its fields, not by splitting it into columns: a line is four slots
(quarters) wide, and `width?: "sm" | "md" | "lg" | "full"` in metadata — or
`frm.setDfProperty(field, "width", value)` at runtime — says how many of them the control takes.

| width | slots | of the line |
| --- | --- | --- |
| `sm`, `md` | 1 | 25% |
| `lg` | 2 | 50% |
| `full` | 4 | 100% |

- **Defaults by fieldtype:**
  - `sm`: `Date`, `Month`, `Time`, `Int`, `Percent`.
  - `md`: `Datetime`, `Float`, `Currency`.
  - `full`: `Text`, `Small Text`, `Text Editor`, `Markdown Editor`, `Code`, `JSON`, `Table`, `HTML`.
  - `lg`: every other type (`Data`, `Link`, `Select`, `Check`, `Attach`, `Password`, etc.).
- **Line packing:**
  Fields fill the line greedily, so `[Status 1/2][Start 1/4][End 1/4]` share one line. A field of
  `s` slots only starts at a multiple of `s`, so a half-line field never begins in the middle of a
  quarter: `[1/4][1/2]` renders as `[1/4][empty 1/4][1/2]`, and a half-line field that no longer
  fits moves to the next line.
- **Dialogs** pack onto a line of two slots, because a modal is too narrow for quarter-line
  controls: there `sm`/`md` is 50% and everything else fills the line.
- **Responsiveness:**
  Below `800px`, every cell collapses to 100% full width.

### Unsaved changes

The desk keeps what the reader typed and did not save in the browser, one draft per user and
record, for seven days. Leaving the form asks first — leave and keep the draft, stay, or
**Discard changes** (`Delete`, or ⌫ on a Mac) and leave; coming back to the record puts the draft
back and says so, and a record saved by someone else in the meantime asks whether to keep the
draft. **Discard changes** in the form's menu — `frm.discardChanges()` — throws the draft away
and goes back to the document as it was loaded, without asking the server for it again.

None of this needs anything from a form script. What it does mean is that `refresh` runs again
after a draft is restored, on a `doc` that is already dirty.

### Buttons on a field

`addButton` is an action on the **document** and belongs in the toolbar. An action on a single
**field** — look this code up, pick one of the e-mails the query returned — reads right only next
to the input it fills, and that is `addFieldButton`:

```ts
frm.addFieldButton("email", {
  label: __("{0} e-mail(s) found", [emails.length]),
  onClick: () => pick("email", emails),
});
frm.addFieldButton("phone", { icon: "search", label: __("Look up"), onClick: () => lookup() });
frm.removeFieldButton("email");
```

The button is rendered by the desk inside the field's control, after the input, so:

- the label is **text**, escaped by the desk — an app never writes HTML and never writes an `esc()`;
- `icon` (a name from the desk's icon set) renders the button icon-only, with `label` as its tooltip
  and accessible name; without `icon` the label is the button's text;
- the field's own `hidden` and `dependsOn` decide whether the button is on screen, and a field the
  reader cannot see carries no button;
- `key` is the button's identity within the field. A second `addFieldButton` with the same key
  **replaces** it, which is what lets a label carrying a count be refreshed after each lookup;
  omit it and the field holds one button. `removeFieldButton(field, key)` removes that one,
  `removeFieldButton(field)` removes every button on the field.

Field buttons are cleared by the same `clearButtons()` that empties the toolbar at the start of
every `refresh` — declare in `refresh` whatever must survive a save or a reload.

## `ddcore` in the desk

- `ddcore.call("app.services.file.fn", args)` — a whitelisted function
- `ddcore.report(name, filters)` — runs a `defineReport`: `{ meta, result: { columns, rows } }`
- `ddcore.db.getValue/getList/count/getDoc/setValue/insert` (asynchronous: `await` them)
- `ddcore.ui.Dialog({ title, fields, values, primaryLabel, primaryAction(values, dlg), dangerLabel, dangerAction(values, dlg), dangerShortcut, onChange(field, values, dlg), size })` → `dlg.show()/hide()/setValue/getValue/setHtml(htmlField, html)/setDfProperty(field, property, value)`
- `ddcore.ui.msgprint(msg, { title, indicator })`, `ddcore.ui.toast`, `ddcore.ui.confirm(msg, title?, { destructive? })` (with `destructive: true` the confirm button is red and "No" is the primary, so Enter keeps the data and `Delete` — ⌫ on a Mac — confirms), `ddcore.ui.prompt(title, fields)`, `ddcore.ui.showError(e)`
- `ddcore.format.currency/date/number/value/statusColor`, `ddcore.datetime.today/addMonths/addDays/monthStart/monthEnd`
- `ddcore.search.global(txt, limit?)` — the documents the global search palette lists. See `search`
- `__("text", [args])` — translation; the key is its English text. See `i18n`.

### Dates and times

`ddcore.datetime` works in **civil dates** (`"YYYY-MM-DD"`) with exactly the semantics of
`ddcore.utils` on the server: `today()` is the day in the **site's** timezone — the same day the
server calls today, which is what makes `due_date < today()` agree on both sides — and
`addMonths` clamps the day to the end of the target month: `addMonths("2026-01-31", 1) === "2026-02-28"`.

`Datetime` fields, by contrast, are **instants**: they travel as ISO UTC and the control shows and
accepts them in the site's timezone. Never slice the string (`v.slice(0, 16)`) to fill a
`datetime-local` — that shows UTC as if it were local time; use `$lib/datetime`
(`toDatetimeLocal`/`fromDatetimeLocal`).

Never format a date, a number or a currency by hand: `ddcore.format.*` derives the order, the
separators and the symbol from the reader's language and the site's currency.

## `defineListView`

Adjusts a DocType's list from a global script (`client/*.ts`):

```ts
import { defineListView, ddcore } from "@ddcore/desk-sdk";

defineListView("Entry", {
  columns: ["contract", "period", "amount", "balance"], // in place of the meta's inListView
  filters: { status: "Open" },          // initial filters (the URL's query string still wins)
  orderBy: "due_date asc",
  pageSize: 50,
  formatters: { amount: (v, row) => ddcore.format.currency(v) }, // the cell's text
  indicator: (row) => (row.balance > 0 ? { label: __("Open"), color: "red" } : { label: __("Settled"), color: "green" }),
  docstatusFilter: false,               // hides the "Document status" filter of a submittable DocType
  modifiedColumn: false,                // hides the trailing "Modified" column
  idColumn: false,                      // hides the leading document-id column
});
```

Every key is optional. `formatters` returns **text** (not HTML); `indicator` replaces the default
status column and may return `null` to show nothing on that row. `docstatusFilter: false` suits a
submittable DocType whose `status` field already separates draft, submitted and cancelled — the
docstatus filter would only repeat it.

`idColumn: false` suits a DocType whose id means nothing to a reader: a `hash`, or a code
already shown in another column. The list leaves the column out on its own when the title field
is a column and is the id (`idGeneration: { field }` equal to `titleField`). With the column hidden,
the row stays clickable and the title field's cell links to the document. To relabel the
column instead of hiding it, set `idLabel` on the DocType (see `fieldtypes`).

Most lists need no `indicator` at all: declare `optionColors` on the status field and the desk
colours and translates it on its own. Reach for `indicator` only when the label is not a field
value — and never key a colour on text a reader sees, since that changes with the language.

Badges and extra filter choices go together when a status has sub-states that are not a value of the
field — say, open entries that are also overdue:

```ts
defineListView<Entry>("Entry", {
  fields: ["overdue_count"],            // fetched though not a column
  badges: (row) => (row.overdue_count ? [{ label: __("Overdue"), color: "red" }] : []),
  filterOptions: {
    status: [{ value: "overdue", label: __("Overdue"), filters: [["status", "=", "Open"], ["overdue_count", ">=", 1]] }],
  },
});
```

`badges` are drawn after the status, in the same cell. `filterOptions` appends choices after a
divider to that field's standard filter; choosing one sends its `filters` instead of an equality, and
the URL carries its `value` (`?status=overdue`). Filters only reach columns, so a sub-state computed
from other documents has to be stored on the document to be filterable.

### Views: Tree, Calendar, Kanban, Gantt and Cards

Besides the table, a list can offer other views of the same filtered rows. A segmented switcher in
the list header shows the views, and the choice is kept in the URL (`?view=kanban`) and per DocType
in the browser. A view appears once it is configured: `calendar` needs its `field`, `kanban` its
`field`, and `gantt` both `startField` and `endField`; `list` and `cards` are always available.
`tree` needs nothing configured here but a DocType declared `isTree`, and comes first for one, so
a hierarchy opens as a hierarchy (see `trees`); its `tree` option composes the node label and
sets the order (`tree: { title: "{acronym} - {title}", orderBy: "title asc" }`). `views` sets the order, or a subset, and is
filtered by the same rule — a view listed there but never configured is dropped rather than shown
as a button that falls back to the table:

```ts
defineListView<Task>("Task", {
  views: ["list", "calendar", "kanban", "gantt", "cards"], // default: list, then each configured view, then cards
  calendar: { field: "start_date", endField: "due_date", titleField: "title", colorField: "status" },
  kanban: { field: "priority", columns: ["High", "Medium", "Low"], titleField: "title", subtitleField: "project", colorField: "status" },
  gantt: { startField: "start_date", endField: "due_date", titleField: "title", colorField: "status", progressField: "percent_done" },
  card: { title: "title", subtitle: "project", dateField: "due_date", image: "cover" },
});
```

- **Calendar:** clicking a day lists that day's records, filtered on the calendar's `field` (a
  `Datetime` field matches the whole day in the site's time zone); the **+** in a day's corner,
  shown on hover, opens a new record with the field set to that day. A calendar on `creation` or
  `modified` has the **+** only. With `endField` (a Date or Datetime), each record is drawn as a
  bar across every day from `field` to `endField`, and the bar wraps at the end of each week. A
  record keeps its row within the week, and it shows on the calendar from the first day to the last
  of the span, including one that started before the month shown. A record with no end (or an
  end before its start) takes its start day alone.
- **Cards** is the default on a phone.
  - `title` defaults to the DocType's `titleField`, then `id`. `dateField` defaults to the first
    visible Date or Datetime field.
  - `image` names an **Attach Image** (or **Attach**) field, and defaults to the DocType's
    `imageField` (see `fieldtypes`).
    - Each card then shows it beside the title as a square thumbnail, cropped to fill.
    - Thumbnails load lazily. A `/private/files/…` image follows the File's read access, like any
      other private file.
  - A card whose image is empty or fails to load shows an avatar instead:
    - it holds the title's initials, from the first letter of the first and last words
      ("MARIA DA SILVA" → `MS`, "Acme" → `A`);
    - its colour is derived from the title, so the same title always gets the same colour;
    - it also appears when the field is above the reader's permission level.
- **Calendar** plots each row on the day of `field` (Date or Datetime) and loads the visible month.
- **Kanban** makes a column of each value of a **Select** `field`.
  - Columns follow `columns` or the field's options, with their translated labels and `optionColors`.
  - A value outside those still gets a column, so no card is hidden, and rows with no value go to "(empty)".
  - `titleField` defaults to the DocType's `titleField`, then `id`; `colorField` defaults to `field`,
    and a card shows an indicator only when `colorField` names another field.
  - A `field` the DocType does not have — or one above the reader's permission level — renders
    "The Kanban field {0} is not a field of {1}". Nothing enforces **Select**: another fieldtype still
    renders, with the columns built from the loaded rows alone.
  - Dragging a card to another column saves the field at once, sending the row's `modified`, so a
    stale card is refused. The card moves back if the server refuses the save (permission, workflow,
    validation, or a concurrent change).
  - Cards are draggable only when the user may write the DocType and the field is not read-only
    (declared, or above the user's permission level); submitted documents never move.
  - Group by a field the user edits: a field a controller computes is usually `readOnly`, and its
    board is read-only.
- **Gantt** draws a bar from `startField` to `endField` (Date or Datetime; `creation` works too).
  - Scales are Day, Week and Month, with previous, today and next navigation; clicking a bar opens the document.
    Week is the default and shows 12 weeks; Day shows 21 days, Month 12 months. The window starts one week —
    one month, at the Month scale — before the anchor's, and previous/next move 7 days, 4 weeks or 3 months.
  - Rows order by `startField asc` unless `orderBy` says otherwise. `titleField` defaults to the DocType's
    `titleField`, then `id`, and `colorField` to `status`.
  - The view loads only rows overlapping the window, a row without an end date is not drawn, and an end
    before the start is clamped to the start.
  - `progressField` (0–100) shades the bar. The view is read-only: bars are not dragged.

**Colour.** `colorField` usually names a **Select**, and the view paints each record with the
indicator colour of its value: the field's `optionColors`, then the framework's canonical statuses,
then a colour derived from the text. When `colorField` names a **Color** field, the record is painted
with the colour it holds instead — a calendar entry and a Gantt bar take its hue (lightened behind
the text, darkened for the text and the dot), and a Kanban card takes it as a coloured left edge
rather than an indicator showing the hex. A record whose Color field is empty keeps the neutral grey.
This is what lets a user pick any colour for a record (a campaign, a project) with the Color field's
picker and see it on the boards.

Kanban, Gantt and Calendar load up to 500 rows matching the filters instead of a page. When more
match, Kanban and Gantt say so and ask for narrower filters.

A DocType may have **several** form scripts: its owner's `<snake>.form.ts`, plus one per app extending it
(`extensions/<snake>.form.ts` — see `extending`). Their handlers accumulate, in app load order; every `refresh`,
`validate` and `onChange` runs. `defineListView` is the exception: one per DocType, and the last registration wins.

Global scripts (`client/*.ts`, listed under `desk.include` in `ddcore.app.ts`) run across the whole desk: masks, shortcuts, `defineForm` for several DocTypes.
Date field inputs carry `data-fieldname` and `data-fieldtype` for masks applied by event delegation.
