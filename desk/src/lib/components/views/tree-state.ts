/**
 * The tree view's state, kept out of the component so it can be tested without
 * a DOM: which nodes are loaded, which are open, and what a depth-first walk of
 * the open ones looks like.
 *
 * The empty string is the key of the roots, which is also what the API calls
 * the parent of a root.
 */
export interface TreeNode {
  id: string;
  title: string;
  parent: string;
  is_group: boolean;
  /** Readable children, counted by the server. */
  children: number;
}

export interface TreeChildrenResult {
  nodes: TreeNode[];
  hasMore: boolean;
}

export interface TreeState {
  nodes: Map<string, TreeNode>;
  children: Map<string, string[]>;
  expanded: Set<string>;
  loading: Set<string>;
  hasMore: Map<string, boolean>;
}

export interface TreeRow {
  node: TreeNode;
  depth: number;
  expandable: boolean;
  expanded: boolean;
  loading: boolean;
}

export const ROOT = "";

export function emptyTree(): TreeState {
  return { nodes: new Map(), children: new Map(), expanded: new Set(), loading: new Set(), hasMore: new Map() };
}

/** Replaces one level with what the server just answered. */
export function mergeChildren(state: TreeState, parent: string, res: TreeChildrenResult): TreeState {
  const nodes = new Map(state.nodes);
  for (const node of res.nodes) nodes.set(node.id, node);
  const children = new Map(state.children);
  children.set(parent, res.nodes.map((n) => n.id));
  const hasMore = new Map(state.hasMore);
  hasMore.set(parent, res.hasMore);
  const loading = new Set(state.loading);
  loading.delete(parent);
  return { ...state, nodes, children, hasMore, loading };
}

/**
 * Opens or closes a node. `needsLoad` is true when opening it for the first
 * time, which is what keeps the view to one request per branch.
 */
export function toggle(state: TreeState, id: string): { state: TreeState; needsLoad: boolean } {
  const expanded = new Set(state.expanded);
  if (expanded.has(id)) {
    expanded.delete(id);
    return { state: { ...state, expanded }, needsLoad: false };
  }
  expanded.add(id);
  const needsLoad = !state.children.has(id);
  const loading = new Set(state.loading);
  if (needsLoad) loading.add(id);
  return { state: { ...state, expanded, loading }, needsLoad };
}

export function markLoading(state: TreeState, parent: string): TreeState {
  const loading = new Set(state.loading);
  loading.add(parent);
  return { ...state, loading };
}

/** The open part of the tree, flattened depth-first into rows to render. */
export function visibleRows(state: TreeState): TreeRow[] {
  const out: TreeRow[] = [];
  const walk = (parent: string, depth: number) => {
    for (const id of state.children.get(parent) || []) {
      const node = state.nodes.get(id);
      if (!node) continue;
      const expandable = node.is_group;
      const expanded = state.expanded.has(id);
      out.push({ node, depth, expandable, expanded, loading: state.loading.has(id) });
      if (expanded) walk(id, depth + 1);
    }
  };
  walk(ROOT, 0);
  return out;
}

/**
 * The levels to fetch again after a change somewhere in the tree: the roots
 * plus every open branch. A level nobody has opened is not worth a request.
 */
export function loadedParents(state: TreeState): string[] {
  return [...state.children.keys()].filter((parent) => parent === ROOT || state.expanded.has(parent));
}

/**
 * The filters for the parent picker: only a group can take children, a
 * document is not its own parent, and its own descendants are not either —
 * the same three rules the server enforces on save.
 */
export function treeParentQuery(doc: { id?: string; __islocal?: boolean }): { filters: any[] } {
  const filters: any[] = [["is_group", "=", 1]];
  if (doc.id && !doc.__islocal) {
    filters.push(["id", "!=", doc.id], ["id", "not descendants of", doc.id]);
  }
  return { filters };
}
