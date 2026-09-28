import { describe, expect, it } from "vitest";
import { fitSize, isSignatureDataUrl, padBounds, trimBounds } from "./signature-state";

// A w×h RGBA buffer with the given pixels opaque.
function image(w: number, h: number, opaque: [number, number][]) {
  const data = new Uint8ClampedArray(w * h * 4);
  for (const [x, y] of opaque) data[(y * w + x) * 4 + 3] = 255;
  return { width: w, height: h, data };
}

describe("signature-state", () => {
  it("accepts only a PNG data URL as something to show", () => {
    expect(isSignatureDataUrl("data:image/png;base64,iVBORw0KGgo=")).toBe(true);
    expect(isSignatureDataUrl("data:image/png;base64,")).toBe(false);
    expect(isSignatureDataUrl("data:image/jpeg;base64,/9j/4AAQ")).toBe(false);
    expect(isSignatureDataUrl("data:image/svg+xml;base64,PHN2Zz4=")).toBe(false);
    expect(isSignatureDataUrl("javascript:alert(1)")).toBe(false);
    expect(isSignatureDataUrl('data:image/png;base64,AAAA" onerror="alert(1)')).toBe(false);
    expect(isSignatureDataUrl("https://example.com/s.png")).toBe(false);
    expect(isSignatureDataUrl(null)).toBe(false);
    expect(isSignatureDataUrl(42)).toBe(false);
  });

  it("finds the box around the strokes, or null on an empty pad", () => {
    expect(trimBounds(image(10, 6, []))).toBe(null);
    expect(trimBounds(image(10, 6, [[3, 2]]))).toEqual({ x: 3, y: 2, width: 1, height: 1 });
    expect(trimBounds(image(10, 6, [[2, 4], [7, 1], [5, 3]]))).toEqual({ x: 2, y: 1, width: 6, height: 4 });
    expect(trimBounds(image(10, 6, [[0, 0], [9, 5]]))).toEqual({ x: 0, y: 0, width: 10, height: 6 });
    // a half-transparent edge pixel is still part of the stroke
    const soft = image(4, 4, []);
    soft.data[(1 * 4 + 2) * 4 + 3] = 10;
    expect(trimBounds(soft)).toEqual({ x: 2, y: 1, width: 1, height: 1 });
  });

  it("pads the box without leaving the canvas", () => {
    expect(padBounds({ x: 10, y: 10, width: 5, height: 5 }, 4, 100, 100)).toEqual({ x: 6, y: 6, width: 13, height: 13 });
    expect(padBounds({ x: 1, y: 2, width: 97, height: 5 }, 4, 100, 10)).toEqual({ x: 0, y: 0, width: 100, height: 10 });
  });

  it("scales a crop past the server's limits down, keeping its shape", () => {
    expect(fitSize(600, 200)).toEqual({ width: 600, height: 200 });
    expect(fitSize(4000, 400)).toEqual({ width: 2000, height: 200 });
    expect(fitSize(1000, 2000)).toEqual({ width: 500, height: 1000 });
  });
});
