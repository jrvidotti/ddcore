# Reports, workspaces, cards and charts

```ts
import { defineReport, _ } from "@ddcore/sdk";
export default defineReport({
  name: "Contracts Due", description: "Contracts that end within the chosen number of days.", refDoctype: "Contract", roles: ["Manager"],
  filters: [{ fieldname: "days", label: "Days", fieldtype: "Int", default: 90, reqd: true }, { fieldname: "property", fieldtype: "Link", options: "Property", label: "Property" }],
  execute(filters, ctx) {
    const rows = ddcore.db.getList("Contract", { filters: {/* … */}, fields: [/* … */], limit: 10000 });
    return {
      columns: [{ fieldname: "id", label: _("Contract"), fieldtype: "Link", options: "Contract", width: 140 }, /* … */],
      rows,
      summary: [{ label: _("Total"), value: 10, datatype: "Currency", indicator: "red" }],
      chart: { type: "bar", labels: [/* … */], datasets: [{ name: _("Revenue"), values: [/* … */] }] },
    };
  },
});
```
Date filter defaults: `"Today"`, `"month_start"`, `"month_end"`, `"-11m"` (the first of the month, N months ago).
A summary card's `datatype` is `Currency`, `Int`, `Float`, `Data`, `Date` or `Datetime`, and the
card is formatted like a cell of that type; without one, a number is shown as a number and
anything else as text.
Route: `/app/report/<name>`; API: `GET /api/report/<name>?filters={...}`.
The report page sorts by a click on a column header and exports CSV or XLSX.
A column draws its values as indicators when it carries a Select's `optionColors` or
`optionIcons` (and `optionIconOnly`), as a field does (see `fieldtypes`); a column named
`status` is an indicator either way.
A `Report` field shows a report inside a form, its filters taken from the document
(`reportFilters: { course: "id" }`); see `fieldtypes`, "Form grids".

A report's `name`, its column labels and its chart labels are all human-facing: `label:` in the
definition is a catalogue key the server translates (a report's `label` defaults to its `name`,
which is then the key), and anything built inside `execute` goes
through `_()`. A status is its own key — `_(row.status)` — because a Select value is canonical
English. See `i18n`.

`description:` is one more line of that kind: a catalogue key, translated like `label`, drawn under
the report's title. Say what the report shows and how to read it.

```ts
import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({
  name: "Sales", label: "Sales", icon: "briefcase", roles: ["Manager"],
  sidebar: [{ label: "Overview", route: "/app/workspace/Sales", icon: "layout-dashboard" }, { label: "Contracts", doctype: "Contract", icon: "notepad-text" }, { label: "Records" /* no link = a heading */ }, { label: "Report X", report: "Report X" }],
  shortcuts: [{ label: "Contracts", doctype: "Contract", icon: "notepad-text" }],
  numberCards: [
    { name: "active", label: "Active Contracts", doctype: "Contract", filters: { status: "Active" }, color: "green", route: "/app/Contract?status=Active" },
    { name: "overdue", label: "Total Overdue", doctype: "Entry", filters: { status: "Overdue" }, aggregate: "sum:balance", color: "red" },
    { name: "month", label: "Received This Month", method() { return { value: 10, formatted: ddcore.utils.formatCurrency(10) }; } },
  ],
  charts: [{ name: "revenue", label: "Monthly Revenue", type: "bar", method() { return { type: "bar", labels: [/* … */], datasets: [{ name: _("Revenue"), values: [/* … */] }] }; } }],
  links: [{ label: "Records", items: [{ label: "People", doctype: "Person" }] }],
});
```

What guards a card and a chart:

- The workspace `roles` gate the workspace in boot and its `/api/workspace/{name}/card|chart`
  endpoints; a workspace without `roles` is open to every signed-in user.
- A `doctype` card also checks `read` on its DocType, and counts with the user's permissions.
- A card or chart `method()` runs **as the calling user with no DocType check**: the workspace
  `roles` are its only gate. `ddcore.db.getList` inside it still applies the user's permissions,
  but `ddcore.db.sql` and `getAll` do not, so a role the workspace lets in sees numbers its
  DocType permissions would hide. Declare the DocType the method reads with `refDoctype`, and
  the card or chart answers 403 to a user without read on it:
  ```ts
  numberCards: [{ name: "logins", label: "Logins", refDoctype: "Portal Login Log",
    method() { return { value: ddcore.db.sql("select count(*) n from tab_portal_login_log")[0].n }; } }],
  ```
  A method that reads several DocTypes, or decides per document, checks inside with
  `ddcore.hasPermission(doctype, "read")` (see `controller-api`) and returns nothing it may not.

<!-- icons:begin — generated from desk/icons/icons.json by `go test ./desk/icons -update` -->
Icons: a subset of lucide, the only names the desk draws: `activity`, `alarm-clock`, `alert-triangle`, `archive`, `archive-restore`, `arrow-down`, `arrow-left`, `arrow-left-right`, `arrow-right`, `arrow-up`, `arrow-up-down`, `arrow-up-right`, `at-sign`, `award`, `baby`, `badge-check`, `badge-percent`, `ban`, `banknote`, `bar-chart-3`, `barcode`, `bed`, `bell`, `bell-off`, `bell-ring`, `bike`, `bold`, `book`, `book-open`, `bookmark`, `bot`, `box`, `boxes`, `briefcase`, `brush`, `bug`, `building`, `building-2`, `bus`, `calculator`, `calendar`, `calendar-check`, `calendar-clock`, `calendar-days`, `calendar-plus`, `camera`, `car`, `chart-area`, `chart-bar`, `chart-gantt`, `chart-line`, `chart-pie`, `check`, `check-square`, `chevron-down`, `chevron-left`, `chevron-right`, `chevron-up`, `chevrons-down-up`, `chevrons-up-down`, `church`, `circle`, `circle-alert`, `circle-check`, `circle-dollar-sign`, `circle-question-mark`, `circle-stop`, `circle-x`, `clipboard`, `clipboard-check`, `clipboard-list`, `clock`, `cloud`, `cloud-download`, `cloud-upload`, `code`, `coffee`, `cog`, `coins`, `compass`, `contact`, `copy`, `cpu`, `credit-card`, `cross`, `database`, `dollar-sign`, `download`, `droplet`, `dumbbell`, `eraser`, `euro`, `external-link`, `eye`, `face-slightly-smiling`, `factory`, `file`, `file-archive`, `file-check`, `file-pen`, `file-plus`, `file-search`, `file-spreadsheet`, `file-text`, `file-x`, `files`, `filter`, `fingerprint-pattern`, `flag`, `flame`, `flower`, `folder`, `folder-open`, `folder-plus`, `folder-tree`, `gauge`, `gift`, `git-branch`, `globe`, `graduation-cap`, `hammer`, `hand-coins`, `hand-heart`, `hand-helping`, `handshake`, `hard-drive`, `heading-2`, `heading-3`, `heart`, `heart-handshake`, `heart-pulse`, `history`, `home`, `hospital`, `hotel`, `hourglass`, `house`, `id-card`, `image`, `inbox`, `info`, `italic`, `kanban`, `key`, `key-round`, `keyboard`, `landmark`, `laptop`, `layers`, `layout-dashboard`, `layout-grid`, `layout-list`, `leaf`, `library`, `lightbulb`, `link`, `link-2`, `list`, `list-checks`, `list-filter`, `list-ordered`, `list-tree`, `locate-fixed`, `lock`, `lock-open`, `log-in`, `log-out`, `mail`, `mail-open`, `map`, `map-pin`, `maximize-2`, `medal`, `megaphone`, `menu`, `message-circle`, `message-square`, `messages-square`, `mic`, `minimize-2`, `minus`, `monitor`, `moon`, `more-horizontal`, `move`, `music`, `navigation`, `network`, `newspaper`, `notebook`, `notepad-text`, `octagon-alert`, `package`, `package-check`, `package-plus`, `palette`, `panel-left`, `panel-right`, `paperclip`, `pause`, `pen`, `pencil`, `pentagon`, `percent`, `phone`, `phone-call`, `piggy-bank`, `pill`, `pipette`, `plane`, `play`, `plug`, `plus`, `power`, `printer`, `qr-code`, `quote`, `receipt`, `receipt-text`, `refresh-cw`, `repeat`, `rotate-ccw`, `rotate-cw`, `route`, `rss`, `save`, `scale`, `scan-barcode`, `scan-line`, `school`, `scissors`, `scroll-text`, `search`, `send`, `server`, `settings`, `share`, `share-2`, `sheet`, `shield`, `shield-alert`, `shield-check`, `ship`, `shirt`, `shopping-bag`, `shopping-cart`, `signature`, `sliders-horizontal`, `smartphone`, `sparkles`, `spline`, `square`, `square-kanban`, `stamp`, `star`, `stethoscope`, `sticky-note`, `store`, `strikethrough`, `sun`, `syringe`, `table`, `tag`, `target`, `terminal`, `thumbs-down`, `thumbs-up`, `ticket`, `timer`, `trash`, `tree-pine`, `trending-down`, `trending-up`, `trophy`, `truck`, `undo-2`, `unlink`, `upload`, `user`, `user-check`, `user-cog`, `user-minus`, `user-pen`, `user-plus`, `user-round`, `user-x`, `users`, `users-round`, `utensils`, `video`, `wallet`, `warehouse`, `webhook`, `wifi`, `workflow`, `wrench`, `x`, `zap`, `zoom-in`, `zoom-out`.
Other lucide names for some of them work too: `alert-circle` (`circle-alert`), `chart-column` (`bar-chart-3`), `check-circle` (`circle-check`), `circle-help` (`circle-question-mark`), `edit-2` (`pen`), `ellipsis` (`more-horizontal`), `fingerprint` (`fingerprint-pattern`), `help-circle` (`circle-question-mark`), `line-chart` (`chart-line`), `pie-chart` (`chart-pie`), `smile` (`face-slightly-smiling`), `square-check-big` (`check-square`), `triangle-alert` (`alert-triangle`), `unlock` (`lock-open`), `x-circle` (`circle-x`).
Any other name in a workspace, a sidebar item, a shortcut or a DocType's `icon` fails the load.
<!-- icons:end -->
A lucide icon the list lacks is a one-line addition to ddcore: ask for it in an issue.

`desk.home` in `ddcore.app.ts` sets the initial workspace.

On a site with tenancy, `space: "platform"` or `space: "tenant"` on the workspace itself, or on
one of its `sidebar`, `shortcuts` or `numberCards` entries, shows it only in that space: the
platform space (where the operator manages tenants and what they share) or inside a tenant.
Without `space` it shows in both, except for an entry that opens a tenant-only DocType
(`space: "tenant"` on the DocType or its app). That covers an entry whose `doctype`, a card's
`refDoctype`, or the `refDoctype` of its `report` is such a DocType. It shows inside a tenant
only, and so does a grouped `links` item. An explicit `space` on the entry still wins. Without
tenancy the field is ignored. Boot leaves out what belongs to the other space, and its number
cards answer 404 there. Any other value fails the load. A workspace with no `space` of its own
stays in both spaces even when all of its entries drop out of one, so give a business workspace
`space: "tenant"` too. See `tenancy`.

```ts
export default defineWorkspace({
  name: "Back Office", label: "Back Office",
  sidebar: [
    { label: "Tenants", doctype: "Site Tenant", space: "platform" },
    { label: "Charges", doctype: "Charge", space: "tenant" }, // or space: "tenant" on Charge itself
    { label: "Banks", doctype: "Bank" }, // a shared reference table: both spaces read it
  ],
});
```

In a workspace, `label`, `title`, `description` and `category` are catalogue keys and are
translated by the server; `name`, `route`, `doctype`, `report` and `icon` are identifiers and
are left alone.
