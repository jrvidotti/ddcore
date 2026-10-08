import { describe, expect, it, vi } from "vitest";
import { FormController } from "./form.svelte";
import type { Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const meta = {
  doctype: {
    name: "Team", label: "Team", formApps: [],
    fields: [{ fieldname: "users", fieldtype: "Table", label: "Users", options: "Team User", gridSelect: true }],
  },
  children: {},
  permissions: { read: true, write: true },
} as unknown as Meta;

const frm = () => new FormController(meta, { doctype: "Team", id: "T-1", users: [{ name: "a" }, { name: "b" }] });

describe("grid actions", () => {
  it("keeps each action under its grid, replacing one with the same key or, without a key, the same label", () => {
    const f = frm();
    f.addGridAction("users", { label: "Enable", onClick: () => {} });
    f.addGridAction("users", { label: "Enable", primary: true, onClick: () => {} });
    f.addGridAction("users", { key: "roles", label: "Set Roles", onClick: () => {} });
    f.addGridAction("users", { key: "roles", label: "Set roles", onClick: () => {} });
    expect(f.gridActions.users.map((a) => [a.label, !!a.primary])).toEqual([["Enable", true], ["Set roles", false]]);
    expect(f.gridActions.other).toBeUndefined();
  });

  it("removes one action by key (or label) and every action of the grid without one", () => {
    const f = frm();
    f.addGridAction("users", { label: "Enable", onClick: () => {} });
    f.addGridAction("users", { key: "roles", label: "Set Roles", onClick: () => {} });
    f.removeGridAction("users", "Enable");
    expect(f.gridActions.users.map((a) => a.label)).toEqual(["Set Roles"]);
    f.removeGridAction("users", "roles");
    expect(f.gridActions.users).toBeUndefined();
    f.addGridAction("users", { label: "Enable", onClick: () => {} });
    f.removeGridAction("users");
    expect(f.gridActions.users).toBeUndefined();
  });

  it("clears them with the toolbar buttons, so a refresh starts from scratch", () => {
    const f = frm();
    f.addGridAction("users", { label: "Enable", onClick: () => {} });
    f.clearButtons();
    expect(f.gridActions).toEqual({});
  });
});

describe("grid selection", () => {
  it("reads and clears the selection the grid on screen registered", () => {
    const f = frm();
    expect(f.getSelectedRows("users")).toEqual([]);
    let picked = [f.doc.users[1]];
    const clear = vi.fn(() => { picked = []; });
    const off = f.registerGridSelection("users", { rows: () => picked, clear });
    expect(f.getSelectedRows("users")).toEqual([f.doc.users[1]]);
    f.clearSelection("users");
    expect(clear).toHaveBeenCalledOnce();
    expect(f.getSelectedRows("users")).toEqual([]);
    off();
    picked = [f.doc.users[0]];
    expect(f.getSelectedRows("users")).toEqual([]);
    f.clearSelection("users"); // no grid on screen: nothing to clear
  });

  it("hands out a copy, so the caller cannot change the grid's selection", () => {
    const f = frm();
    const picked = [f.doc.users[0]];
    f.registerGridSelection("users", { rows: () => picked, clear: () => {} });
    f.getSelectedRows("users").pop();
    expect(f.getSelectedRows("users")).toHaveLength(1);
  });

  it("an older registration going away leaves the newer one in place", () => {
    const f = frm();
    const offOld = f.registerGridSelection("users", { rows: () => ["old"], clear: () => {} });
    f.registerGridSelection("users", { rows: () => ["new"], clear: () => {} });
    offOld();
    expect(f.getSelectedRows("users")).toEqual(["new"]);
  });
});
