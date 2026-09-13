import { defineDoctype } from "@ddcore/sdk";

/**
 * One subscription: where to send, which events, and the key to sign with.
 *
 * A row and not a file in an app, because every part of it belongs to the
 * deployment — staging posts to a different receiver than production, and the
 * signing key is a secret — so none of it can be versioned with the code.
 *
 * `secret` is a Vault field: encrypted at rest, never returned by a read, kept
 * out of Version. The controller refuses DocTypes the framework writes itself;
 * see internal/engine/webhooks.go for how an event becomes a delivery.
 */
export default defineDoctype({
  name: "Webhook",
  module: "Core",
  label: "Webhook",
  icon: "send",
  naming: { hash: true },
  titleField: "url",
  trackChanges: true,
  fields: [
    { fieldname: "enabled", fieldtype: "Check", label: "Enabled", default: true, inListView: true },
    { fieldname: "url", fieldtype: "Data", label: "URL", reqd: true, inListView: true,
      description: "HTTPS address that receives a signed POST for every matching event." },
    { fieldname: "event_type", fieldtype: "Select", label: "Event Type", reqd: true, inListView: true,
      options: ["Document", "Custom"], default: "Document" },

    { fieldtype: "Section Break", label: "Document Events", dependsOn: "doc.event_type == 'Document'" },
    { fieldname: "webhook_doctype", fieldtype: "Data", label: "DocType", inListView: true, searchIndex: true,
      mandatoryDependsOn: "doc.event_type == 'Document'" },
    { fieldname: "on_insert", fieldtype: "Check", label: "On Insert" },
    { fieldname: "on_update", fieldtype: "Check", label: "On Update" },
    { fieldname: "on_submit", fieldtype: "Check", label: "On Submit" },
    { fieldname: "on_cancel", fieldtype: "Check", label: "On Cancel" },
    { fieldname: "on_trash", fieldtype: "Check", label: "On Delete" },

    { fieldtype: "Section Break", label: "Custom Event", dependsOn: "doc.event_type == 'Custom'" },
    { fieldname: "custom_event", fieldtype: "Data", label: "Event Name", searchIndex: true,
      mandatoryDependsOn: "doc.event_type == 'Custom'",
      description: "The name an app passes to ddcore.webhooks.emit, e.g. shop.order_paid." },

    { fieldtype: "Section Break", label: "Delivery" },
    { fieldname: "secret", fieldtype: "Vault", label: "Signing Secret", reqd: true,
      description: "Standard Webhooks key. A whsec_ prefix marks a base64 key; anything else is used as is." },
    { fieldname: "timeout", fieldtype: "Int", label: "Timeout (seconds)", default: 10 },
    { fieldname: "max_attempts", fieldtype: "Int", label: "Max Attempts", default: 6 },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }],
});
