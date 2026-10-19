import { flushSync, mount, unmount } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("$lib/boot.svelte", () => ({
  __: (s: string) => s,
  boot: { data: { lang: "en-GB", user: "ana@example.com", site: { timezone: "UTC" }, doctypes: {} } },
}));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn(), toast: vi.fn() }));
vi.mock("$lib/api", () => ({ api: { linkSearch: vi.fn(async () => [{ id: "bia@example.com" }]) } }));
vi.mock("$lib/meta", () => ({ getMeta: vi.fn(async () => ({ permissions: {} })) }));
vi.mock("$app/state", () => ({ page: { params: {} } }));
vi.mock("$lib/components/sidebar-workspace", () => ({ getRememberedWorkspace: () => "" }));

globalThis.ResizeObserver ??= class { observe() {} disconnect() {} } as any;
Element.prototype.scrollIntoView ??= () => {};

import AssignModal from "./AssignModal.svelte";
import type { AssignArgs } from "$lib/api";
import { showError } from "$lib/ui.svelte";

let view: any;
let target: HTMLDivElement;
let onsubmit: Mock<(args: AssignArgs) => Promise<void>>;
let onclose: Mock<() => void>;

function setup(required: "assignee" | "description", assignee?: string) {
  onsubmit = vi.fn(async (_: AssignArgs) => {});
  onclose = vi.fn(() => {});
  target = document.createElement("div");
  document.body.append(target);
  view = mount(AssignModal, {
    target,
    props: { assignee, title: "Assign document", icon: "user-plus", required, submitLabel: "Assign", busyLabel: "Assigning...", onsubmit, onclose },
  });
  flushSync();
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  unmount(view);
  target.remove();
});

const q = <T extends Element = HTMLElement>(sel: string) => target.querySelector<T>(sel) as T;
const submitButton = () => q<HTMLButtonElement>(".foot .btn.primary");
const settle = async () => { await vi.waitFor(() => {}); flushSync(); };

function type(sel: string, value: string) {
  const el = q<HTMLInputElement | HTMLTextAreaElement>(sel);
  el.value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  flushSync();
}

async function pickAssignee() {
  q("#assignee-input").dispatchEvent(new FocusEvent("focus"));
  await vi.waitFor(() => expect(target.querySelector('[role="option"]')).toBeTruthy());
  q('[role="option"]').dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
  flushSync();
}

describe("AssignModal", () => {
  it("shows the title and marks the assignee as the required field when assigning", () => {
    setup("assignee");
    expect(q("[role=dialog]").textContent).toContain("Assign document");
    expect(q('label[for="assignee-input"]').classList.contains("reqd")).toBe(true);
    expect(q('label[for="assign-description"]').classList.contains("reqd")).toBe(false);
  });

  it("marks the description as the required field for a standalone task", () => {
    setup("description");
    expect(q('label[for="assignee-input"]').classList.contains("reqd")).toBe(false);
    expect(q('label[for="assign-description"]').classList.contains("reqd")).toBe(true);
  });

  it("puts the focus in the description of a standalone task, and leaves it on the dialog when assigning", () => {
    setup("description");
    expect(document.activeElement).toBe(q("#assign-description"));
    unmount(view);
    target.remove();
    setup("assignee");
    expect(document.activeElement).toBe(q("[role=dialog]"));
  });

  it("offers the priorities from TODO_PRIORITIES, Medium selected", () => {
    setup("assignee");
    const select = q<HTMLSelectElement>("#assign-priority");
    expect([...select.options].map((o) => o.value)).toEqual(["Low", "Medium", "High", "Urgent"]);
    expect(select.value).toBe("Medium");
  });

  it("keeps Assign disabled until an assignee is chosen", async () => {
    setup("assignee");
    expect(submitButton().disabled).toBe(true);
    await pickAssignee();
    expect(submitButton().disabled).toBe(false);
  });

  it("keeps Save disabled until the description has text", () => {
    setup("description");
    expect(submitButton().disabled).toBe(true);
    type("#assign-description", "   ");
    expect(submitButton().disabled).toBe(true);
    type("#assign-description", "Call the client");
    expect(submitButton().disabled).toBe(false);
  });

  it("submits the assignee, due date, priority and trimmed description, then closes", async () => {
    setup("assignee");
    await pickAssignee();
    type("#assign-due-date", "15/03/2026");
    q("#assign-due-date").dispatchEvent(new FocusEvent("blur"));
    q<HTMLSelectElement>("#assign-priority").value = "High";
    q("#assign-priority").dispatchEvent(new Event("change", { bubbles: true }));
    type("#assign-description", "  Review the draft  ");
    flushSync();

    submitButton().click();
    await settle();

    expect(onsubmit).toHaveBeenCalledWith({
      allocated_to: "bia@example.com",
      date: "2026-03-15",
      priority: "High",
      description: "Review the draft",
    });
    expect(onclose).toHaveBeenCalledOnce();
  });

  it("starts on the given assignee, so a task can be submitted for them at once", async () => {
    setup("description", "ana@example.com");
    type("#assign-description", "Water the plants");
    submitButton().click();
    await settle();

    expect(onsubmit).toHaveBeenCalledWith(expect.objectContaining({ allocated_to: "ana@example.com" }));
  });

  it("submits a bare task without a date or an assignee", async () => {
    setup("description");
    type("#assign-description", "Water the plants");
    submitButton().click();
    await settle();

    expect(onsubmit).toHaveBeenCalledWith({ allocated_to: "", date: undefined, priority: "Medium", description: "Water the plants" });
    expect(onclose).toHaveBeenCalledOnce();
  });

  it("stays open and reports the error when the callback fails", async () => {
    setup("description");
    const err = new Error("nope");
    onsubmit.mockRejectedValueOnce(err);
    type("#assign-description", "Water the plants");
    submitButton().click();
    await settle();

    expect(showError).toHaveBeenCalledWith(err);
    expect(onclose).not.toHaveBeenCalled();
    expect(submitButton().disabled).toBe(false);
  });
});
