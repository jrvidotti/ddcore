// The portal the acceptance suite signs in to: a Website User reads the tasks
// assigned to them, and nothing else. The identity is the User row itself, so
// the fixture needs no link DocType of its own.
import { definePortal } from "@ddcore/sdk";

export default definePortal({
  name: "Assignee",
  title: "Task Portal",
  roles: ["Project Contributor"],
  identity: { doctype: "User", userField: "id" },
  pages: [
    {
      name: "tasks",
      label: "My tasks",
      doctype: "Task",
      match: { assignee: "id" },
      fields: ["code", "title", "status", "due_date"],
    },
  ],
});
