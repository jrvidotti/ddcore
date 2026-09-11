import { defineDoctype } from "@ddcore/sdk";

/**
 * One outgoing message, and what became of it.
 *
 * Written by the framework, never by an app: there is no `create` permission
 * below, and `ddcore.sendMail` inserts the record with permissions ignored. A
 * System Manager reads it to answer "did the customer get it?", and deletes it
 * to prune.
 *
 * The rendered body is deliberately absent. A message is re-rendered from
 * `template` and `args` whenever it is needed, which keeps this table small and
 * keeps a recovery link — a live credential — out of it. A template that mails
 * a credential declares `sensitive`, and then not even `args` is stored.
 */
export default defineDoctype({
  name: "Email Delivery",
  module: "Core",
  label: "Email Delivery",
  icon: "mail",
  titleField: "subject",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "to", fieldtype: "Data", label: "To", reqd: true, inListView: true, searchIndex: true },
    { fieldname: "subject", fieldtype: "Data", label: "Subject", inListView: true },
    { fieldname: "status", fieldtype: "Select", label: "Status", reqd: true, inListView: true, searchIndex: true,
      options: ["Queued", "Sent", "Failed", "Uncertain"] },
    { fieldname: "template", fieldtype: "Data", label: "Template", reqd: true, inListView: true },
    { fieldname: "lang", fieldtype: "Data", label: "Language" },
    { fieldname: "sent_at", fieldtype: "Datetime", label: "Sent at", inListView: true },
    { fieldname: "attempts", fieldtype: "Int", label: "Attempts" },

    { fieldtype: "Section Break", label: "Origin" },
    // Two plain columns rather than a Dynamic Link, matching File, Comment and
    // Version: the pair is swept by name on rename, and deliberately *not* on
    // delete — see coreRefs in internal/engine/rename.go.
    { fieldname: "reference_doctype", fieldtype: "Data", label: "Reference DocType", searchIndex: true },
    { fieldname: "reference_name", fieldtype: "Data", label: "Reference Name", searchIndex: true },
    { fieldname: "key", fieldtype: "Data", label: "Idempotency Key", unique: true },
    { fieldname: "job", fieldtype: "Int", label: "Job" },

    { fieldtype: "Section Break", label: "Content" },
    { fieldname: "args", fieldtype: "JSON", label: "Arguments" },
    { fieldname: "attachments", fieldtype: "JSON", label: "Attachments" },
    { fieldname: "error", fieldtype: "Text", label: "Error" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, delete: true }],
});
