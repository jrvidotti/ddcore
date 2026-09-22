import { defineForm, ddcore } from "@ddcore/desk-sdk";

// Replay is offered only once a delivery has finished: the server refuses a
// delivery still on its way, and a button that always fails is noise.
defineForm("Webhook Delivery", {
  refresh(frm) {
    if (frm.isNew || !["Sent", "Failed"].includes(frm.doc.status)) return;
    frm.addButton(__("Replay"), async () => {
      const ok = await ddcore.ui.confirm(__("Send this event to the receiver again, with the same webhook-id?"));
      if (!ok) return;
      try {
        await ddcore.call("core.services.webhooks.replay", { delivery: frm.doc.id });
        ddcore.ui.toast(__("Delivery queued again"));
        await frm.reload();
      } catch (e) {
        ddcore.ui.showError(e);
      }
    });
  },
});
