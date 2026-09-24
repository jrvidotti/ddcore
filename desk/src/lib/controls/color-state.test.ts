import { describe, expect, it } from "vitest";
import { BASIC_COLORS, hexToRgb, hsvToRgb, isDarkColor, normalizeColor, rgbToHex, rgbToHsv } from "./color-state";

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

describe("rgb and hsv", () => {
  it("reads and writes a hex", () => {
    expect(hexToRgb("#F97316")).toEqual({ r: 249, g: 115, b: 22 });
    expect(hexToRgb("nope")).toBe(null);
    expect(rgbToHex({ r: 249, g: 115, b: 22 })).toBe("#f97316");
    expect(rgbToHex({ r: 300, g: -4, b: NaN })).toBe("#ff0000");
  });
  it("goes to hsv and back without losing the colour", () => {
    expect(rgbToHsv({ r: 255, g: 0, b: 0 })).toEqual({ h: 0, s: 1, v: 1 });
    expect(rgbToHsv({ r: 0, g: 0, b: 255 }).h).toBe(240);
    for (const c of BASIC_COLORS) expect(rgbToHex(hsvToRgb(rgbToHsv(hexToRgb(c)!)))).toBe(c);
  });
});
