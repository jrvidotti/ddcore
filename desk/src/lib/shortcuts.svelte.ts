// Reactive state and actions for the shortcuts modal dialog in Svelte 5.
export * from "./shortcuts";

export const shortcutsState = $state<{ open: boolean }>({ open: false });

export function openShortcutsHelp() {
  shortcutsState.open = true;
}

export function closeShortcutsHelp() {
  shortcutsState.open = false;
}

export function toggleShortcutsHelp() {
  shortcutsState.open = !shortcutsState.open;
}
