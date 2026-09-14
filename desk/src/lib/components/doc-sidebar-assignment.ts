/** Pure helper functions for the document sidebar assignment widget. */

export function isAssignmentOverdue(dateStr?: string, status?: string, todayStr?: string): boolean {
  if (!dateStr || status !== "Open") return false;
  const now = todayStr || new Date().toISOString().slice(0, 10);
  return dateStr < now;
}

export function assignmentInitial(user: string): string {
  return (user || "U").trim().charAt(0).toUpperCase();
}

export function priorityBadgeClass(priority: string): string {
  switch ((priority || "").toLowerCase()) {
    case "urgent":
      return "priority-urgent";
    case "high":
      return "priority-high";
    case "medium":
      return "priority-medium";
    default:
      return "priority-low";
  }
}
