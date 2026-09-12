import { describe, expect, it } from "vitest";
import { resolveVaultState } from "./vault-state";

describe("resolveVaultState", () => {
  it("resolves to configured when value is { configured: true }", () => {
    const s = resolveVaultState({ configured: true }, false, true);
    expect(s.mode).toBe("configured");
  });

  it("resolves to cleared when value is { clear: true }", () => {
    const s = resolveVaultState({ clear: true }, false, true);
    expect(s.mode).toBe("cleared");
  });

  it("resolves to empty when value is null/undefined and not editing", () => {
    expect(resolveVaultState(null, false, false).mode).toBe("empty");
    expect(resolveVaultState(undefined, false, false).mode).toBe("empty");
    expect(resolveVaultState("", false, false).mode).toBe("empty");
  });

  it("resolves to editing when isEditing is true", () => {
    const s = resolveVaultState(null, true, true);
    expect(s.mode).toBe("editing");
    expect(s.isNew).toBe(false);

    const sNew = resolveVaultState(null, true, false);
    expect(sNew.mode).toBe("editing");
    expect(sNew.isNew).toBe(true);
  });

  it("resolves to editing when a secret string is being entered", () => {
    const s = resolveVaultState("my-new-secret", false, false);
    expect(s.mode).toBe("editing");
  });

  it("cleared takes precedence over isEditing", () => {
    const s = resolveVaultState({ clear: true }, true, true);
    expect(s.mode).toBe("cleared");
  });
});
