import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import TreeView from "./TreeView.svelte";
import { api } from "$lib/api";
import type { Meta } from "$lib/meta";

vi.mock("$lib/api", () => ({ api: { treeChildren: vi.fn() } }));
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
});
