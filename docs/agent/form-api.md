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
`setDfProperty(field, prop, value)` (`hidden`, `readOnly`, `reqd`, `label`, `options`, `width`, `cannotAddRows`, `cannotDeleteRows`),
`setQuery(field, () => ({ filters }))`, `toggleDisplay/toggleReqd/toggleEnable`, `addButton(label, fn, group)`, `removeButton`,
`setPrimaryAction(label, fn)`, `setInnerGroupAsPrimary(group)`, `addIndicator(label, colour)`, `addChild(table, values)`, `removeChild(table, idx)`,
`addFieldButton(field, { label, icon, onClick, key })`, `removeFieldButton(field, key?)`,
`trigger(field)`, `save()`, `submit()`, `cancel()`, `reload()`, `discardChanges()`,
`call(method, args, { reload })` → calls the controller's `methods.<method>` and reloads the doc.

### Field width and layout

A form line is four slots (quarters) wide. `width?: "sm" | "md" | "lg" | "full"` in metadata — or
`frm.setDfProperty(field, "width", value)` at runtime — says how many of them the control takes, so
a control keeps the same size whether or not its section was split with a `Column Break`:

| width | section with one column | section with a `Column Break` |
| --- | --- | --- |
| `sm`, `md` | 25% of the line | 50% of the column (25% of the line) |
| `lg` | 50% of the line | 100% of the column (50% of the line) |
| `full` | 100% of the line | 100% of the column |

- **Defaults by fieldtype:**
  - `sm`: `Date`, `Month`, `Time`, `Int`, `Percent`.
  - `md`: `Datetime`, `Float`, `Currency`.
  - `full`: `Text`, `Small Text`, `Text Editor`, `JSON`, `Table`, `HTML`.
  - `lg`: every other type (`Data`, `Link`, `Select`, `Check`, `Attach`, `Password`, etc.).
- **Line packing:**
  Fields fill the line greedily, so `[Status 1/2][Start 1/4][End 1/4]` share one line. A field of
  `s` slots only starts at a multiple of `s`, so a half-line field never begins in the middle of a
  quarter: `[1/4][1/2]` renders as `[1/4][empty 1/4][1/2]`, and a half-line field that no longer
  fits moves to the next line.
- **Dialogs** keep the two-slot layout of a column (`sm`/`md` at 50%, everything else at 100%),
  because a modal is too narrow for quarter-line controls.
- **Responsiveness:**
  Below `800px`, every cell collapses to 100% full width.

### Unsaved changes

The desk keeps what the reader typed and did not save in the browser, one draft per user and
record, for seven days. Leaving the form asks first; coming back to the record puts the draft
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
- `ddcore.db.getValue/getList/count/getDoc/setValue/insert` (asynchronous: `await` them)
- `ddcore.ui.Dialog({ title, fields, values, primaryLabel, primaryAction(values, dlg), dangerLabel, dangerAction(values, dlg), onChange(field, values, dlg), size })` → `dlg.show()/hide()/setValue/getValue/setHtml(htmlField, html)`
- `ddcore.ui.msgprint(msg, { title, indicator })`, `ddcore.ui.toast`, `ddcore.ui.confirm(msg)`, `ddcore.ui.prompt(title, fields)`, `ddcore.ui.showError(e)`
- `ddcore.format.currency/date/number/value/statusColor`, `ddcore.datetime.today/addMonths/addDays/monthStart/monthEnd`
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
});
```

Every key is optional. `formatters` returns **text** (not HTML); `indicator` replaces the default
status column and may return `null` to show nothing on that row.

Most lists need no `indicator` at all: declare `optionColors` on the status field and the desk
colours and translates it on its own. Reach for `indicator` only when the label is not a field
value — and never key a colour on text a reader sees, since that changes with the language.

A DocType may have **several** form scripts: its owner's `<snake>.form.ts`, plus one per app extending it
(`extensions/<snake>.form.ts` — see `extending`). Their handlers accumulate, in app load order; every `refresh`,
`validate` and `onChange` runs. `defineListView` is the exception: one per DocType, and the last registration wins.

Global scripts (`client/*.ts`, listed under `desk.include` in `ddcore.app.ts`) run across the whole desk: masks, shortcuts, `defineForm` for several DocTypes.
Date field inputs carry `data-fieldname` and `data-fieldtype` for masks applied by event delegation.
