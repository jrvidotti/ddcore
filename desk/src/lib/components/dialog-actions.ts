export async function runDialogAction<T extends { busy: boolean }>(
  action: (values: Record<string, any>, dialog: T) => any,
  values: Record<string, any>,
  dialog: T,
): Promise<void> {
  dialog.busy = true;
  try { await action(values, dialog); } finally { dialog.busy = false; }
}

export interface DialogKey {
  key: string;
  ctrlKey?: boolean;
  metaKey?: boolean;
  shiftKey?: boolean;
  altKey?: boolean;
  repeat?: boolean;
  isComposing?: boolean;
  defaultPrevented?: boolean;
  /** tag name of the focused element, upper-case as the DOM reports it */
  targetTag?: string;
  targetEditable?: boolean;
}

/**
 * What a key pressed while a dialog is on top does: Escape cancels it, Enter
 * runs its primary action. Enter is left alone where it already means
 * something — a new line in a textarea or rich text (unless Ctrl/Cmd is held),
 * a focused button or link, an option picked in a dropdown (defaultPrevented),
 * or an IME composition.
 */
export function dialogKeyAction(e: DialogKey): "cancel" | "primary" | null {
  if (e.isComposing || e.defaultPrevented) return null;
  if (e.key === "Escape") return "cancel";
  if (e.key !== "Enter" || e.repeat || e.shiftKey || e.altKey) return null;
  const tag = e.targetTag || "";
  if (tag === "BUTTON" || tag === "A") return null;
  if ((tag === "TEXTAREA" || e.targetEditable) && !(e.ctrlKey || e.metaKey)) return null;
  return "primary";
}
