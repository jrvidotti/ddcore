import { describe, it, expect } from "vitest";
import { emptyTree, mergeChildren, toggle, visibleRows, loadedParents, treeParentQuery, ROOT, type TreeNode } from "./tree-state";

const node = (id: string, parent = "", is_group = false, children = 0): TreeNode =>
  ({ id, title: id, parent, is_group, children });

function seeded() {
  let state = emptyTree();
  state = mergeChildren(state, ROOT, { nodes: [node("World", "", true, 2), node("Antarctica")], hasMore: false });
  return state;
}

describe("tree state", () => {
  it("renders the roots it was given", () => {
    expect(visibleRows(seeded()).map((r) => r.node.id)).toEqual(["World", "Antarctica"]);
  });

  it("asks for a branch the first time it opens, and not again", () => {
    let state = seeded();
    const first = toggle(state, "World");
    expect(first.needsLoad).toBe(true);
    expect(first.state.loading.has("World")).toBe(true);
    state = mergeChildren(first.state, "World", { nodes: [node("Brazil", "World", true, 1)], hasMore: false });
    expect(state.loading.has("World")).toBe(false);
    const closed = toggle(state, "World");
    expect(closed.needsLoad).toBe(false);
    const reopened = toggle(closed.state, "World");
    expect(reopened.needsLoad).toBe(false);
  });

  it("flattens the open branches depth-first, with their depth", () => {
    let state = seeded();
    state = toggle(state, "World").state;
    state = mergeChildren(state, "World", { nodes: [node("Brazil", "World", true, 1)], hasMore: false });
    state = toggle(state, "Brazil").state;
    state = mergeChildren(state, "Brazil", { nodes: [node("PR", "Brazil")], hasMore: true });
    expect(visibleRows(state).map((r) => [r.node.id, r.depth])).toEqual([
      ["World", 0], ["Brazil", 1], ["PR", 2], ["Antarctica", 0],
    ]);
    expect(state.hasMore.get("Brazil")).toBe(true);
  });

  it("hides the subtree of a closed node without forgetting it", () => {
    let state = seeded();
    state = toggle(state, "World").state;
    state = mergeChildren(state, "World", { nodes: [node("Brazil", "World")], hasMore: false });
    state = toggle(state, "World").state;
    expect(visibleRows(state).map((r) => r.node.id)).toEqual(["World", "Antarctica"]);
    expect(state.children.get("World")).toEqual(["Brazil"]);
  });

  it("only marks groups as expandable", () => {
    expect(visibleRows(seeded()).map((r) => r.expandable)).toEqual([true, false]);
  });

  it("refreshes the roots and every open branch, not the closed ones", () => {
    let state = seeded();
    state = toggle(state, "World").state;
    state = mergeChildren(state, "World", { nodes: [node("Brazil", "World", true)], hasMore: false });
    state = toggle(state, "Brazil").state;
    state = mergeChildren(state, "Brazil", { nodes: [node("PR", "Brazil")], hasMore: false });
    state = toggle(state, "Brazil").state; // closed again
    expect(loadedParents(state).sort()).toEqual([ROOT, "World"]);
  });
});

describe("parent picker query", () => {
  it("offers groups only", () => {
    expect(treeParentQuery({ id: "new-1", __islocal: true }).filters).toEqual([["is_group", "=", 1]]);
  });

  it("excludes the document itself and its descendants", () => {
    expect(treeParentQuery({ id: "Brazil" }).filters).toEqual([
      ["is_group", "=", 1], ["id", "!=", "Brazil"], ["id", "not descendants of", "Brazil"],
    ]);
  });
});
