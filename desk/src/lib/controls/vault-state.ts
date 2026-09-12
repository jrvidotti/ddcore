export type VaultMode = "configured" | "cleared" | "editing" | "empty";

export interface VaultState {
  mode: VaultMode;
  isNew: boolean;
}

/**
 * Resolves the visual mode for a Vault field control.
 */
export function resolveVaultState(value: any, isEditing: boolean, wasConfigured: boolean): VaultState {
  if (value && typeof value === "object" && value.clear === true) {
    return { mode: "cleared", isNew: false };
  }
  if (isEditing) {
    return { mode: "editing", isNew: !wasConfigured };
  }
  if (value && typeof value === "object" && value.configured === true) {
    return { mode: "configured", isNew: false };
  }
  if (typeof value === "string" && value.length > 0) {
    return { mode: "editing", isNew: !wasConfigured };
  }
  return { mode: "empty", isNew: !wasConfigured };
}
