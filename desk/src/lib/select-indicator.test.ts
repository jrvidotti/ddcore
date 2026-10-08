import { describe, it, expect } from "vitest";
import { isIndicatorSelect, selectIndicator } from "./format";

describe("isIndicatorSelect", () => {
  it("is a Select that declares optionColors or optionIcons", () => {
    expect(isIndicatorSelect({ fieldtype: "Select", options: ["A"] })).toBe(false);
    expect(isIndicatorSelect({ fieldtype: "Select", optionColors: {} })).toBe(false);
    expect(isIndicatorSelect({ fieldtype: "Select", optionColors: { A: "green" } })).toBe(true);
    expect(isIndicatorSelect({ fieldtype: "Select", optionIcons: { A: "check" } })).toBe(true);
    expect(isIndicatorSelect({ fieldtype: "Data", optionColors: { A: "green" } })).toBe(false);
  });

  it("takes a report column, which may carry no fieldtype", () => {
    expect(isIndicatorSelect({ optionIcons: { A: "check" } })).toBe(true);
  });
});

describe("selectIndicator", () => {
  const field = {
    fieldtype: "Select", options: ["Not registered", "Enabled", "Disabled"],
    optionLabels: ["Não cadastrado", "Ativo", "Inativo"],
    optionColors: { "Not registered": "gray", Enabled: "green", Disabled: "red" },
    optionIcons: { "Not registered": "x", Enabled: "check" },
  };

  it("is null for an empty value", () => {
    expect(selectIndicator("", field)).toBeNull();
    expect(selectIndicator(null, field)).toBeNull();
    expect(selectIndicator(undefined, field)).toBeNull();
  });

  it("draws the translated label, the value's colour and its icon", () => {
    expect(selectIndicator("Enabled", field)).toEqual({ label: "Ativo", color: "green", icon: "check" });
    expect(selectIndicator("Not registered", field)).toEqual({ label: "Não cadastrado", color: "gray", icon: "x" });
  });

  it("has no icon for a value optionIcons leaves out", () => {
    expect(selectIndicator("Disabled", field)).toEqual({ label: "Inativo", color: "red", icon: undefined });
  });

  it("keeps a value outside the options as its own label", () => {
    expect(selectIndicator("Retired", field)?.label).toBe("Retired");
  });
});
