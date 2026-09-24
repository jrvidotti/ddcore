import { defineForm, ddcore } from "@ddcore/desk-sdk";

// A task's status moves through the assignment endpoints, which check who may
// act and note the change on the referenced document; the field is read-only
// and the header offers the moves the current status allows.
// (to_do.form.ts: a form script is found by the snake case of the DocType's
// name, and "ToDo" gives "to_do".)
defineForm("ToDo", {
  refresh(frm) {
    if (frm.isNew) return;
    const run = (move: (id: string) => Promise<unknown>, done: string) => async () => {
      if (frm.isDirty) {
        ddcore.ui.toast(__("Save the document first"), { indicator: "orange" });
        return;
      }
      try {
        await move(frm.doc.id);
        ddcore.ui.toast(done);
        await frm.reload();
      } catch (e) {
        ddcore.ui.showError(e);
      }
    };
    if (frm.doc.status === "Open") {
      frm.addButton(__("Complete"), run(ddcore.assignments.complete, __("Task completed")));
      const cancel = run(ddcore.assignments.revoke, __("Task cancelled"));
      frm.addButton(__("Cancel"), async () => {
        if (await ddcore.ui.confirm(__("Cancel this task?"), __("Cancel"), { destructive: true })) await cancel();
      });
    } else {
      frm.addButton(__("Reopen"), run(ddcore.assignments.reopen, __("Task reopened")));
    }
  },
});
