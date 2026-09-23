// Central reactive store for resolving, caching, and batch-fetching link titles in desk.
import { api } from "./api";
import { boot } from "./boot.svelte";

const cache = $state<Record<string, Record<string, string>>>({});
const pending = new Map<string, Set<string>>();
let batchTimer: any = null;

/** Registers a batch of titles into the reactive cache, e.g. from api.get or api.list. */
export function registerTitles(titles: Record<string, Record<string, string>>) {
  if (!titles) return;
  for (const [dt, map] of Object.entries(titles)) {
    if (!cache[dt]) cache[dt] = {};
    for (const [id, title] of Object.entries(map)) {
      if (title) cache[dt][id] = title;
    }
  }
}

/** Sets a single link title in the reactive cache. */
export function setLinkTitle(doctype: string, id: string, title: string) {
  if (!doctype || !id) return;
  if (!cache[doctype]) cache[doctype] = {};
  cache[doctype][id] = title;
}

/**
 * Returns the title for a document if cached. If uncached and the doctype
 * has a titleField, schedules a background batch fetch and returns the id as fallback.
 */
export function getLinkTitle(doctype: string, id: string): string {
  if (!doctype || !id) return "";
  const existing = cache[doctype]?.[id];
  if (existing !== undefined) return existing;

  // If doctype metadata is known and has no titleField (or titleField === "id"),
  // then the id is the title.
  const dtMeta = boot.data?.doctypes[doctype];
  if (boot.ready && dtMeta && (!dtMeta.titleField || dtMeta.titleField === "id")) {
    return id;
  }

  queueFetch(doctype, id);
  return id;
}

export function hasLinkTitle(doctype: string, id: string): boolean {
  return cache[doctype]?.[id] !== undefined;
}

export function clearTitleCache() {
  for (const k of Object.keys(cache)) {
    delete cache[k];
  }
}

function queueFetch(doctype: string, id: string) {
  // a portal user has no title endpoint: the portal's responses carry theirs
  if (boot.data?.website) return;
  if (!pending.has(doctype)) pending.set(doctype, new Set());
  pending.get(doctype)!.add(id);

  clearTimeout(batchTimer);
  batchTimer = setTimeout(flushPending, 30);
}

async function flushPending() {
  const current = new Map(pending);
  pending.clear();

  for (const [doctype, namesSet] of current.entries()) {
    const names = Array.from(namesSet).filter((n) => cache[doctype]?.[n] === undefined);
    if (!names.length) continue;
    try {
      const res = await api.linkTitles(doctype, names);
      if (res) registerTitles(res);
    } catch {
      // Ignore errors, fallback to the id
    }
  }
}
