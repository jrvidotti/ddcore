// Local drafts: what the user typed and has not saved yet, kept in the browser
// so that closing the tab, reloading the page or a stray click does not throw
// the work away. Storage is passed in rather than reached for, which keeps this
// module testable where there is no `localStorage`.

export interface Draft {
  doctype: string;
  /** the record's name, or "new" while it has none */
  name: string;
  /** the document as the user left it */
  doc: any;
  /** when the draft was written, ms since the epoch */
  savedAt: number;
  /** the document's `modified` when it was loaded — how a stale draft is spotted */
  modified: string | null;
}

/** The slice of `Storage` a draft needs. */
export interface DraftStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
  key(index: number): string | null;
  readonly length: number;
}

export const DRAFT_PREFIX = "ddcore_draft:";
/** A draft nobody came back for is not worth keeping forever. */
export const DRAFT_TTL_MS = 7 * 24 * 60 * 60 * 1000;

/** The browser's storage, or null where there is none (SSR, private mode). */
export function localDrafts(): DraftStore | null {
  try {
    return (globalThis as any).localStorage ?? null;
  } catch { /* private mode */ return null; }
}

/** A draft belongs to one user on this machine, and to one record. */
export function draftKey(user: string | null | undefined, doctype: string, name: string): string {
  return `${DRAFT_PREFIX}${user || "guest"}:${doctype}:${name || "new"}`;
}

export function readDraft(store: DraftStore | null, key: string, now = Date.now()): Draft | null {
  const d = parseDraft(store, key);
  if (!d) return null;
  if (now - d.savedAt > DRAFT_TTL_MS) { clearDraft(store, key); return null; }
  return d;
}

export function writeDraft(store: DraftStore | null, key: string, draft: Draft): void {
  try { store?.setItem(key, JSON.stringify(draft)); } catch { /* full or blocked */ }
}

export function clearDraft(store: DraftStore | null, key: string): void {
  try { store?.removeItem(key); } catch { /* blocked */ }
}

/** Drops every draft that expired or no longer parses, whoever wrote it. */
export function pruneDrafts(store: DraftStore | null, now = Date.now()): void {
  try {
    if (!store) return;
    const gone: string[] = [];
    for (let i = 0; i < store.length; i++) {
      const key = store.key(i);
      if (!key?.startsWith(DRAFT_PREFIX)) continue;
      const d = parseDraft(store, key);
      if (!d || now - d.savedAt > DRAFT_TTL_MS) gone.push(key);
    }
    for (const key of gone) clearDraft(store, key);
  } catch { /* blocked */ }
}

/**
 * What to do with the draft found for a record that has just been loaded:
 * restore it, ask first because the record changed underneath it, or nothing.
 */
export function draftDecision(draft: Draft | null, serverDoc: any): "none" | "restore" | "conflict" {
  if (!draft) return "none";
  if (JSON.stringify(draft.doc) === JSON.stringify(serverDoc)) return "none";
  return (draft.modified ?? null) === (serverDoc?.modified ?? null) ? "restore" : "conflict";
}

function parseDraft(store: DraftStore | null, key: string): Draft | null {
  try {
    const raw = store?.getItem(key);
    if (!raw) return null;
    const d = JSON.parse(raw);
    if (!d || typeof d !== "object" || !d.doc || typeof d.savedAt !== "number") return null;
    return d as Draft;
  } catch { /* corrupted or blocked */ return null; }
}
