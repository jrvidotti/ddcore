import { describe, expect, it } from "vitest";
import { committedValue, filterSuggestions, moveActive } from "./autocomplete-state";

describe("filterSuggestions", () => {
  const options = ["São Paulo", "Rio de Janeiro", "Paulínia", "Santos", ""];

  it("lists every suggestion for an empty text, blanks dropped", () => {
    expect(filterSuggestions(options, "")).toEqual(["São Paulo", "Rio de Janeiro", "Paulínia", "Santos"]);
  });

  it("ignores accents and case, prefixes first", () => {
    expect(filterSuggestions(options, "PAUL")).toEqual(["Paulínia", "São Paulo"]);
    expect(filterSuggestions(options, "sao")).toEqual(["São Paulo"]);
  });

  it("drops repeats and honours the limit", () => {
    expect(filterSuggestions(["a", "a", "ab", "abc"], "a", 2)).toEqual(["a", "ab"]);
  });

  it("finds nothing for text no suggestion holds", () => {
    expect(filterSuggestions(options, "Curitiba")).toEqual([]);
  });
});

describe("moveActive", () => {
  it("steps down from the typed text into the list and stops at the end", () => {
    expect(moveActive(-1, "ArrowDown", 3)).toBe(0);
    expect(moveActive(2, "ArrowDown", 3)).toBe(2);
  });

  it("steps back up to the typed text and no further", () => {
    expect(moveActive(0, "ArrowUp", 3)).toBe(-1);
    expect(moveActive(-1, "ArrowUp", 3)).toBe(-1);
  });

  it("has nothing to highlight in an empty list", () => {
    expect(moveActive(1, "ArrowDown", 0)).toBe(-1);
  });
});

describe("committedValue", () => {
  it("trims, and stores a blank as null", () => {
    expect(committedValue("  Green ")).toBe("Green");
    expect(committedValue("   ")).toBe(null);
  });
});
