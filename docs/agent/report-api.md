# Reports, workspaces, cards and charts

```ts
import { defineReport, _ } from "@ddcore/sdk";
export default defineReport({
  name: "Contracts Due", refDoctype: "Contract", roles: ["Manager"],
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
A `Report` field shows a report inside a form, its filters taken from the document
(`reportFilters: { course: "id" }`); see `fieldtypes`, "Form grids".

A report's `name`, its column labels and its chart labels are all human-facing: `label:` in the
definition is a catalogue key the server translates (a report's `label` defaults to its `name`,
which is then the key), and anything built inside `execute` goes
through `_()`. A status is its own key — `_(row.status)` — because a Select value is canonical
English. See `i18n`.

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
<!-- icons:begin — generated from desk/icons/icons.json by `go test ./desk/icons -update` -->
Icons: a subset of lucide, the only names the desk draws: `alert-triangle`, `banknote`, `bar-chart-3`, `bell`, `bold`, `book-open`, `briefcase`, `bug`, `building`, `building-2`, `calendar`, `chart-bar`, `chart-gantt`, `check`, `check-square`, `chevron-down`, `chevron-left`, `chevron-right`, `chevron-up`, `chevrons-down-up`, `chevrons-up-down`, `circle`, `code`, `credit-card`, `download`, `eraser`, `external-link`, `eye`, `file`, `file-text`, `filter`, `folder`, `folder-tree`, `heading-2`, `heading-3`, `history`, `home`, `house`, `image`, `inbox`, `italic`, `key`, `keyboard`, `landmark`, `layout-dashboard`, `layout-grid`, `lightbulb`, `link`, `list`, `list-ordered`, `list-tree`, `locate-fixed`, `lock`, `log-out`, `mail`, `map-pin`, `maximize-2`, `menu`, `message-square`, `mic`, `minimize-2`, `more-horizontal`, `notepad-text`, `panel-left`, `panel-right`, `paperclip`, `pen`, `pencil`, `pentagon`, `percent`, `pipette`, `plus`, `printer`, `qr-code`, `quote`, `receipt`, `refresh-cw`, `rotate-ccw`, `scan-line`, `search`, `send`, `settings`, `share-2`, `shield`, `spline`, `square`, `square-kanban`, `strikethrough`, `tag`, `trash`, `trending-up`, `undo-2`, `upload`, `user`, `user-plus`, `users`, `wallet`, `wrench`, `x`.
Other lucide names for some of them work too: `chart-column` (`bar-chart-3`), `edit-2` (`pen`), `ellipsis` (`more-horizontal`), `square-check-big` (`check-square`), `triangle-alert` (`alert-triangle`).
Any other name in a workspace, a sidebar item, a shortcut or a DocType's `icon` fails the load.
<!-- icons:end -->
`desk.home` in `ddcore.app.ts` sets the initial workspace.

On a site with tenancy, `space: "platform"` or `space: "tenant"` on the workspace itself, or on
one of its `sidebar`, `shortcuts` or `numberCards` entries, shows it only in that space: the
platform space (where the operator manages tenants and what they share) or inside a tenant.
Without `space` it shows in both; without tenancy it is ignored. Boot leaves out what belongs to
the other space, and its number cards answer 404 there. Any other value fails the load. See
`tenancy`.

```ts
export default defineWorkspace({
  name: "Back Office", label: "Back Office",
  sidebar: [
    { label: "Tenants", doctype: "Site Tenant", space: "platform" },
    { label: "Charges", doctype: "Charge", space: "tenant" },
    { label: "Banks", doctype: "Bank" }, // a shared reference table: both spaces read it
  ],
});
```

In a workspace, `label`, `title`, `description` and `category` are catalogue keys and are
translated by the server; `name`, `route`, `doctype`, `report` and `icon` are identifiers and
are left alone.
