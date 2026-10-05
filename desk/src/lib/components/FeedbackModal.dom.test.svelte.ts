import { flushSync, mount, unmount } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$lib/boot.svelte", () => ({
  __: (s: string, a: any[] = []) => s.replace(/\{(\d+)\}/g, (_, i) => String(a[+i])),
  boot: { data: { user: "ana@example.com", roles: ["All"], lang: "en", apps: [{ name: "core", title: "Core" }], site: { name: "Acme", version: "1.2.3", feedback: true } } },
}));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn(), toast: vi.fn() }));
vi.mock("$lib/format", () => ({ formatDatetime: (v: string) => v }));
vi.mock("$lib/api", () => ({
  api: {
    submitFeedback: vi.fn(async () => ({ id: "FB-1" })),
    myFeedback: vi.fn(async () => [
      { id: "FB-1", title: "Save fails", feedback_type: "Bug", status: "In Review", response: "Looking into it", creation: "2026-10-05 10:00:00" },
    ]),
  },
}));
vi.mock("$app/state", () => ({
  page: { url: new URL("http://localhost/app/crm/lead/L-1?tenant=acme"), params: { workspace: "crm", doctype: "lead", id: "L-1" } },
}));

import FeedbackModal from "./FeedbackModal.svelte";
import { closeFeedback, openFeedback } from "$lib/feedback.svelte";
import { api } from "$lib/api";
import { showError, toast } from "$lib/ui.svelte";

const URL_HREF = "http://localhost/app/crm/lead/L-1?tenant=acme";

let view: any;
let target: HTMLDivElement;

beforeEach(() => {
  vi.clearAllMocks();
  target = document.createElement("div");
  document.body.append(target);
  view = mount(FeedbackModal, { target });
  flushSync();
});

afterEach(() => {
  closeFeedback();
  flushSync();
  unmount(view);
  target.remove();
});

function open(preset?: Parameters<typeof openFeedback>[0]) {
  openFeedback(preset);
  flushSync();
}

const q = <T extends Element = HTMLElement>(sel: string) => target.querySelector<T>(sel) as T;
const button = (text: string) => [...target.querySelectorAll("button")].find((b) => b.textContent?.trim() === text) as HTMLButtonElement;
const submitButton = () => q<HTMLButtonElement>(".foot .btn.primary");
const labels = () => [...target.querySelectorAll(".field label")].map((l) => l.textContent?.replace("*", "").trim());

function type(sel: string, value: string) {
  const el = q<HTMLInputElement>(sel);
  el.value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  flushSync();
}

function fillRequired() {
  type("#feedback-field-title", "  Save fails ");
  type("#feedback-field-description", "Clicking save does nothing");
}

async function send() {
  submitButton().click();
  flushSync();
  await vi.waitFor(() => expect(api.submitFeedback).toHaveBeenCalled());
  return (api.submitFeedback as any).mock.calls[0] as [any, File[]];
}

describe("FeedbackModal", () => {
  it("renders nothing until opened", () => {
    expect(q(".modal")).toBe(null);
    open();
    expect(q(".modal")).not.toBe(null);
  });

  it("sends the page address by default and the context only when asked", () => {
    open();
    expect(q<HTMLInputElement>("#feedback-send-url").checked).toBe(true);
    expect(q<HTMLInputElement>("#feedback-send-context").checked).toBe(false);
    expect(target.textContent).toContain(URL_HREF);
  });

  it("shows the fields of the chosen type", () => {
    open();
    expect(labels()).toContain("Steps to reproduce");
    expect(labels()).toContain("Severity");
    button("Improvement").click();
    flushSync();
    expect(labels()).toContain("How it works today");
    expect(labels()).not.toContain("Steps to reproduce");
    button("Feature Request").click();
    flushSync();
    expect(labels()).toContain("Problem to solve");
    expect(button("Feature Request").getAttribute("aria-checked")).toBe("true");
  });

  it("does not send without a title and a description", () => {
    open();
    submitButton().click();
    flushSync();
    expect(api.submitFeedback).not.toHaveBeenCalled();
    expect(target.querySelectorAll(".err")).toHaveLength(2);
  });

  it("sends the type's fields and the page address, without the context", async () => {
    open();
    fillRequired();
    type("#feedback-field-steps_to_reproduce", " open, save ");
    const sev = q<HTMLSelectElement>("#feedback-field-severity");
    sev.value = "High";
    sev.dispatchEvent(new Event("change", { bubbles: true }));
    flushSync();
    const [data, files] = await send();
    expect(data).toEqual({
      feedback_type: "Bug", title: "Save fails", description: "Clicking save does nothing",
      severity: "High", steps_to_reproduce: "open, save", page_url: URL_HREF,
    });
    expect(files).toEqual([]);
  });

  it("leaves the address out when unchecked and adds the context when checked", async () => {
    open();
    fillRequired();
    q<HTMLInputElement>("#feedback-send-url").click();
    q<HTMLInputElement>("#feedback-send-context").click();
    flushSync();
    const [data] = await send();
    expect("page_url" in data).toBe(false);
    expect(data.context).toMatchObject({
      user: "ana@example.com", site: { name: "Acme", version: "1.2.3" }, apps: ["core"],
      path: "/app/crm/lead/L-1", workspace: "crm", doctype: "lead", id: "L-1",
    });
  });

  it("previews the context it would send", () => {
    open();
    button("See what will be sent").click();
    flushSync();
    expect(q("pre.context-preview").textContent).toContain('"user": "ana@example.com"');
  });

  it("takes pasted screenshots and dropped files, and sends them", async () => {
    open();
    const png = new File(["png"], "image.png", { type: "image/png" });
    const paste = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(paste, "clipboardData", {
      value: { items: [{ kind: "file", type: "image/png", getAsFile: () => png }, { kind: "string", type: "text/plain", getAsFile: () => null }] },
    });
    q("#feedback-field-description").dispatchEvent(paste);
    flushSync();
    expect(paste.defaultPrevented).toBe(true);

    const drop = new Event("drop", { bubbles: true, cancelable: true });
    Object.defineProperty(drop, "dataTransfer", { value: { files: [new File(["log"], "console.txt", { type: "text/plain" })], types: ["Files"] } });
    q(".dropzone").dispatchEvent(drop);
    flushSync();

    const names = [...target.querySelectorAll(".files .fname")].map((n) => n.textContent);
    expect(names).toEqual(["screenshot-1.png", "console.txt"]);

    fillRequired();
    const [, files] = await send();
    expect(files.map((f) => f.name)).toEqual(["screenshot-1.png", "console.txt"]);
    expect(files[0].type).toBe("image/png");
  });

  it("removes an attachment", () => {
    open();
    const drop = new Event("drop", { bubbles: true, cancelable: true });
    Object.defineProperty(drop, "dataTransfer", { value: { files: [new File(["a"], "a.txt"), new File(["b"], "b.txt")] } });
    q(".dropzone").dispatchEvent(drop);
    flushSync();
    (q('button[aria-label="Remove a.txt"]') as HTMLButtonElement).click();
    flushSync();
    expect([...target.querySelectorAll(".files .fname")].map((n) => n.textContent)).toEqual(["b.txt"]);
  });

  it("opens with the preset type and title", () => {
    open({ type: "Feature Request", title: "Dark mode" });
    expect(q<HTMLInputElement>("#feedback-field-title").value).toBe("Dark mode");
    expect(button("Feature Request").getAttribute("aria-checked")).toBe("true");
    expect(labels()).toContain("Expected benefit");
  });

  it("thanks, clears the form and shows what was sent before", async () => {
    open();
    fillRequired();
    await send();
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith("Thanks! Your feedback was sent.", expect.anything()));
    await vi.waitFor(() => { flushSync(); expect(target.querySelector(".mine li")).not.toBe(null); });
    expect(api.myFeedback).toHaveBeenCalled();
    const status = q(".mine .indicator");
    expect(status.textContent).toBe("In Review");
    expect(status.classList.contains("orange")).toBe(true);
    expect(target.textContent).toContain("Looking into it");
    // back on the form, it is blank again
    button("Send").click();
    flushSync();
    expect(q<HTMLInputElement>("#feedback-field-title").value).toBe("");
  });

  it("shows the error and keeps the draft when sending fails", async () => {
    (api.submitFeedback as any).mockRejectedValueOnce(new Error("boom"));
    open();
    fillRequired();
    await send();
    await vi.waitFor(() => expect(showError).toHaveBeenCalled());
    flushSync();
    expect(q<HTMLInputElement>("#feedback-field-title").value).toBe("  Save fails ");
  });

  it("closes on Escape", () => {
    open();
    q(".modal-bg").dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    flushSync();
    expect(q(".modal")).toBe(null);
  });
});
