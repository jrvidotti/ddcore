import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import TreeView from "./TreeView.svelte";
import { api } from "$lib/api";
import type { Meta } from "$lib/meta";

vi.mock("$lib/api", () => ({ api: { treeChildren: vi.fn(), treeSearch: vi.fn() } }));
vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn() }));

const meta = { doctype: { name: "Task Category", isTree: true, parentField: "parent_task_category" }, permissions: {} } as unknown as Meta;

describe("TreeView", () => {
  it("loads the root once and settles on an empty tree", async () => {
    const treeChildren = vi.mocked(api.treeChildren).mockResolvedValue({ nodes: [], hasMore: false });
    const target = document.createElement("div");
    const view = mount(TreeView, { target, props: { meta, doctype: "Task Category", wsPrefix: "/app/Projects" } });

    for (let i = 0; i < 10; i++) {
      await Promise.resolve();
      flushSync();
    }

    expect(treeChildren).toHaveBeenCalledTimes(1);
    expect(target.textContent).toContain("No records");
    unmount(view);
  });

  it("with a search text shows the matches inside their open branches", async () => {
    vi.mocked(api.treeChildren).mockClear();
    const treeSearch = vi.mocked(api.treeSearch).mockResolvedValue({
      nodes: [
        { id: "All", title: "All", parent: "", is_group: true, children: 2 },
        { id: "Shipping", title: "Shipping", parent: "All", is_group: false, children: 0, match: true },
      ],
      hasMore: true,
    });
    const target = document.createElement("div");
    const view = mount(TreeView, { target, props: { meta, doctype: "Task Category", wsPrefix: "/app/Projects", search: " ship " } });

    for (let i = 0; i < 10; i++) {
      await Promise.resolve();
      flushSync();
    }

    expect(treeSearch).toHaveBeenCalledWith("Task Category", "ship", expect.anything());
    expect(api.treeChildren).not.toHaveBeenCalled();
    expect([...target.querySelectorAll("a.title")].map((a) => [a.textContent, a.classList.contains("match")])).toEqual([["All", false], ["Shipping", true]]);
    expect(target.textContent).toContain("Showing the first matches only; refine the search");
    unmount(view);
  });
});
