import { render } from "svelte/server";
import { describe, expect, it, vi } from "vitest";
import LinkControl from "./LinkControl.svelte";
import type { Field } from "$lib/meta";

vi.mock("$lib/api", () => ({ api: { linkSearch: vi.fn() } }));
vi.mock("$lib/boot.svelte", () => ({
  boot: { data: { doctypes: { Pessoa: { label: "Pessoa" } } } },
  // no catalogue here: a key is its own English text, with {0} filled in
  __: (key: string, args: unknown[] = []) => key.replace(/\{(\d+)\}/g, (_, i) => String(args[Number(i)])),
}));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "Ana", setLinkTitle: vi.fn() }));
vi.mock("$app/state", () => ({ page: { params: {} } }));
vi.mock("$lib/components/sidebar-workspace", () => ({ getRememberedWorkspace: () => "" }));

const field = { fieldname: "proprietario", fieldtype: "Link", options: "Pessoa" } as Field;

describe("LinkControl", () => {
  it("renders a clear action beside the open-record shortcut for an editable selected link", () => {
    const { body } = render(LinkControl, { props: { field, value: "PES-00001", onchange: vi.fn() } });

    expect(body).toContain('class="clear"');
    expect(body).toContain('aria-label="Clear Pessoa (PES-00001)"');
    expect(body).toContain('class="open"');
    expect(body).toContain('title="Open Pessoa (PES-00001)"');
  });
});
