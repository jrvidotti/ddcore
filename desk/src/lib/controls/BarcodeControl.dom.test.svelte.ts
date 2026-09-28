import { flushSync, mount, unmount } from "svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import BarcodeControl from "./BarcodeControl.svelte";
import type { Field } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));

function setup(initial: string | null, options?: string, extra: Record<string, any> = {}) {
  const changes: any[] = [];
  const props = $state({
    field: { fieldname: "gtin", fieldtype: "Barcode", label: "GTIN", options } as Field,
    value: initial as any,
    id: "f-gtin",
    onchange: (v: any) => { changes.push(v); props.value = v; },
    ...extra,
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(BarcodeControl, { target, props });
  flushSync();
  const input = target.querySelector<HTMLInputElement>("input")!;
  const img = () => target.querySelector<HTMLImageElement>("img.barcode-preview");
  const type = (s: string) => { input.dispatchEvent(new FocusEvent("focus")); input.value = s; input.dispatchEvent(new Event("input", { bubbles: true })); flushSync(); };
  const blur = () => { input.dispatchEvent(new FocusEvent("blur")); flushSync(); };
  const key = (k: string) => { input.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true })); flushSync(); };
  const wait = (ms: number) => { vi.advanceTimersByTime(ms); flushSync(); };
  return { props, changes, target, input, img, type, blur, key, wait, done: () => { unmount(view); target.remove(); } };
}

afterEach(() => {
  vi.useRealTimers();
  delete (window as any).BarcodeDetector;
});

describe("BarcodeControl", () => {
  it("previews the stored value from the endpoint, a moment after it changes", () => {
    vi.useFakeTimers();
    const t = setup("400638133393", "EAN-13");
    expect(t.img()).toBe(null);
    t.wait(300);
    expect(t.img()!.getAttribute("src")).toBe("/api/barcode?symbology=EAN-13&value=400638133393");
    t.type("12");
    t.wait(100);
    expect(t.img()!.getAttribute("src")).toContain("value=400638133393");
    t.wait(250);
    expect(t.img()!.getAttribute("src")).toBe("/api/barcode?symbology=EAN-13&value=12");
    t.done();
  });

  it("says the barcode is invalid when the image fails, and shows nothing when empty", () => {
    vi.useFakeTimers();
    const t = setup("abc", "EAN-13");
    t.wait(300);
    t.img()!.dispatchEvent(new Event("error"));
    flushSync();
    expect(t.img()).toBe(null);
    expect(t.target.textContent).toContain("Invalid barcode");
    t.type("");
    t.wait(300);
    expect(t.img()).toBe(null);
    expect(t.target.textContent).not.toContain("Invalid barcode");
    t.done();
  });

  it("commits trimmed text on blur and on Enter, as a scanner types it", () => {
    const t = setup(null);
    t.type("  AB-1 ");
    expect(t.changes).toEqual([]);
    t.blur();
    expect(t.changes).toEqual(["AB-1"]);
    t.type("XY-2");
    t.key("Enter");
    expect(t.changes).toEqual(["AB-1", "XY-2"]);
    t.type(" ");
    t.blur();
    expect(t.changes.at(-1)).toBe(null);
    t.done();
  });

  it("has no preview in a filter or a grid cell", () => {
    vi.useFakeTimers();
    const t = setup("AB-1", undefined, { preview: false });
    t.wait(300);
    expect(t.img()).toBe(null);
    t.done();
  });

  it("offers the camera only where BarcodeDetector exists, and stops it after reading", async () => {
    const plain = setup(null);
    expect(plain.target.querySelector(".scan-btn")).toBe(null);
    plain.done();

    const stopTrack = vi.fn();
    const formats: string[][] = [];
    (window as any).BarcodeDetector = class {
      constructor(o: { formats: string[] }) { formats.push(o.formats); }
      async detect() { return [{ rawValue: "4006381333931" }]; }
    };
    Object.defineProperty(window, "isSecureContext", { value: true, configurable: true });
    Object.defineProperty(navigator, "mediaDevices", {
      value: { getUserMedia: vi.fn(async () => ({ getTracks: () => [{ stop: stopTrack }] })) }, configurable: true,
    });
    HTMLMediaElement.prototype.play = vi.fn(async () => {});

    const t = setup(null, "EAN-13");
    const btn = t.target.querySelector<HTMLButtonElement>(".scan-btn")!;
    expect(btn).not.toBe(null);
    btn.click();
    flushSync();
    expect(document.querySelector(".scan-overlay")).not.toBe(null);
    await vi.waitFor(() => expect(t.changes).toEqual(["4006381333931"]));
    flushSync();
    expect(formats).toEqual([["ean_13"]]);
    expect(stopTrack).toHaveBeenCalled();
    expect(document.querySelector(".scan-overlay")).toBe(null);
    t.done();
  });
});
