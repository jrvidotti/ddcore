import { defineDoctype } from "@ddcore/sdk";

/**
 * One event on its way to one receiver, and what became of it.
 *
 * Written by the framework, never by an app: no role has `create`. The name is
 * the `webhook-id` header, the same on every attempt and on a replay, which is
 * what lets a receiver drop a duplicate. `payload` is the exact body, frozen
 * when the event happened, so a retry an hour later still describes the
 * document as it was at the time rather than as it is now.
 */
export default defineDoctype({
  name: "Webhook Delivery",
  module: "Core",
  label: "Webhook Delivery",
  icon: "send",
  naming: { hash: true },
  titleField: "event",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "webhook", fieldtype: "Link", label: "Webhook", options: "Webhook", reqd: true, inListView: true, searchIndex: true },
    { fieldname: "event", fieldtype: "Data", label: "Event", reqd: true, inListView: true, searchIndex: true },
    { fieldname: "status", fieldtype: "Select", label: "Status", reqd: true, inListView: true, searchIndex: true,
      options: ["Queued", "Retrying", "Sent", "Failed"],
      optionColors: { Queued: "gray", Retrying: "orange", Sent: "green", Failed: "red" } },
    { fieldname: "attempts", fieldtype: "Int", label: "Attempts", inListView: true },
    { fieldname: "response_status", fieldtype: "Int", label: "Response Status" },
    { fieldname: "sent_at", fieldtype: "Datetime", label: "Sent at" },

    { fieldtype: "Section Break", label: "Origin" },
    // Plain columns, swept on rename and kept on delete: see coreRefs in
    // internal/engine/rename.go.
    { fieldname: "reference_doctype", fieldtype: "Data", label: "Reference DocType", searchIndex: true },
    { fieldname: "reference_name", fieldtype: "Data", label: "Reference Name", searchIndex: true },
    { fieldname: "key", fieldtype: "Data", label: "Idempotency Key", unique: true },
    { fieldname: "job", fieldtype: "Int", label: "Job" },

    { fieldtype: "Section Break", label: "Content" },
    { fieldname: "payload", fieldtype: "JSON", label: "Payload" },
    { fieldname: "error", fieldtype: "Text", label: "Error" },
  ],
  permissions: [{ role: "System Manager", read: true, delete: true }],
});
