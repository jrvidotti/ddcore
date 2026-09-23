import { __ } from "./boot.svelte";
// Keyboard shortcuts utilities: platform detection, modifier keys,
// editable element checks, and shortcut registry.

export interface ShortcutItem {
  keys: string[];
  description: string;
}

export interface ShortcutGroup {
  category: string;
  shortcuts: ShortcutItem[];
}

/** Check if the current environment is running on macOS or iOS */
export function isMac(platformOrUserAgent?: string): boolean {
  if (platformOrUserAgent !== undefined) {
    return /Mac|iPod|iPhone|iPad/i.test(platformOrUserAgent);
  }
  if (typeof navigator === "undefined") return false;
  const nav = navigator as any;
  if (nav.userAgentData?.platform) {
    return nav.userAgentData.platform.toLowerCase().includes("mac");
  }
  return /Mac|iPod|iPhone|iPad/i.test(navigator.platform || navigator.userAgent || "");
}

/** Returns '⌘' on macOS and 'Ctrl' on Windows/Linux */
export function getModifierKey(mac?: boolean): string {
  const isApple = mac !== undefined ? mac : isMac();
  return isApple ? "⌘" : "Ctrl";
}

/** Returns full modifier name ('Command' on macOS and 'Ctrl' on Windows/Linux) */
export function getModifierName(mac?: boolean): string {
  const isApple = mac !== undefined ? mac : isMac();
  return isApple ? "Command" : "Ctrl";
}

/**
 * Checks whether an event target is an interactive text input
 * where typing '?' should type the character rather than opening shortcuts.
 */
export function isEditableElement(target: EventTarget | null): boolean {
  if (!target || typeof target !== "object") return false;
  const el = target as HTMLElement;
  const tag = el.tagName?.toUpperCase();
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") {
    return true;
  }
  return Boolean(el.isContentEditable);
}

/**
 * Commits what is being typed before a keyboard action reads the document.
 * Controls commit on `change`/`blur`, and both only fire once focus leaves the
 * field — a shortcut such as Cmd+S does not move focus, so the pending edit has
 * to be flushed by hand or the save would post the value the field had before.
 * Returns whether there was an edit to flush.
 */
export function commitFocusedEdit(active: EventTarget | null): boolean {
  if (!isEditableElement(active)) return false;
  (active as HTMLElement).blur?.();
  return true;
}

/**
 * Determines whether a keydown event should trigger opening the shortcuts help.
 */
export function shouldToggleShortcuts(e: { key: string; ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean; target?: EventTarget | null }): boolean {
  // Cmd+/ or Ctrl+/ toggles shortcuts help from anywhere
  if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key === "/") {
    return true;
  }
  // '?' triggers shortcuts help ONLY when not focused on an editable element
  if (e.key === "?" && !e.ctrlKey && !e.metaKey && !e.altKey) {
    return !isEditableElement(e.target ?? null);
  }
  return false;
}

/**
 * Returns categorized shortcuts list with the appropriate modifier symbol for the user platform.
 */
export function getShortcutsList(mac?: boolean): ShortcutGroup[] {
  const mod = getModifierKey(mac);
  return [
    {
      category: __("Form & editing"),
      shortcuts: [
        { keys: [mod, "S"], description: __("Save or update the record") },
        { keys: [mod, "Enter"], description: __("Post a comment on the form") },
      ],
    },
    {
      category: __("Navigation & modals"),
      shortcuts: [
        { keys: [mod, "K"], description: __("Search documents and DocTypes") },
        { keys: ["?"], description: __("Show this shortcuts window") },
        { keys: [mod, "/"], description: __("Show this shortcuts window") },
        { keys: ["Esc"], description: __("Close modals, menus or dialogs") },
        { keys: ["Enter"], description: __("Confirm a dialog (its primary button)") },
      ],
    },
    {
      category: __("Search & selection (Link fields)"),
      shortcuts: [
        { keys: ["↓", "↑"], description: __("Move through the list options") },
        { keys: ["Enter"], description: __("Pick the highlighted option") },
        { keys: ["Esc"], description: __("Close the options list") },
      ],
    },
  ];
}
