import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LinkSearch } from "./link-search.svelte";

vi.mock("$lib/boot.svelte", () => ({ boot: { data: { doctypes: { Pessoa: { titleField: "nome" } } } } }));
vi.mock("$lib/titles.svelte", () => ({ setLinkTitle: vi.fn() }));

const key = (k: string) => ({ key: k, preventDefault: vi.fn() }) as unknown as KeyboardEvent;

describe("LinkSearch", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("titles an option by its translated id, its title field, or its id", () => {
    const ls = new LinkSearch({ target: () => "Pessoa", fetch: async () => [] });
    expect(ls.title({ id: "P1", _title: "Ana" })).toBe("Ana");
    expect(ls.title({ id: "P1", nome: "Bia" })).toBe("Bia");
    expect(ls.title({ id: "P1" })).toBe("P1");
  });

  it("searches after a pause and drops the answer of a superseded search", async () => {
    const fetch = vi.fn(async (t: string) => [{ id: t }]);
    const ls = new LinkSearch({ target: () => "Pessoa", fetch });
    ls.typed("a");
    ls.typed("ab");
    await vi.advanceTimersByTimeAsync(150);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(ls.options).toEqual([{ id: "ab" }]);
    expect(ls.open).toBe(true);
    const slow = ls.search("x");
    ls.cancel();
    await slow;
    expect(ls.options).toEqual([{ id: "ab" }]);
  });

  it("moves the highlight within the list, picks on Enter and closes on Escape", () => {
    const ls = new LinkSearch({ target: () => "Pessoa", fetch: async () => [] });
    const pick = vi.fn();
    const items = [{ id: "a" }, { id: "b" }];
    ls.keydown(key("ArrowDown"), items, pick);
    expect(ls.active).toBe(0);
    ls.open = true;
    ls.keydown(key("ArrowDown"), items, pick);
    ls.keydown(key("ArrowDown"), items, pick);
    expect(ls.active).toBe(1);
    ls.keydown(key("Enter"), items, pick);
    expect(pick).toHaveBeenCalledWith({ id: "b" });
    ls.keydown(key("Escape"), items, pick);
    expect(ls.open).toBe(false);
  });

  it("closes on blur after a moment and runs the callback", () => {
    const ls = new LinkSearch({ target: () => "Pessoa", fetch: async () => [] });
    const after = vi.fn();
    ls.open = true;
    ls.blur(after);
    expect(ls.open).toBe(true);
    vi.advanceTimersByTime(150);
    expect(ls.open).toBe(false);
    expect(after).toHaveBeenCalled();
  });
});
