import { defineListView } from "@ddcore/desk-sdk";
import type { Task } from "../.ddcore/types";

// No `indicator` here on purpose: with `optionColors` declared on the Task's
// status field, the desk already colours the status and translates its label.
defineListView<Task>("Task", {
  columns: ["project", "title", "assignee", "priority", "status", "due_date"],
  orderBy: "due_date asc",
});
