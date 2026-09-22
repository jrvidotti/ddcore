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
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));

import { deskSDK } from "./desk-sdk";

describe("deskSDK listRegistry", () => {
  it("registers and retrieves listView options including docstatusFilter", () => {
    deskSDK.defineListView("Contrato", { docstatusFilter: false });
    expect(deskSDK.listSettings("Contrato")).toEqual({ docstatusFilter: false });
  });

  it("registers and retrieves the nameColumn option", () => {
    deskSDK.defineListView("Empresa", { nameColumn: false });
    expect(deskSDK.listSettings("Empresa")).toEqual({ nameColumn: false });
  });

  it("registers and retrieves the modifiedColumn option", () => {
    deskSDK.defineListView("Contrato", { modifiedColumn: false });
    expect(deskSDK.listSettings("Contrato")).toEqual({ modifiedColumn: false });
  });

  it("allows registering list options without docstatusFilter", () => {
    deskSDK.defineListView("Task", { pageSize: 50 });
    expect(deskSDK.listSettings("Task")).toEqual({ pageSize: 50 });
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
