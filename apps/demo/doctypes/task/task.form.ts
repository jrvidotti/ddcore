import { defineForm, ddcore } from "@ddcore/desk-sdk";
import type { Task } from "../../.ddcore/types";

defineForm<Task>("Task", {
  setup(frm) {
    // no new task on a project that is already finished
    frm.setQuery("project", () => ({ filters: { status: ["!=", "Completed"] } }));
  },
  refresh(frm) {
    if (frm.isNew) return;
    const status = frm.doc.status || "Open";
    // the colour comes from the field's optionColors, through the framework
    const statusField = frm.meta.doctype.fields.find((f: any) => f.fieldname === "status");
    frm.addIndicator(__(status), ddcore.format.statusColor(status, statusField));

    // the state moves only through the controller's methods; the form never
    // writes a derived field
    if (status !== "In progress" && status !== "Completed") frm.addButton(__("Start"), () => transition(frm, "start"), __("Actions"));
    if (status !== "Completed") frm.addButton(__("Complete"), () => transition(frm, "complete"), __("Actions"));
    if (status === "Completed") frm.addButton(__("Reopen"), () => transition(frm, "reopen"), __("Actions"));
    frm.setInnerGroupAsPrimary(__("Actions"));
  },
});

async function transition(frm: any, method: "start" | "complete" | "reopen") {
  try {
    await frm.call(method);
    await frm.reload();
  } catch (e) {
    ddcore.ui.showError(e);
  }
}
