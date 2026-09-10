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
    for (const [name, title] of Object.entries(map)) {
      if (title) cache[dt][name] = title;
    }
  }
}

/** Sets a single link title in the reactive cache. */
export function setLinkTitle(doctype: string, name: string, title: string) {
  if (!doctype || !name) return;
  if (!cache[doctype]) cache[doctype] = {};
  cache[doctype][name] = title;
}

/**
 * Returns the title for a document if cached. If uncached and the doctype
 * has a titleField, schedules a background batch fetch and returns name as fallback.
 */
export function getLinkTitle(doctype: string, name: string): string {
  if (!doctype || !name) return "";
  const existing = cache[doctype]?.[name];
  if (existing !== undefined) return existing;

  // If doctype metadata is known and has no titleField (or titleField === "name"),
  // then name is the title.
  const dtMeta = boot.data?.doctypes[doctype];
  if (boot.ready && dtMeta && (!dtMeta.titleField || dtMeta.titleField === "name")) {
    return name;
  }

  queueFetch(doctype, name);
  return name;
}

export function hasLinkTitle(doctype: string, name: string): boolean {
  return cache[doctype]?.[name] !== undefined;
}

export function clearTitleCache() {
  for (const k of Object.keys(cache)) {
    delete cache[k];
  }
}

function queueFetch(doctype: string, name: string) {
  if (!pending.has(doctype)) pending.set(doctype, new Set());
  pending.get(doctype)!.add(name);

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
      // Ignore errors, fallback to name
    }
  }
}
