import { defineListView } from "@ddcore/desk-sdk";
import type { Task } from "../.ddcore/types";

// No `indicator` here on purpose: with `optionColors` declared on the Task's
// status field, the desk already colours the status and translates its label.
defineListView<Task>("Task", {
  columns: ["project", "title", "assignee", "priority", "status", "due_date"],
  orderBy: "due_date asc",
  views: ["list", "calendar", "cards"],
  calendar: {
    field: "due_date",
    endField: "completed_at",
    titleField: "title",
    colorField: "status",
  },
  card: {
    title: "title",
    subtitle: "project",
    dateField: "due_date",
  },
});
