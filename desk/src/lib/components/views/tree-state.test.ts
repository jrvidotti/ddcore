import { describe, it, expect } from "vitest";
import { emptyTree, mergeChildren, toggle, visibleRows, loadedParents, treeParentQuery, expandAll, collapseAll, treeFields, treeLabel, rememberTreeExpand, getRememberedTreeExpand, ROOT, type TreeNode } from "./tree-state";

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
    expect(treeParentQuery({ id: "new-1", __islocal: true }).filters).toEqual([["is_group", "=", true]]);
  });

  it("excludes the document itself and its descendants", () => {
    expect(treeParentQuery({ id: "Brazil" }).filters).toEqual([
      ["is_group", "=", true], ["id", "!=", "Brazil"], ["id", "not descendants of", "Brazil"],
    ]);
  });

  it("expands every group level by level, fetching only branches with unloaded children", () => {
    let state = mergeChildren(seeded(), ROOT, { nodes: [node("World", "", true, 2), node("Antarctica"), node("Empty", "", true, 0)], hasMore: false });
    const first = expandAll(state);
    expect(first.toLoad).toEqual(["World"]);
    expect([...first.state.expanded].sort()).toEqual(["Empty", "World"]);
    expect(first.state.loading.has("World")).toBe(true);

    state = mergeChildren(first.state, "World", { nodes: [node("Brazil", "World", true, 1), node("Chile", "World")], hasMore: false });
    const second = expandAll(state);
    expect(second.toLoad).toEqual(["Brazil"]);

    state = mergeChildren(second.state, "Brazil", { nodes: [node("PR", "Brazil")], hasMore: false });
    const third = expandAll(state);
    expect(third.toLoad).toEqual([]);
    expect(visibleRows(third.state).map((r) => [r.node.id, r.depth])).toEqual([
      ["World", 0], ["Brazil", 1], ["PR", 2], ["Chile", 1], ["Antarctica", 0], ["Empty", 0],
    ]);
  });

  it("does not ask again for a branch already being fetched", () => {
    const first = expandAll(seeded());
    expect(expandAll(first.state).toLoad).toEqual([]);
  });

  it("collapses every branch and keeps what it loaded", () => {
    let state = toggle(seeded(), "World").state;
    state = mergeChildren(state, "World", { nodes: [node("Brazil", "World")], hasMore: false });
    const closed = collapseAll(state);
    expect(visibleRows(closed).map((r) => r.node.id)).toEqual(["World", "Antarctica"]);
    expect(toggle(closed, "World").needsLoad).toBe(false);
  });
});

describe("tree labels", () => {
  const cat: TreeNode = { id: "Design", title: "Design", parent: "", is_group: false, children: 0, values: { acronym: "DSG", title: "Design" } };

  it("asks for a template's placeholders and the title field", () => {
    expect(treeFields({ title: "{acronym} - {title}" }, "title")).toEqual(["acronym", "title"]);
    expect(treeFields({ title: (r) => r.title, fields: ["acronym"] }, "title")).toEqual(["acronym", "title"]);
    expect(treeFields({ title: "{id}: {acronym}" })).toEqual(["acronym"]);
    expect(treeFields({ orderBy: "title asc" }, "title")).toEqual([]);
    expect(treeFields(undefined, "title")).toEqual([]);
  });

  it("composes a label from a template or a function", () => {
    expect(treeLabel(cat, { title: "{acronym} - {title}" })).toBe("DSG - Design");
    expect(treeLabel(cat, { title: "{title} ({acronym})" })).toBe("Design (DSG)");
    expect(treeLabel(cat, { title: (r) => `${r.id}/${r.acronym}` })).toBe("Design/DSG");
  });

  it("renders an empty placeholder as nothing, and a blank label as the title", () => {
    const bare = { ...cat, values: { acronym: null, title: "Design" } };
    expect(treeLabel(bare, { title: "{acronym} - {title}" })).toBe("Design");
    expect(treeLabel(bare, { title: "{title} ({acronym})" })).toBe("Design");
    expect(treeLabel(bare, { title: "{title} [{acronym}]" })).toBe("Design");
    expect(treeLabel(cat, { title: "- {acronym} -" })).toBe("- DSG -");
    expect(treeLabel({ ...cat, values: {} }, { title: "{acronym}" })).toBe("Design");
    expect(treeLabel(cat)).toBe("Design");
  });
});

describe("remembered expand mode", () => {
  const memory = () => {
    const m = new Map<string, string>();
    return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
  };

  it("round-trips per DocType", () => {
    const store = memory();
    expect(getRememberedTreeExpand("Task Category", store)).toBe("");
    rememberTreeExpand("Task Category", "expanded", store);
    expect(getRememberedTreeExpand("Task Category", store)).toBe("expanded");
    expect(getRememberedTreeExpand("Territory", store)).toBe("");
    rememberTreeExpand("Task Category", "collapsed", store);
    expect(getRememberedTreeExpand("Task Category", store)).toBe("collapsed");
  });

  it("ignores an unknown value and a storage that throws", () => {
    const store = memory();
    store.setItem("ddcore_tree_expand_Task Category", "sideways");
    expect(getRememberedTreeExpand("Task Category", store)).toBe("");
    const broken = { getItem: () => { throw new Error("denied"); }, setItem: () => { throw new Error("denied"); } };
    expect(() => rememberTreeExpand("Task Category", "expanded", broken)).not.toThrow();
    expect(getRememberedTreeExpand("Task Category", broken)).toBe("");
  });
});
