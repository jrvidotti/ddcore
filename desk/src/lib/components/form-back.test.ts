import { describe, it, expect } from "vitest";
import { formBackTarget } from "./form-back";

const base = { isSingle: false, basePath: "/app/Projects/Task", wsPrefix: "/app/Projects" };

describe("formBackTarget", () => {
  it("goes to the list by default", () => {
    expect(formBackTarget(base)).toEqual({ href: "/app/Projects/Task" });
  });
  it("goes back to the list as it was left, filters and page in the query", () => {
    expect(formBackTarget({ ...base, remembered: "/app/Projects/Task?status=Open&page=2" }))
      .toEqual({ href: "/app/Projects/Task?status=Open&page=2" });
  });
  it("ignores a remembered list under another path", () => {
    expect(formBackTarget({ ...base, remembered: "/app/Other/Task?status=Open" })).toEqual({ href: "/app/Projects/Task" });
    expect(formBackTarget({ ...base, remembered: "/app/Projects/TaskType" })).toEqual({ href: "/app/Projects/Task" });
  });
  it("sends a Single to its workspace", () => {
    expect(formBackTarget({ ...base, isSingle: true, basePath: "/app/Projects/Settings" })).toEqual({ href: "/app/Projects" });
    expect(formBackTarget({ ...base, isSingle: true, wsPrefix: "/app" })).toEqual({ href: "/app" });
  });
  it("follows the script's route and label", () => {
    expect(formBackTarget({ ...base, isSingle: true, override: { path: "/app/OTBPlan/P%201", label: "Back to Plan" } }))
      .toEqual({ href: "/app/OTBPlan/P%201", label: "Back to Plan" });
    expect(formBackTarget({ ...base, remembered: "/app/Projects/Task?x=1", override: { path: "/app/Home" } }))
      .toEqual({ href: "/app/Home", label: undefined });
  });
  it("is hidden when the script sets null", () => {
    expect(formBackTarget({ ...base, override: { path: null } })).toBeNull();
  });
});
