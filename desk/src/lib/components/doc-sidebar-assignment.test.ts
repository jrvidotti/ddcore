import { describe, it, expect } from "vitest";
import { isAssignmentOverdue, assignmentInitial, priorityBadgeClass } from "./doc-sidebar-assignment";

describe("doc-sidebar-assignment helpers", () => {
  it("determines if an open assignment is overdue", () => {
    const today = "2026-09-14";
    expect(isAssignmentOverdue("2026-09-13", "Open", today)).toBe(true);
    expect(isAssignmentOverdue("2026-09-14", "Open", today)).toBe(false);
    expect(isAssignmentOverdue("2026-09-15", "Open", today)).toBe(false);
    // Closed or Cancelled tasks are never overdue
    expect(isAssignmentOverdue("2026-09-13", "Closed", today)).toBe(false);
    expect(isAssignmentOverdue("2026-09-13", "Cancelled", today)).toBe(false);
    // Empty dates are never overdue
    expect(isAssignmentOverdue("", "Open", today)).toBe(false);
  });

  it("extracts user initial correctly", () => {
    expect(assignmentInitial("ana@x.com")).toBe("A");
    expect(assignmentInitial(" Carlos ")).toBe("C");
    expect(assignmentInitial("")).toBe("U");
  });

  it("returns priority badge classes", () => {
    expect(priorityBadgeClass("Urgent")).toBe("priority-urgent");
    expect(priorityBadgeClass("High")).toBe("priority-high");
    expect(priorityBadgeClass("Medium")).toBe("priority-medium");
    expect(priorityBadgeClass("Low")).toBe("priority-low");
  });
});
