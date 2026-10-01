import { defineNotification, _ } from "@ddcore/sdk";

const title = (doc: any) => _("Assignment due today: {0}", [doc.description || doc.id]);
const message = (doc: any) => _("Task assigned by {0} is due today", [doc.assigned_by]);

export default defineNotification({
  name: "core.todo_due",
  doctype: "ToDo",
  date: { field: "date", days: 0 },
  condition: (doc) => doc.status === "Open",
  recipients: (doc) => [doc.allocated_to],
  desk: { title, message },
  // The assignee may turn this email off on their profile (User.mute_due_email);
  // the inbox notification is written either way.
  email: {
    template: "core.notification",
    args: (doc) => ({ title: title(doc), message: message(doc), doctype: "ToDo", id: doc.id }),
  },
});
