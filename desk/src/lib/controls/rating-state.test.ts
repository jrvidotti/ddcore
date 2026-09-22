import { describe, expect, it } from "vitest";
import { clampRating, ratingMax, ratingOnKey, ratingOnPick, ratingStars } from "./rating-state";

describe("ratingMax", () => {
  it("takes the number of stars from options, within reason", () => {
    expect(ratingMax(undefined)).toBe(5);
    expect(ratingMax(10)).toBe(10);
    expect(ratingMax("3")).toBe(3);
    expect(ratingMax(0)).toBe(5);
    expect(ratingMax(99)).toBe(5);
  });
});

describe("clampRating", () => {
  it("keeps values within range", () => {
    expect(clampRating(3, 5)).toBe(3);
    expect(clampRating(0, 5)).toBe(0);
    expect(clampRating(5, 5)).toBe(5);
  });
  it("clamps out-of-range values", () => {
    expect(clampRating(42, 5)).toBe(5);
    expect(clampRating(-3, 5)).toBe(0);
  });
  it("preserves null and undefined as not rated", () => {
    expect(clampRating(null, 5)).toBe(null);
    expect(clampRating(undefined, 5)).toBe(null);
    expect(clampRating("", 5)).toBe(null);
  });
});

describe("picking a star", () => {
  it("sets the value", () => expect(ratingOnPick(2, 4)).toBe(4));
  it("clears it when the current star is picked again", () => expect(ratingOnPick(4, 4)).toBe(null));
});

describe("the keyboard", () => {
  it("moves within the range", () => {
    expect(ratingOnKey(2, "ArrowRight", 5)).toBe(3);
    expect(ratingOnKey(5, "ArrowRight", 5)).toBe(5);
    expect(ratingOnKey(0, "ArrowLeft", 5)).toBe(0);
    expect(ratingOnKey(2, "End", 5)).toBe(5);
    expect(ratingOnKey(2, "Delete", 5)).toBe(null);
  });
  it("leaves other keys alone", () => expect(ratingOnKey(2, "a", 5)).toBe(undefined));
});

describe("ratingStars", () => {
  it("fills up to the value", () => expect(ratingStars(2, 4)).toEqual([true, true, false, false]));
  it("treats null as none filled", () => expect(ratingStars(null, 3)).toEqual([false, false, false]));
  it("clamps values outside range", () => {
    expect(ratingStars(42, 5)).toEqual([true, true, true, true, true]);
    expect(ratingStars(-3, 5)).toEqual([false, false, false, false, false]);
  });
});
