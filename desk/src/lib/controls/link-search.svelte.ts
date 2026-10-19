// The typeahead of the Link and Table MultiSelect controls: a debounced search that drops stale answers, the open list and its highlight.
import { boot } from "$lib/boot.svelte";
import { setLinkTitle } from "$lib/titles.svelte";

interface Source {
  target: () => string | undefined;
  fetch: (text: string) => Promise<any[]>;
  /** An option's own title (a portal search's rows); undefined falls back to the default. */
  ownTitle?: (o: any) => string | undefined;
}

export class LinkSearch {
  text = $state("");
  open = $state(false);
  options = $state<any[]>([]);
  active = $state(0);
  #timer: ReturnType<typeof setTimeout> | undefined;
  #version = 0;
  #src!: Source;

  constructor(src: Source) { this.#src = src; }

  /** The translated id the server sends, else the target's title field, else the id. */
  title(o: any): string {
    const own = this.#src.ownTitle?.(o);
    const target = this.#src.target();
    const titleField = target ? boot.data?.doctypes[target]?.titleField : undefined;
    return own ?? String(o._title || (titleField && o[titleField]) || o.id);
  }

  async search(text: string) {
    const target = this.#src.target();
    if (!target) { this.options = []; return; }
    const version = ++this.#version;
    try {
      const results = await this.#src.fetch(text);
      if (version !== this.#version) return;
      this.options = results;
      for (const o of results) setLinkTitle(target, o.id, this.title(o));
      this.active = 0;
      this.open = true;
    } catch { this.options = []; }
  }

  /** The user typed: search after a pause, or at once. */
  typed(text: string, now = false) {
    this.text = text;
    clearTimeout(this.#timer);
    if (now) void this.search(text);
    else this.#timer = setTimeout(() => this.search(this.text), 150);
  }

  /** Drops the pending search and the answer of the one in flight. */
  cancel() { clearTimeout(this.#timer); this.#version++; }

  /** Closes the list a moment later, so a click on an option still lands, then runs `after`. */
  blur(after: () => void) {
    this.cancel();
    setTimeout(() => { this.open = false; after(); }, 150);
  }

  keydown(e: KeyboardEvent, items: any[], pick: (o: any) => void) {
    if (!this.open) return;
    if (e.key === "ArrowDown") { this.active = Math.min(this.active + 1, items.length - 1); e.preventDefault(); }
    else if (e.key === "ArrowUp") { this.active = Math.max(this.active - 1, 0); e.preventDefault(); }
    else if (e.key === "Enter") { if (items[this.active]) pick(items[this.active]); e.preventDefault(); }
    else if (e.key === "Escape") { this.open = false; e.preventDefault(); }
  }
}
