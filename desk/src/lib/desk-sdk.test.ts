import { describe, expect, it, vi } from "vitest";

vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("./ui.svelte", () => ({
  dialog: vi.fn(),
  toast: vi.fn(),
  confirm: vi.fn(),
  prompt: vi.fn(),
  showError: vi.fn(),
  ui: { busy: 0 },
}));
const bootState = vi.hoisted(() => ({ data: null as any }));
vi.mock("./boot.svelte", () => ({
  __: (s: string) => s,
  boot: bootState,
  hasRole: (r: string) => !!bootState.data?.roles.includes(r),
}));

import { deskSDK } from "./desk-sdk";

describe("deskSDK profile sections", () => {
  it("keeps the order apps register them in, and replaces one registered again by id", () => {
    deskSDK.defineProfileSection({ id: "a", title: "A" });
    deskSDK.defineProfileSection({ id: "b", title: "B" });
    deskSDK.defineProfileSection({ id: "a", title: "A again" });
    expect(deskSDK.profileSections().map((s) => [s.id, s.title])).toEqual([["a", "A again"], ["b", "B"]]);
  });

  it("hands out a copy, so the page cannot change the registry", () => {
    deskSDK.profileSections().length = 0;
    expect(deskSDK.profileSections().length).toBeGreaterThan(0);
  });
});

describe("deskSDK listRegistry", () => {
  it("registers and retrieves listView options including docstatusFilter", () => {
    deskSDK.defineListView("Contrato", { docstatusFilter: false });
    expect(deskSDK.listSettings("Contrato")).toEqual({ docstatusFilter: false });
  });

  it("registers and retrieves the idColumn option", () => {
    deskSDK.defineListView("Empresa", { idColumn: false });
    expect(deskSDK.listSettings("Empresa")).toEqual({ idColumn: false });
  });

  it("registers and retrieves the plainLinks option", () => {
    deskSDK.defineListView("Turma", { plainLinks: ["course"] });
    expect(deskSDK.listSettings("Turma")).toEqual({ plainLinks: ["course"] });
  });

  it("registers and retrieves the modifiedColumn option", () => {
    deskSDK.defineListView("Contrato", { modifiedColumn: false });
    expect(deskSDK.listSettings("Contrato")).toEqual({ modifiedColumn: false });
  });

  it("registers and retrieves the filtersCollapsed option", () => {
    deskSDK.defineListView("Campaign", { filtersCollapsed: false });
    expect(deskSDK.listSettings("Campaign")).toEqual({ filtersCollapsed: false });
  });

  it("allows registering list options without docstatusFilter", () => {
    deskSDK.defineListView("Task", { pageSize: 50 });
    expect(deskSDK.listSettings("Task")).toEqual({ pageSize: 50 });
  });
});

describe("deskSDK session", () => {
  it("reads the signed-in user from boot", () => {
    bootState.data = { user: "ana@x.com", roles: ["Portal Manager"], userDoc: { id: "ana@x.com", full_name: "Ana" }, lang: "pt-BR" };
    expect(deskSDK.ddcore.session).toEqual({ user: "ana@x.com", fullName: "Ana", roles: ["Portal Manager"], lang: "pt-BR" });
    expect(deskSDK.ddcore.hasRole("Portal Manager")).toBe(true);
    expect(deskSDK.ddcore.hasRole("Portal User")).toBe(false);
  });

  it("gives a copy of the roles, so a script cannot change the desk's", () => {
    bootState.data = { user: "ana@x.com", roles: ["Portal Manager"], userDoc: null, lang: "en" };
    deskSDK.ddcore.session.roles.push("System Manager");
    expect(deskSDK.ddcore.hasRole("System Manager")).toBe(false);
    expect(deskSDK.ddcore.session.fullName).toBe("ana@x.com");
  });

  it("is the guest before boot", () => {
    bootState.data = null;
    expect(deskSDK.ddcore.session).toEqual({ user: "Guest", fullName: "Guest", roles: [], lang: "" });
    expect(deskSDK.ddcore.hasRole("Guest")).toBe(false);
  });
});

describe("docstatus filter visibility logic", () => {
  function shouldShowDocstatusFilter(submittable?: boolean, docstatusFilterSetting?: boolean): boolean {
    return !!submittable && docstatusFilterSetting !== false;
  }

  it("shows filter by default for submittable DocTypes when setting is undefined or true", () => {
    expect(shouldShowDocstatusFilter(true, undefined)).toBe(true);
    expect(shouldShowDocstatusFilter(true, true)).toBe(true);
  });

  it("hides filter when docstatusFilter is explicitly false for submittable DocTypes", () => {
    expect(shouldShowDocstatusFilter(true, false)).toBe(false);
  });

  it("does not show filter for non-submittable DocTypes", () => {
    expect(shouldShowDocstatusFilter(false, undefined)).toBe(false);
    expect(shouldShowDocstatusFilter(false, true)).toBe(false);
    expect(shouldShowDocstatusFilter(false, false)).toBe(false);
    expect(shouldShowDocstatusFilter(undefined, false)).toBe(false);
  });
});
