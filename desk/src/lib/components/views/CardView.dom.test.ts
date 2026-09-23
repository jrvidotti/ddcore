import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import CardView from "./CardView.svelte";
import type { Meta } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, doctypeLabel: (s: string) => s }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "" }));

const meta = (imageField?: string) => ({
  doctype: {
    name: "Person", app: "base", label: "Person", idGeneration: {}, titleField: "person_name", imageField,
    fields: [{ fieldname: "person_name", fieldtype: "Data" }, { fieldname: "photo", fieldtype: "Attach Image" }],
  },
  permissions: {},
}) as unknown as Meta;

function render(m: Meta, rows: any[]) {
  const target = document.createElement("div");
  const view = mount(CardView, { target, props: { rows, meta: m, doctype: "Person", wsPrefix: "/app/HR", selected: new Set<string>(), onToggle: () => {} } });
  flushSync();
  return { target, view };
}

describe("CardView image", () => {
  it("shows the image lazily, and the title's initials when there is none", () => {
    const { target, view } = render(meta("photo"), [
      { id: "P-1", person_name: "Ana Souza", photo: "/private/files/ana.png" },
      { id: "P-2", person_name: "MARIA DA SILVA", photo: null },
    ]);
    const [first, second] = target.querySelectorAll("article");
    const img = first.querySelector("img.media") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe("/private/files/ana.png");
    expect(img.getAttribute("loading")).toBe("lazy");
    expect(second.querySelector("img")).toBeNull();
    expect(second.querySelector(".card-avatar")?.textContent).toBe("MS");
    unmount(view);
  });

  it("falls back to the avatar when the image fails to load", () => {
    const { target, view } = render(meta("photo"), [{ id: "P-1", person_name: "Ana Souza", photo: "/files/gone.png" }]);
    target.querySelector("img.media")!.dispatchEvent(new Event("error"));
    flushSync();
    expect(target.querySelector("img")).toBeNull();
    expect(target.querySelector(".card-avatar")?.textContent).toBe("AS");
    unmount(view);
  });

  it("has no picture slot when no image is configured", () => {
    const { target, view } = render(meta(), [{ id: "P-1", person_name: "Ana Souza" }]);
    expect(target.querySelector(".media")).toBeNull();
    unmount(view);
  });
});
