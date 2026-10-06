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
  afterDelete(frm) { /* the form's Delete succeeded; frm.doc is what was deleted */ },
});
```

## `frm`

`doc`, `doctype`, `meta`, `isNew`, `isDirty`, `docstatus`, `perm`, `getValue`, `setValue(field | {..}, value)`, `field(field)`,
`setDfProperty(field, prop, value)` (`hidden`, `readOnly`, `reqd`, `label`, `description`, `options`, `width`, `cannotAddRows`, `cannotDeleteRows`,
`gridSort`, `gridSortable`, `gridExport`, `gridSelect`, `gridFilters`, `gridSearch`, `gridIndex`, `hideLabel`, `reportFilters`), `refreshField(field)` (re-runs a Report field's report),
`setQuery(field, () => ({ filters }))`, `toggleDisplay/toggleReqd/toggleEnable`, `addButton(label, fn, group)`, `removeButton`,
`setPrimaryAction(label, fn)`, `setInnerGroupAsPrimary(group)`, `addIndicator(label, colour)`, `addChild(table, values)`, `removeChild(table, idx)`,
`setRowValue(table, row | rowId, field | {..}, value)`,
`addFieldButton(field, { label, icon, onClick, key })`, `removeFieldButton(field, key?)`,
`trigger(field)`, `save()`, `submit()`, `cancel()`, `reload()`, `discardChanges()`,
`call(method, args, { reload })` → calls the controller's `methods.<method>` and reloads the doc,
`onRealtime(event, handler)` → runs `handler(payload)` on each `ddcore.publish(event, …)` from the server while the form is open.

```ts
// a thread that shows new messages as they arrive; the server publishes
// ddcore.publish("my_app.chat", { session: doc.id }, { doctype: "Chat Session", id: doc.id })
defineForm("Chat Session", {
  onload(frm) {
    frm.onRealtime("my_app.chat", (p) => { if (p.session === frm.doc.id && !frm.isDirty) frm.reload(); });
  },
});
```

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
  - `full`: `Text`, `Small Text`, `Text Editor`, `Markdown Editor`, `Code`, `JSON`, `Geolocation`, `Table`, `HTML`.
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

Where the button goes depends on the field: beside the input on a field that has one; in the
toolbar above the grid, to the right, on a `Table` or a `Report` field; under the content on an
`HTML` field. That makes it the way to put an action next to a grid:

```ts
// a Report field listing the course's classes, and the button that creates one
frm.addFieldButton("classes", {
  label: __("New Class"),
  onClick: () => ddcore.route("/app/Training%20Class/new?course=" + encodeURIComponent(frm.doc.id)),
});
```

A `Report` field has no grid until its document is saved, so its buttons appear only then.

The button is rendered by the desk, so:

- the label is **text**, escaped by the desk — an app never writes HTML and never writes an `esc()`;
- `icon` (a name from the desk's icon set, listed under "Icons" in `report-api`) renders the button icon-only, with `label` as its tooltip
  and accessible name; without `icon` the label is the button's text;
- the field's own `hidden` and `dependsOn` decide whether the button is on screen, and a field the
  reader cannot see carries no button;
- `key` is the button's identity within the field. A second `addFieldButton` with the same key
  **replaces** it, which is what lets a label carrying a count be refreshed after each lookup;
  omit it and the field holds one button. `removeFieldButton(field, key)` removes that one,
  `removeFieldButton(field)` removes every button on the field.

Field buttons are cleared by the same `clearButtons()` that empties the toolbar at the start of
every `refresh` — declare in `refresh` whatever must survive a save or a reload.

### Grids: row changes and cell clicks

`onChange` on a **Table** is keyed by the table's fieldname, not the child field, and fires for a
change in any of its rows — an edit in the grid, the row dialog, or `setRowValue`:

```ts
onChange: {
  attendance(frm, cdt, cdn, row, changed) {
    // cdt: the child DocType ("Training Class Attendance")
    // cdn: the row's id — undefined for a row not saved yet
    // row: the row itself, the object in frm.doc.attendance
    // changed: the child fields that changed — ["grade"] after an edit in its cell
    if (row) frm.setValue("present", frm.doc.attendance.filter((r) => r.in_class).length);
  },
},
```

Adding or removing a row fires it too, with no `cdt`, `cdn`, `row` or `changed`. An edit in a
cell changes one field; the row dialog and `setRowValue` pass every field whose value they
changed (possibly none, for a dialog applied as it was).

When what to do depends on *which* field of the row changed, give that field its own handler in
`grids.<table>.onChange.<child field>(frm, row)`. It runs for each changed field, before the
table's `onChange`, so a total computed there sees the row as the field handlers left it:

```ts
defineForm<TrainingClass>("Training Class", {
  grids: {
    attendance: {
      onChange: {
        in_class(frm, row) {
          if (!row.in_class) frm.setRowValue("attendance", row, { attended: 0, grade: null });
        },
        attended(frm, row) {
          if (row.attended && !row.in_class) frm.setRowValue("attendance", row, "in_class", 1);
        },
        grade(frm, row) {
          if (row.grade != null && !row.in_class) frm.setRowValue("attendance", row, { in_class: 1, attended: 1 });
        },
      },
    },
  },
});
```

A `setRowValue` inside a handler fires the handlers of the fields it sets, so set a field only
when it must change — as above — or two handlers can undo each other forever. `setRowValue`
leaves a field already holding the value alone and fires nothing for it.

`frm.setRowValue(table, row, field, value)` — or `(table, row, { field: value, … })` — changes a
row so that everything follows as if the reader had edited it: the grid shows the value, the form
turns dirty, the handlers of the fields it changed run and the table's `onChange` fires once. `row` is the row object or its `id`; a row
that is not in the table throws. Assigning to `frm.doc.<table>[i].<field>` directly also updates
the grid, but fires no `onChange`.

`grids.<table>.onCellClick.<child field>` makes a cell act on a click. The cell renders as a
button showing its usual value, and the handler receives the row clicked — the row itself, even
when the grid is sorted or filtered:

```ts
defineForm<TrainingClass>("Training Class", {
  grids: {
    attendance: {
      onCellClick: {
        employee(frm, row) {
          if (frm.docstatus === 0) frm.setRowValue("attendance", row, "in_class", row.in_class ? 0 : 1);
        },
      },
    },
  },
});
```

Only cells shown as text take the click: a read-only column, or every cell of a
`gridEditMode: "dialog"` grid. An editable cell keeps its control. The handler runs on a
submitted or read-only form as well, so check before changing anything, as above. Handlers from
several scripts for the same cell all run.

A `Report` field takes `onCellClick` too, keyed by the report's column `fieldname`. Every
non-empty cell of that column becomes a button (a Link cell no longer navigates); an empty cell
takes no click. The handler receives the row as the report returned it. A handler may be async,
and an error it throws or rejects with is shown. `frm.call(..., { reload: true })` or
`frm.refreshField(field)` re-runs the report:

```ts
defineForm<Charge>("Charge", {
  grids: {
    transactions: {                       // a Report field; its report returns an `action` column
      onCellClick: {
        async action(frm, row) {
          if (!(await ddcore.ui.confirm(__("{0} {1}?", [row.action, row.id])))) return;
          await frm.call("rowAction", { id: row.id }, { reload: true });
        },
      },
    },
  },
});
```

## `ddcore` in the desk

- `ddcore.call("app.services.file.fn", args)` — a whitelisted function
- `ddcore.report(name, filters)` — runs a `defineReport`: `{ meta, result: { columns, rows } }`
- `ddcore.db.getValue/getList/count/getDoc/setValue/insert` (asynchronous: `await` them).
  `getList(doctype, { filters, fields, orderBy, limit, start })` returns the rows the user may
  read. Without `limit` it returns a page of **20**, unlike the server's `getList`, which returns
  every row; `limit: 0` returns every matching row, with no cap, and a negative limit is refused.
  A bulk action that acts on "all of them" passes `limit: 0`.
- `ddcore.ui.Dialog({ title, fields, values, primaryLabel, primaryAction(values, dlg), dangerLabel, dangerAction(values, dlg), dangerShortcut, onChange(field, values, dlg), size })` → `dlg.show()/hide()/setValue/getValue/setHtml(htmlField, html)/setDfProperty(field, property, value)`
- A dialog's `fields` are DocType fields plus one type that exists only there, **`File`**: a file picker whose value is the file itself, `{ name, size, type, base64 }` (or `null`), handed to the script. **Nothing is uploaded and no `File` document is created**, which is what a credential needs; a file to keep is an `Attach`. `options` is the accept list (`".pfx,.p12"`) and `maxBytes` the size cap, 5 MB when left out: a larger file is refused before it is read. A DocType cannot declare it.
  ```ts
  ddcore.ui.Dialog({
    title: __("Replace Certificate"),
    fields: [
      { fieldname: "file", fieldtype: "File", label: "Certificate (.pfx)", options: ".pfx,.p12", reqd: true },
      { fieldname: "password", fieldtype: "Password", label: "Password" },
    ],
    async primaryAction(values, dlg) {
      await frm.call("replaceCertificate", { pfx: values.file.base64, password: values.password });
      dlg.hide();
    },
  }).show();
  ```
  The server method checks it with `ddcore.crypto.pfxInfo` before storing it in the vault. See `controller-api`.
- `ddcore.ui.msgprint(msg, { title, indicator })`, `ddcore.ui.toast`, `ddcore.ui.confirm(msg, title?, { destructive? })` (with `destructive: true` the confirm button is red and "No" is the primary, so Enter keeps the data and `Delete` — ⌫ on a Mac — confirms), `ddcore.ui.prompt(title, fields)`, `ddcore.ui.showError(e)`
- `ddcore.session` → `{ user, fullName, roles, lang }`, the signed-in user as the desk loaded it,
  and `ddcore.hasRole(role)`. They mirror the server's `ddcore.session` and decide what the desk
  *shows*: a button for the roles the server will accept. They grant nothing, so the method the
  button calls still checks on the server.
  ```ts
  defineForm("Portal Payment", {
    refresh(frm) {
      if (ddcore.hasRole("Portal Manager")) frm.addButton(__("Resolve Manually"), () => frm.call("resolveManually"));
    },
  });
  ```
- `ddcore.format.currency/date/number/value/statusColor`, `ddcore.datetime.today/addMonths/addDays/dateDiff/monthDiff/monthStart/monthEnd`
- `ddcore.search.global(txt, limit?)` — the documents the global search palette lists. See `search`
- `ddcore.realtime.on(event, handler)` → an `off()` function; `ddcore.realtime.off(event, handler)`. Events sent with
  `ddcore.publish` (see `controller-api`), outside a form — in a form, `frm.onRealtime` drops the handler when the form closes
- `ddcore.route(path)` — navigates the Desk to a path; `ddcore.setRoute(...parts)` — the same from
  path segments under the current workspace: the first is a name and goes in as its route name,
  the rest are encoded as they are. See "Navigation" below
- `__("text", [args])` — translation; the key is its English text. See `i18n`.

### Navigation

An app passes the **short** route and never needs to know which workspace a DocType lives in:

| Route | Opens |
| --- | --- |
| `/app/<DocType>` | the list (the form, for a Single) |
| `/app/<DocType>/<id>` | the document |
| `/app/<DocType>/new` | a new document |

The Desk redirects each of them to `/app/<Workspace>/<DocType>…`, the workspace that owns the
DocType, and the query string and the hash travel with the redirect.

A DocType, a workspace or a report goes into the path as its **route name**: the name without
its spaces, case kept — `Training Class` is `/app/TrainingClass`, the report `Open Tasks` is
`/app/<Workspace>/report/OpenTasks`. The name as it is (`/app/Training%20Class`) still opens and
is redirected to the route name, so a link written before this rule keeps working. A record id is
never changed: it goes in whole, percent-encoded. `/api` paths take the real name, spaces included.

- **Prefilling a new document** — `/app/<DocType>/new?field=value` sets every query key that names
  a field of the DocType on the new document; any other key is ignored. A button on a Course that
  opens a Training Class for it:

  ```ts
  frm.addButton(__("New Class"), () =>
    ddcore.route(`/app/TrainingClass/new?course=${encodeURIComponent(frm.doc.id)}`));
  ```

- **Opening a filtered list** — `/app/<DocType>?field=value` opens the list with that filter
  applied, the same query string the list writes as the user filters. It is what a workspace
  card's `route:` takes (see `report-api`).

Encode every value and every record id with `encodeURIComponent`.

### Dates and times

`ddcore.datetime` works in **civil dates** (`"YYYY-MM-DD"`) with exactly the semantics of
`ddcore.utils` on the server: `today()` is the day in the **site's** timezone — the same day the
server calls today, which is what makes `due_date < today()` agree on both sides — and
`addMonths` clamps the day to the end of the target month: `addMonths("2026-01-31", 1) === "2026-02-28"`.

`dateDiff(a, b)` is the whole days from `b` to `a` and `monthDiff(a, b)` the calendar months, the
day ignored — `dateDiff("2026-05-10", "2026-05-01") === 9`, `monthDiff("2026-02-01", "2026-01-31") === 1`
— so an `onChange` can show what the controller's `validate` is about to compute:

```ts
end_date(frm) {
  frm.setValue("total_days", ddcore.datetime.dateDiff(frm.doc.end_date, frm.doc.start_date) + 1);
}
```

`Datetime` fields, by contrast, are **instants**: they travel as ISO UTC and the control shows and
accepts them in the site's timezone. Never slice the string (`v.slice(0, 16)`) to fill a
`datetime-local` — that shows UTC as if it were local time; use `$lib/datetime`
(`toDatetimeLocal`/`fromDatetimeLocal`).

Never format a date, a number or a currency by hand: `ddcore.format.*` derives the order, the
separators and the symbol from the reader's language and the site's currency.

## `defineListView`

The list's title comes from the DocType's `label`, with its `description` under it when there
is one; the **New** button follows the reader's `create` permission and the DocType's
`allowCreate` (see "DocType properties" in `fieldtypes`).

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
  filtersCollapsed: false,              // opens the filter card by default (it starts hidden)
  modifiedColumn: false,                // hides the trailing "Modified" column
  idColumn: false,                      // hides (or `true` shows) the leading document-id column
  plainLinks: ["contract"],             // Link columns shown as the title in text, not as a link
});
```

Every key is optional. `formatters` returns **text** (not HTML); `indicator` replaces the default
status column and may return `null` to show nothing on that row. `docstatusFilter: false` suits a
submittable DocType whose `status` field already separates draft, submitted and cancelled — the
docstatus filter would only repeat it.

The filter card (search, document status, the `inStandardFilter` fields, "Clear filters") sits
behind a **Filters** button in the list's header and starts hidden, so a Calendar, Kanban or Gantt
view keeps the screen. The button counts the filters in force — `filters` defaults and a day
picked on the calendar included — so a hidden filter still shows. Each user's choice is kept per
DocType in the browser; `filtersCollapsed: false` only changes where a user who has not chosen
yet starts.

Left out, `idColumn` follows the id. The list hides the column on its own when the id is a hash
(`idGeneration: { hash: true }`, or no rule at all), unless the list has no other column, and when
the title field is a column and is the id (`idGeneration: { field }` equal to `titleField`).
`idColumn: true` brings a hash back; `idColumn: false` hides an id that means nothing to a reader
for another reason, such as a code already shown in another column. With the column hidden, the
row stays clickable and the title field's cell links to the document. To relabel the
column instead of hiding it, set `idLabel` on the DocType (see `fieldtypes`).

A Link column links to the document it names, so a click on it leaves the row. When the rows are
easily mistaken for what they link to — the classes of a course, each showing its course —
`plainLinks: ["course"]` shows that column as the course's title in plain text, and a click on it
opens the class like the rest of the row. Only Link and Dynamic Link columns are affected.

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

### Actions on the selected rows

`actions` adds buttons over the rows a user ticks in the List or Cards view. They show next to
"Delete (n)" while at least one selected row applies:

```ts
defineListView<MessageTemplate>("Message Template", {
  fields: ["approval_status"],          // what `condition` reads, when it is not a column
  actions: [{
    label: __("Submit for approval"),
    condition: (row) => row.approval_status !== "Submitted",
    async onClick(ids, list) {
      await ddcore.call("oblata.services.whatsapp_templates.submitTemplates", { ids });
      ddcore.ui.toast(__("{0} submitted", [ids.length]), { indicator: "green" });
    },
  }],
});
```

The button reads `label (n)`, where `n` counts the selected rows that pass `condition` (every
selected row when there is none), and it hides while no row passes. Only those rows reach
`onClick`: their `id`s first, then `list` with `rows` (the loaded values, in list order),
`doctype` and `refresh()`. A `condition` that throws counts as `false`. `primary: true` draws the
button as primary.

While `onClick` runs, the list's action buttons are disabled; a rejection is shown to the user.
Either way the selection is cleared and the list reloads, so `onClick` needs no `refresh()` of its
own. The rows are the ones on screen, loaded with the list's columns and `fields` — fetch anything
else by id. The button is only a shortcut: the method it calls must check permissions itself.

### Actions on the whole list

`toolbarActions` adds buttons to the list toolbar that need no selection — "Pay out all", "Sync
now". They show in every view, before the "Delete (n)" and Filters buttons:

```ts
defineListView("Portal Store", {
  toolbarActions: [{
    label: __("Pay out all"),
    condition: () => ddcore.hasRole("Portal Manager"),
    async onClick(list) {
      const n = await ddcore.call("portal.services.payouts.payOutAll", { filters: list.filters });
      ddcore.ui.toast(__("{0} paid out", [n]), { indicator: "green" });
    },
  }],
});
```

`onClick` gets `list` with `doctype`, `filters` (the `[field, op, value]` triples the current
view loads with, the search box left out) and `refresh()`. To act on every row those filters
match, fetch them with `limit: 0` — `ddcore.db.getList(list.doctype, { filters: list.filters,
fields: ["id"], limit: 0 })` — or pass `filters` to a server method that reads them itself; the
default page of 20 would act on part of the rows. `condition` takes no row: it decides
whether the button shows when the list renders, and one that throws hides it. `primary: true`
draws the button as primary. While `onClick` runs the list's action buttons are disabled, a
rejection is shown to the user, and the list reloads afterwards, keeping the selection. As with
`actions`, the method it calls must check permissions itself.

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

- **Calendar:** clicking a day lists the records the grid draws on it (a `Datetime` field matches
  the whole day in the site's time zone): those whose `field` falls on the day and, with
  `endField`, those that started earlier and end on it or later. The day stays a filter of its
  own, `?calendar_day=YYYY-MM-DD`, shown as a removable **Day** in the filter card. The **+** in a day's corner,
  shown on hover, opens a new record with the field set to that day. A calendar on `creation` or
  `modified` has the **+** only. With `endField` (a Date or Datetime), each record is drawn as a
  bar across every day from `field` to `endField`, and the bar wraps at the end of each week. A
  record keeps its row within the week, and it shows on the calendar from the first day to the last
  of the span, including one that started before the month shown. A record with no end (or an
  end before its start) takes its start day alone.

  `calendar.newOptions` points the day's **+** elsewhere — for a calendar that gathers other
  DocTypes' records into a read-only or virtual DocType, whose own **+** would never show. Each
  option names the `doctype` to create, its Date or Datetime `field` set to the day, an optional
  `endField` set to the same day (a Datetime one to its last minute) and a `label` (shown as
  given, so wrap it in `__()`; the DocType's label when omitted). An option shows only when the user may create its
  DocType; one left links straight to its form, several open a menu, and with none the **+** is
  hidden. The list's own DocType can be one of them.

  ```ts
  calendar: {
    field: "start_date", endField: "end_date",
    newOptions: [
      { label: __("Campaign"), doctype: "Campaign", field: "start_date", endField: "end_date" },
      { label: __("Diary"), doctype: "Marketing Log", field: "log_date" },
    ],
  },
  ```
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
