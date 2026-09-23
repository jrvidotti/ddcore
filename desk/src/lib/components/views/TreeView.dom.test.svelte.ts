import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import TreeView from "./TreeView.svelte";
import { api } from "$lib/api";
import type { Meta } from "$lib/meta";

vi.mock("$lib/api", () => ({ api: { treeChildren: vi.fn() } }));
vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn() }));

const meta = { doctype: { name: "Task Category", isTree: true, parentField: "parent_task_category" }, permissions: {} } as unknown as Meta;

async function settle() {
  for (let i = 0; i < 20; i++) {
    await new Promise((r) => setTimeout(r));
    flushSync();
  }
}

describe("TreeView reloads", () => {
  it("coalesces a burst of reloadKey bumps", async () => {
    const treeChildren = vi.mocked(api.treeChildren).mockImplementation(
      () => new Promise((r) => setTimeout(() => r({ nodes: [{ id: "A", title: "A", isGroup: false, children: 0 }], hasMore: false } as any))),
    );
    const props = $state({ meta, doctype: "Task Category", wsPrefix: "/app/Projects", reloadKey: 0 });
    const target = document.createElement("div");
    const view = mount(TreeView, { target, props });
    await settle();
    expect(treeChildren).toHaveBeenCalledTimes(1);

    // one list_update per imported row
    for (let i = 0; i < 100; i++) {
      props.reloadKey++;
      flushSync();
    }
    await settle();

    // the reload already running, plus one for everything that came in meanwhile
    expect(treeChildren).toHaveBeenCalledTimes(3);
    unmount(view);
  });
});
