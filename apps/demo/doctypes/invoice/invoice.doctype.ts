import { defineDoctype } from "@ddcore/sdk";

/**
 * The example's one monetary document.
 *
 * It exists because decimal precision is a framework contract rather than an
 * app feature: the rule that a Currency is stored at the site's precision, and
 * that a schedule of instalments adds back up to its total, has to be
 * demonstrated somewhere an app author will actually read.
 */
export default defineDoctype({
  name: "Invoice",
  module: "Projects",
  label: "Invoice",
  naming: { field: "code" },
  titleField: "code",
  trackChanges: true,
  fields: [
    { fieldname: "code", fieldtype: "Data", label: "Code", reqd: true, unique: true, inListView: true },
    { fieldname: "project", fieldtype: "Link", label: "Project", options: "Project", reqd: true, inStandardFilter: true },
    { fieldname: "issue_date", fieldtype: "Date", label: "Issue date", reqd: true, default: "Today", inListView: true },
    { fieldname: "total", fieldtype: "Currency", label: "Total", reqd: true, inListView: true },
    { fieldname: "monthly_interest", fieldtype: "Percent", label: "Monthly interest", default: 0 },
    { fieldname: "installments", fieldtype: "Table", label: "Installments", options: "Invoice Installment", gridEditMode: "inline" },
  ],
  permissions: [
    { role: "Project Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "Project Contributor", read: true },
  ],
});
