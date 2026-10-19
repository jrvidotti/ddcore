/** Pure helper functions for the document sidebar assignment widget. */

/** `todayStr` is the site's today (`datetime.today()`), the day the server counts a due date against. */
export function isAssignmentOverdue(dateStr: string | undefined, status: string | undefined, todayStr: string): boolean {
  if (!dateStr || status !== "Open") return false;
  return dateStr < todayStr;
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
