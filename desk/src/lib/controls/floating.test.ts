import { describe, it, expect } from "vitest";
import { computeAnchoredPosition } from "./floating";

// A dropdown anchored to an input inside a grid used to be clipped by the
// grid's `overflow:auto` card. It is now positioned against the viewport, so
// the maths that places it has to stand on its own.

const viewport = { width: 1000, height: 800 };

describe("computeAnchoredPosition", () => {
  it("opens below the anchor when there is room", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 100, top: 200, width: 300, height: 32 },
      panel: { width: 300, height: 200 },
      viewport,
    });

    expect(pos.placement).toBe("below");
    expect(pos.top).toBe(236); // 200 + 32 + gap
    expect(pos.left).toBe(100);
  });

  it("flips above when the panel does not fit below and fits above", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 100, top: 600, width: 300, height: 32 },
      panel: { width: 300, height: 260 },
      viewport,
    });

    expect(pos.placement).toBe("above");
    expect(pos.top).toBe(336); // 600 - gap - 260
  });

  it("stays below when neither side fits but below has more room", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 100, top: 300, width: 300, height: 32 },
      panel: { width: 300, height: 600 },
      viewport,
    });

    expect(pos.placement).toBe("below");
  });

  it("takes the anchor's width when asked to match it", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 100, top: 200, width: 300, height: 32 },
      panel: { width: 120, height: 100 },
      viewport,
      matchWidth: true,
    });

    expect(pos.width).toBe(300);
  });

  it("grows past the anchor's width when its content is wider", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 100, top: 200, width: 190, height: 32 },
      panel: { width: 320, height: 100 },
      viewport,
      matchWidth: true,
    });

    expect(pos.width).toBe(320);
    expect(pos.left).toBe(100);
  });

  it("shifts a list wider than its anchor back into the viewport", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 800, top: 200, width: 190, height: 32 },
      panel: { width: 320, height: 100 },
      viewport,
      matchWidth: true,
    });

    expect(pos.width).toBe(320);
    expect(pos.left).toBe(672); // 1000 - 320 - margin
  });

  it("keeps its own width, aligned to the anchor's right edge, when asked", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 100, top: 200, width: 300, height: 32 },
      panel: { width: 250, height: 100 },
      viewport,
      align: "end",
    });

    expect(pos.width).toBe(250);
    expect(pos.left).toBe(150); // 100 + 300 - 250
  });

  it("never leaves the viewport sideways", () => {
    const offRight = computeAnchoredPosition({
      anchor: { left: 900, top: 200, width: 300, height: 32 },
      panel: { width: 300, height: 100 },
      viewport,
    });
    expect(offRight.left).toBe(692); // 1000 - 300 - margin

    const offLeft = computeAnchoredPosition({
      anchor: { left: -40, top: 200, width: 300, height: 32 },
      panel: { width: 300, height: 100 },
      viewport,
    });
    expect(offLeft.left).toBe(8); // margin
  });

  it("caps the height to the room it actually has", () => {
    const pos = computeAnchoredPosition({
      anchor: { left: 100, top: 600, width: 300, height: 32 },
      panel: { width: 300, height: 600 },
      viewport,
    });

    expect(pos.placement).toBe("above");
    expect(pos.maxHeight).toBe(588); // 600 - gap - margin
    expect(pos.top).toBe(8); // clamped to the margin, the panel scrolls
  });
});
