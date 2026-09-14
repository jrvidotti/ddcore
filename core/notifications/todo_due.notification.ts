import { defineNotification, _ } from "@ddcore/sdk";

export default defineNotification({
  name: "core.todo_due",
  doctype: "ToDo",
  date: { field: "date", days: 0 },
  condition: (doc) => doc.status === "Open",
  recipients: (doc) => [doc.allocated_to],
  desk: {
    title: (doc) => _("Assignment due today: {0}", [doc.description || doc.name]),
    message: (doc) => _("Task assigned by {0} is due today", [doc.assigned_by]),
  },
});
