import { flushSync, mount, unmount } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SignatureControl from "./SignatureControl.svelte";
import type { Field } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));

const STORED = "data:image/png;base64,U1RPUkVE";
const DRAWN = "data:image/png;base64,RFJBV04=";

// jsdom has no canvas: a 2D context that records what is drawn, whose pixels
// are opaque once anything was stroked, and a toDataURL that returns DRAWN.
let drawn: string[] = [];
let inked = false;
function fakeContext(canvas: HTMLCanvasElement): any {
  const rec = (name: string) => (..._: any[]) => { drawn.push(name); if (name === "stroke" || name === "fill") inked = true; };
  return {
    canvas, lineWidth: 1,
    setTransform: rec("setTransform"), save: rec("save"), restore: rec("restore"),
    beginPath: rec("beginPath"), moveTo: rec("moveTo"), lineTo: rec("lineTo"), arc: rec("arc"),
    quadraticCurveTo: rec("quadraticCurveTo"), stroke: rec("stroke"), fill: rec("fill"),
    drawImage: rec("drawImage"),
    clearRect: () => { drawn.push("clearRect"); inked = false; },
    getImageData: (_x: number, _y: number, w: number, h: number) => {
      const data = new Uint8ClampedArray(w * h * 4);
      if (inked) data[((h >> 1) * w + (w >> 1)) * 4 + 3] = 255;
      return { width: w, height: h, data };
    },
  };
}

beforeEach(() => {
  drawn = [];
  inked = false;
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(function (this: HTMLCanvasElement) { return fakeContext(this); } as any);
  vi.spyOn(HTMLCanvasElement.prototype, "toDataURL").mockImplementation(() => DRAWN);
});
afterEach(() => vi.restoreAllMocks());

function setup(initial: string | null, extra: Record<string, any> = {}) {
  const changes: any[] = [];
  const props = $state({
    field: { fieldname: "signed", fieldtype: "Signature", label: "Signed by" } as Field,
    value: initial as any,
    id: "f-signed",
    onchange: (v: any) => { changes.push(v); props.value = v; },
    ...extra,
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(SignatureControl, { target, props });
  flushSync();
  const q = (sel: string) => target.querySelector<HTMLElement>(sel);
  const click = (sel: string) => { q(sel)!.click(); flushSync(); };
  const pointer = (type: string, x: number, y: number) => {
    const e = new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true });
    Object.defineProperty(e, "pointerId", { value: 1 });
    q("canvas")!.dispatchEvent(e);
    flushSync();
  };
  const stroke = () => { pointer("pointerdown", 10, 10); pointer("pointermove", 30, 20); pointer("pointerup", 40, 25); };
  return { props, changes, target, q, click, stroke, done: () => { unmount(view); target.remove(); } };
}

describe("SignatureControl", () => {
  it("shows a stored signature as an image, read-only without buttons", () => {
    const t = setup(STORED, { readOnly: true });
    expect(t.q("img.signature-image")!.getAttribute("src")).toBe(STORED);
    expect(t.q("canvas")).toBe(null);
    expect(t.q("button")).toBe(null);
    t.done();
  });

  it("never puts a value that is not a PNG data URL in the image", () => {
    const t = setup("javascript:alert(1)", { readOnly: true });
    expect(t.q("img")).toBe(null);
    expect(t.target.textContent).toContain("Invalid signature");
    t.done();
    const empty = setup(null, { readOnly: true });
    expect(empty.q("canvas")).toBe(null);
    expect(empty.target.textContent).toContain("Not signed");
    empty.done();
  });

  it("commits the pad, cropped, when a stroke ends, and keeps the pad up", () => {
    const t = setup(null);
    expect(t.q("canvas")).not.toBe(null);
    t.stroke();
    expect(t.changes).toEqual([DRAWN]);
    expect(drawn).toContain("quadraticCurveTo");
    expect(drawn).toContain("drawImage");
    // the pad stays so the person can go on signing; the image is not shown
    expect(t.q("canvas")).not.toBe(null);
    expect(t.q("img")).toBe(null);
    t.done();
  });

  it("an empty pad commits nothing", () => {
    const t = setup(null);
    t.q("canvas")!.dispatchEvent(Object.assign(new MouseEvent("pointerup", { bubbles: true }), { pointerId: 1 }));
    flushSync();
    expect(t.changes).toEqual([]);
    t.done();
  });

  it("Clear empties the pad and the value", () => {
    const t = setup(null);
    t.stroke();
    t.click(".signature-clear");
    expect(t.changes).toEqual([DRAWN, null]);
    expect(drawn).toContain("clearRect");
    expect(t.q("canvas")).not.toBe(null);
    t.done();
  });

  it("Sign again opens an empty pad without clearing the value, until a stroke or Clear", () => {
    const t = setup(STORED);
    expect(t.q("img.signature-image")).not.toBe(null);
    t.click(".signature-again");
    expect(t.q("canvas")).not.toBe(null);
    expect(t.q("img")).toBe(null);
    expect(t.changes).toEqual([]);
    // the stored image is never drawn back onto the pad
    expect(drawn).not.toContain("drawImage");
    // Cancel goes back to the stored signature
    t.click(".signature-cancel");
    expect(t.q("img.signature-image")!.getAttribute("src")).toBe(STORED);
    t.click(".signature-again");
    t.stroke();
    expect(t.changes).toEqual([DRAWN]);
    expect(t.q(".signature-cancel")).toBe(null);
    t.done();
  });

  it("a new value from outside shows the image again", () => {
    const t = setup(null);
    t.props.value = STORED;
    flushSync();
    expect(t.q("img.signature-image")!.getAttribute("src")).toBe(STORED);
    t.done();
  });
});
