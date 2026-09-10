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
      category: "Formulário & Edição",
      shortcuts: [
        { keys: [mod, "S"], description: "Salvar ou atualizar registro" },
        { keys: [mod, "Enter"], description: "Enviar comentário no formulário" },
      ],
    },
    {
      category: "Navegação & Modais",
      shortcuts: [
        { keys: ["?"], description: "Exibir esta janela de atalhos" },
        { keys: [mod, "/"], description: "Exibir esta janela de atalhos" },
        { keys: ["Esc"], description: "Fechar modais, menus ou diálogos" },
      ],
    },
    {
      category: "Busca & Seleção (Campos Link)",
      shortcuts: [
        { keys: ["↓", "↑"], description: "Navegar entre as opções da lista" },
        { keys: ["Enter"], description: "Selecionar opção destacada" },
        { keys: ["Esc"], description: "Fechar lista de opções" },
      ],
    },
  ];
}
