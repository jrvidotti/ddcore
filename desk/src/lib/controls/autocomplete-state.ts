/**
 * An Autocomplete is free text with suggestions. The suggestions come from
 * the field's `options` — static in the DocType, or replaced at runtime with
 * `frm.setDfProperty(field, "options", list)` — and they never limit what can
 * be stored.
 */
import { fold } from "$lib/text";

/** How many suggestions the list shows at once. */
export const SUGGESTION_LIMIT = 50;

/**
 * The suggestions matching `text`, ignoring case and accents: those that
 * start with it first, then those that contain it, each group in the order the
 * options declare. An empty text lists them all. Blanks and repeats are
 * dropped, so a newline-separated `options` with a trailing line is harmless.
 */
export function filterSuggestions(options: string[], text: string, limit = SUGGESTION_LIMIT): string[] {
  const needle = fold(String(text ?? "").trim());
  const seen = new Set<string>();
  const starts: string[] = [];
  const contains: string[] = [];
  for (const o of options) {
    if (!o || !o.trim() || seen.has(o)) continue;
    seen.add(o);
    const hay = fold(o);
    if (!needle || hay.startsWith(needle)) starts.push(o);
    else if (hay.includes(needle)) contains.push(o);
  }
  return [...starts, ...contains].slice(0, limit);
}

/**
 * The highlighted suggestion after an arrow key. -1 is "none": the typed text
 * stands, and Enter keeps it. ArrowUp from the first suggestion goes back to
 * it; neither end wraps. Any other key leaves the index where it was.
 */
export function moveActive(current: number, key: string, count: number): number {
  if (count <= 0) return -1;
  if (key === "ArrowDown") return Math.min(current + 1, count - 1);
  if (key === "ArrowUp") return Math.max(current - 1, -1);
  return current;
}

/** What a commit stores: the text trimmed, and a blank as null — as the server does. */
export function committedValue(text: string): string | null {
  const s = String(text ?? "").trim();
  return s === "" ? null : s;
}
