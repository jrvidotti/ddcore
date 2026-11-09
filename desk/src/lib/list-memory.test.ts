import { describe, it, expect, beforeEach } from "vitest";
import { rememberListHref, lastListHref, forgetListHrefs } from "./list-memory";

describe("list-memory", () => {
  beforeEach(() => forgetListHrefs());
  it("keeps each DocType's last list URL", () => {
    expect(lastListHref("Task")).toBeUndefined();
    rememberListHref("Task", "/app/Projects/Task?status=Open");
    rememberListHref("Project", "/app/Projects/Project");
    rememberListHref("Task", "/app/Projects/Task?page=3");
    expect(lastListHref("Task")).toBe("/app/Projects/Task?page=3");
    expect(lastListHref("Project")).toBe("/app/Projects/Project");
  });
});
