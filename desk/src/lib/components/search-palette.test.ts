import { describe, expect, it } from "vitest";
import { buildItems, fold, matchDoctypes, moveIndex, shouldOpenSearch, workspaceFor } from "./search-palette";

const doctypes = {
  Task: { label: "Task", app: "testapp" },
  Project: { label: "Projeto", app: "testapp" },
  User: { label: "Usuário", app: "core" },
  "Task Type": { label: "Task Type", app: "testapp" },
};
const workspaces = [{ name: "work", label: "Work", app: "testapp", sidebar: [{ label: "Tasks", doctype: "Task" }] }];

describe("global search palette", () => {
  it("folds accents and case like the server", () => {
    expect(fold("Usuário ÇA")).toBe("usuario ca");
  });

  it("matches DocTypes by label or name, prefixes first", () => {
    expect(matchDoctypes("usua", doctypes)).toEqual([["User", "Usuário"]]);
    expect(matchDoctypes("project", doctypes)).toEqual([["Project", "Projeto"]]);
    expect(matchDoctypes("type", doctypes)).toEqual([["Task Type", "Task Type"]]);
    expect(matchDoctypes("task", doctypes).map(([n]) => n)).toEqual(["Task", "Task Type"]);
    expect(matchDoctypes("  ", doctypes)).toEqual([]);
  });

  it("opens each result in the workspace that owns its DocType", () => {
    expect(workspaceFor("Task", workspaces, doctypes, "other")).toBe("work");
    expect(workspaceFor("User", workspaces, doctypes, "other")).toBe("other");
    expect(workspaceFor("User", workspaces, doctypes, "")).toBe("work");
    expect(workspaceFor("User", [], doctypes, "")).toBe("core");
  });

  it("lists DocTypes before documents and keeps the server's order", () => {
    const items = buildItems("task", [
      { doctype: "Task", label: "Task", name: "TASK-2", title: "Task two" },
      { doctype: "User", label: "Usuário", name: "a@x.com", title: "" },
    ], { doctypes, workspaces, remembered: "" });
    expect(items.map((i) => [i.kind, i.title, i.href])).toEqual([
      ["doctype", "Task", "/app/work/Task"],
      ["doctype", "Task Type", "/app/work/Task%20Type"],
      ["document", "Task two", "/app/work/Task/TASK-2"],
      ["document", "a@x.com", "/app/work/User/a%40x.com"],
    ]);
  });

  it("wraps keyboard navigation", () => {
    expect(moveIndex(-1, 1, 3)).toBe(0);
    expect(moveIndex(-1, -1, 3)).toBe(2);
    expect(moveIndex(2, 1, 3)).toBe(0);
    expect(moveIndex(0, -1, 3)).toBe(2);
    expect(moveIndex(0, 1, 0)).toBe(-1);
  });

  it("opens on Mod+K only", () => {
    expect(shouldOpenSearch({ key: "k", metaKey: true })).toBe(true);
    expect(shouldOpenSearch({ key: "K", ctrlKey: true })).toBe(true);
    expect(shouldOpenSearch({ key: "k" })).toBe(false);
    expect(shouldOpenSearch({ key: "k", metaKey: true, shiftKey: true })).toBe(false);
  });
});
