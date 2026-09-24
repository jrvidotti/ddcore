import { describe, expect, it } from "vitest";
import { BASIC_COLORS, isDarkColor, normalizeColor } from "./color-state";

describe("normalizeColor", () => {
  it("writes one colour one way", () => {
    expect(normalizeColor("#ABC")).toBe("#aabbcc");
    expect(normalizeColor(" #A1B2C3 ")).toBe("#a1b2c3");
    expect(normalizeColor("a1b2c3")).toBe("#a1b2c3");
  });
  it("refuses what is not a colour", () => {
    for (const v of ["", "red", "#12345", "#gggggg", null, undefined]) {
      expect(normalizeColor(v)).toBe(null);
    }
  });
});

describe("isDarkColor", () => {
  it("decides what needs light text on top", () => {
    expect(isDarkColor("#000000")).toBe(true);
    expect(isDarkColor("#ffffff")).toBe(false);
    expect(isDarkColor("not a colour")).toBe(false);
  });
});

describe("BASIC_COLORS", () => {
  it("holds each colour once, already written the stored way", () => {
    for (const c of BASIC_COLORS) expect(normalizeColor(c)).toBe(c);
    expect(new Set(BASIC_COLORS).size).toBe(BASIC_COLORS.length);
    expect(BASIC_COLORS.length % 10).toBe(0);
  });
});
