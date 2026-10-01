import { describe, it, expect } from "vitest";
import { routeName, seg, resolveName, resolveDoctype, resolveReport, resolveWorkspace, canonicalAppPath } from "./routes";

const names = {
  workspaces: [{ name: "Human Resources" }, { name: "Projects" }],
  doctypes: { "Training Class": {}, Course: {}, "Work Item": {} },
  virtuals: { "Any Party": ["Person"] },
  reports: { "Open Tasks": {}, Sales: {} },
};

describe("route names", () => {
  it("drops the whitespace of a name and keeps its case", () => {
    expect(routeName("Training Class")).toBe("TrainingClass");
    expect(routeName("Course")).toBe("Course");
    expect(seg("Training Class")).toBe("TrainingClass");
    expect(seg("Ação Social")).toBe(encodeURIComponent("AçãoSocial"));
  });

  it("is idempotent, so a segment read from the URL can be written back", () => {
    expect(seg(seg("Training Class"))).toBe("TrainingClass");
  });

  it("resolves a segment to the real name, the exact name first", () => {
    expect(resolveName("TrainingClass", ["Course", "Training Class"])).toBe("Training Class");
    expect(resolveName("Training Class", ["Course", "Training Class"])).toBe("Training Class");
    expect(resolveName("TrainingClass", ["Training Class", "TrainingClass"])).toBe("TrainingClass");
    expect(resolveName("Nope", ["Course"])).toBeNull();
  });

  it("resolves DocTypes, virtual DocTypes and reports, and hands back what it does not know", () => {
    expect(resolveDoctype("WorkItem", names)).toBe("Work Item");
    expect(resolveDoctype("Work Item", names)).toBe("Work Item");
    expect(resolveDoctype("AnyParty", names)).toBe("Any Party");
    expect(resolveDoctype("Task Tag", names)).toBe("Task Tag");
    expect(resolveDoctype("course", names)).toBe("course"); // a DocType name is case-sensitive
    expect(resolveReport("OpenTasks", names)).toBe("Open Tasks");
    expect(resolveReport("Unknown", names)).toBe("Unknown");
    expect(resolveDoctype("X", null)).toBe("X");
  });

  it("resolves a workspace whatever its case", () => {
    expect(resolveWorkspace("HumanResources", names)).toBe("Human Resources");
    expect(resolveWorkspace("human resources", names)).toBe("Human Resources");
    expect(resolveWorkspace("humanresources", names)).toBe("Human Resources");
    expect(resolveWorkspace("Course", names)).toBeNull();
  });
});

describe("canonicalAppPath", () => {
  it("rewrites the names of an old path and leaves the rest", () => {
    expect(canonicalAppPath("/app/Human%20Resources", names)).toBe("/app/HumanResources");
    expect(canonicalAppPath("/app/Human%20Resources/Training%20Class", names)).toBe("/app/HumanResources/TrainingClass");
    expect(canonicalAppPath("/app/Projects/Work%20Item/WI%201", names)).toBe("/app/Projects/WorkItem/WI%201");
    expect(canonicalAppPath("/app/Projects/Work%20Item/new", names)).toBe("/app/Projects/WorkItem/new");
    expect(canonicalAppPath("/app/Projects/Work%20Item/WI-1/print", names)).toBe("/app/Projects/WorkItem/WI-1/print");
    expect(canonicalAppPath("/app/Projects/report/Open%20Tasks", names)).toBe("/app/Projects/report/OpenTasks");
    expect(canonicalAppPath("/app/report/Open%20Tasks", names)).toBe("/app/report/OpenTasks");
    expect(canonicalAppPath("/app/workspace/Human%20Resources", names)).toBe("/app/workspace/HumanResources");
    expect(canonicalAppPath("/app/projects/Course", names)).toBe("/app/Projects/Course");
  });

  it("answers null for a path that is already canonical", () => {
    expect(canonicalAppPath("/app/HumanResources/TrainingClass/TC%201", names)).toBeNull();
    expect(canonicalAppPath("/app/Projects/Course/a%20b", names)).toBeNull();
    expect(canonicalAppPath("/app/Projects/report/OpenTasks", names)).toBeNull();
    expect(canonicalAppPath("/app", names)).toBeNull();
    expect(canonicalAppPath("/login", names)).toBeNull();
  });

  it("leaves the static pages, the short route and unknown names alone", () => {
    expect(canonicalAppPath("/app/todo/a%20b", names)).toBeNull();
    expect(canonicalAppPath("/app/notifications", names)).toBeNull();
    expect(canonicalAppPath("/app/Training%20Class/new", names)).toBeNull();
    expect(canonicalAppPath("/app/Projects/Task%20Tag", names)).toBeNull();
    expect(canonicalAppPath("/app/Projects/Work%20Item", null)).toBeNull();
  });
});
