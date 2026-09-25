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
Icons: a subset of lucide (`building-2, users, user, notepad-text, receipt, list, bar-chart-3, layout-dashboard, shield, paperclip, message-square, house, tag, history, settings`).
`desk.home` in `ddcore.app.ts` sets the initial workspace.

In a workspace, `label`, `title`, `description` and `category` are catalogue keys and are
translated by the server; `name`, `route`, `doctype`, `report` and `icon` are identifiers and
are left alone.
