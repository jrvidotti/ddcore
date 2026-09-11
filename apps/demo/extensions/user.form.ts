import { defineForm, ddcore } from "@ddcore/desk-sdk";

// A form script for a DocType another app owns. Core ships no user.form.ts;
// even if it did, this one would run alongside it — handlers accumulate
// rather than replacing one another.
defineForm("User", {
  refresh(frm) {
    if (frm.isNew || !frm.doc.name) return;
    // the list reads its filters from the query string
    frm.addButton(__("Tasks"), () => ddcore.route(`/app/Task?assignee=${encodeURIComponent(frm.doc.name!)}`));
  },
});
