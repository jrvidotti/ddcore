import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import FileControl from "./FileControl.svelte";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string, a: any[] = []) => s.replace(/\{(\d+)\}/g, (_, i) => String(a[+i])) }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn() }));

function setup(extra: Record<string, any> = {}) {
  const busy: boolean[] = [];
  const props = $state({
    value: null as any,
    error: "",
    onchange: (v: any) => { props.value = v; },
    onerror: (m: string) => { props.error = m; },
    onbusychange: (b: boolean) => { busy.push(b); },
    ...extra,
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(FileControl, { target, props });
  flushSync();
  const input = () => target.querySelector<HTMLInputElement>('input[type="file"]')!;
  async function pick(file: File) {
    Object.defineProperty(input(), "files", { value: [file], configurable: true });
    input().dispatchEvent(new Event("change", { bubbles: true }));
    await vi.waitFor(() => { if (busy.length && busy[busy.length - 1]) throw new Error("still reading"); });
    await Promise.resolve();
    flushSync();
  }
  const button = (label: string) => [...target.querySelectorAll("button")].find((b) => b.textContent === label);
  return { props, busy, target, input, pick, button, done: () => { unmount(view); target.remove(); } };
}

const pfx = () => new File([new Uint8Array([0, 255, 128, 10])], "cert.pfx", { type: "application/x-pkcs12" });

describe("FileControl", () => {
  it("hands the chosen file to the script, busy while it reads", async () => {
    const t = setup({ accept: ".pfx,.p12" });
    expect(t.input().accept).toBe(".pfx,.p12");
    expect(t.target.textContent).toContain("Choose file");
    await t.pick(pfx());
    expect(t.props.value).toEqual({ name: "cert.pfx", size: 4, type: "application/x-pkcs12", base64: "AP+ACg==" });
    expect(t.busy).toEqual([true, false]);
    expect(t.target.textContent).toContain("cert.pfx");
    expect(t.target.textContent).toContain("Replace");
    t.done();
  });

  it("refuses a file over maxBytes without reading it", async () => {
    const t = setup({ maxBytes: 3 });
    await t.pick(pfx());
    expect(t.props.value).toBe(null);
    expect(t.props.error).toBe("File is too large (limit 3 B)");
    expect(t.busy).toEqual([]);
    t.done();
  });

  it("clears the error once a file fits", async () => {
    const t = setup({ maxBytes: 3 });
    await t.pick(pfx());
    await t.pick(new File(["ab"], "small.pfx"));
    expect(t.props.error).toBe("");
    expect(t.props.value.name).toBe("small.pfx");
    t.done();
  });

  it("removes the file", async () => {
    const t = setup();
    await t.pick(pfx());
    t.button("Remove")!.click();
    flushSync();
    expect(t.props.value).toBe(null);
    expect(t.target.textContent).toContain("Choose file");
    t.done();
  });

  it("offers nothing to choose when read-only", () => {
    const t = setup({ readOnly: true });
    expect(t.target.querySelector('input[type="file"]')).toBe(null);
    t.done();
  });
});
