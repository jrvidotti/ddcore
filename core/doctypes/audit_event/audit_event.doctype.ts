import { defineDoctype } from "@ddcore/sdk";

/**
 * Who did something sensitive to what, and whether they were allowed to.
 *
 * The minimum the first operational service needed — a webhook replay sends
 * data to a third party again, and that has to be answerable later. Rows are
 * written by the framework and only read here: no role may create or change
 * one, so the record cannot be tidied by the person it describes. `detail`
 * never carries a secret or a payload.
 */
export default defineDoctype({
  name: "Audit Event",
  module: "Core",
  label: "Audit Event",
  icon: "shield",
  naming: { hash: true },
  titleField: "action",
  globalSearch: false,
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "action", fieldtype: "Data", label: "Action", reqd: true, inListView: true, searchIndex: true },
    { fieldname: "outcome", fieldtype: "Select", label: "Outcome", reqd: true, inListView: true, searchIndex: true,
      options: ["Allowed", "Denied"], optionColors: { Allowed: "green", Denied: "red" } },
    { fieldname: "actor", fieldtype: "Data", label: "Actor", inListView: true, searchIndex: true },
    { fieldname: "target_doctype", fieldtype: "Data", label: "Target DocType", inListView: true },
    { fieldname: "target_name", fieldtype: "Data", label: "Target Name", inListView: true, searchIndex: true },
    { fieldname: "ip", fieldtype: "Data", label: "IP Address" },
    { fieldname: "request_id", fieldtype: "Data", label: "Request ID", searchIndex: true },
    { fieldname: "detail", fieldtype: "JSON", label: "Detail" },
  ],
  permissions: [{ role: "System Manager", read: true }],
});
