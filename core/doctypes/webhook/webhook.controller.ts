import { defineController } from "@ddcore/sdk";

export default defineController("Webhook", {
  validate(doc) {
    doc.url = String(doc.url || "").trim();
    doc.custom_event = doc.custom_event ? String(doc.custom_event).trim() : doc.custom_event;
    // The rules live in Go, next to the delivery path they protect.
    (ddcore as any).__webhooks.validate(doc);
  },
});
