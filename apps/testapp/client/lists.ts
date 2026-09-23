import { defineListView } from "@ddcore/desk-sdk";
import type { Task, TaskCategory } from "../.ddcore/types";

// No `indicator` here on purpose: with `optionColors` declared on the Task's
// status field, the desk already colours the status and translates its label.
defineListView<Task>("Task", {
  columns: ["project", "title", "assignee", "priority", "status", "due_date"],
  orderBy: "due_date asc",
  views: ["list", "calendar", "kanban", "gantt", "cards"],
  calendar: {
    field: "due_date",
    endField: "completed_at",
    titleField: "title",
    colorField: "status",
  },
  // status is set by the controller (readOnly), so the board groups by
  // priority, which a contributor may change by dragging a card
  kanban: {
    field: "priority",
    titleField: "title",
    subtitleField: "project",
    colorField: "status",
  },
  gantt: {
    startField: "creation",
    endField: "due_date",
    titleField: "title",
    colorField: "status",
  },
  card: {
    title: "title",
    subtitle: "project",
    dateField: "due_date",
  },
});

// The tree reads "DLV - Delivery" and sorts on the title alone, groups and
// leaves together.
defineListView<TaskCategory>("Task Category", {
  tree: { title: "{acronym} - {title}", orderBy: "title asc" },
});
