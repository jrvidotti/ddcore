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
  /** The fields a `tree` option asked for, when it asked for any. */
  values?: Record<string, any>;
}

/** The `tree` option of `defineListView`: how a node is labelled and ordered. */
export interface TreeViewSettings {
  /** A template such as "{acronym} - {title}", or a function of the node's values. */
  title?: string | ((row: Record<string, any>) => string);
  /** Fields to fetch for a `title` function; a template's placeholders are fetched anyway. */
  fields?: string[];
  /** Replaces the default order (groups first, then the title), e.g. "title asc". */
  orderBy?: string;
}

const PLACEHOLDER = /\{(\w+)\}/g;

/**
 * The fields the view asks the server for: a template's placeholders plus
 * `fields`, and the DocType's title field whenever a label is composed, so a
 * function can read it without listing it. `id` is on every node already.
 */
export function treeFields(settings?: TreeViewSettings, titleField = ""): string[] {
  const out = new Set(settings?.fields ?? []);
  if (typeof settings?.title === "string") {
    for (const m of settings.title.matchAll(PLACEHOLDER)) out.add(m[1]);
  }
  if (settings?.title && titleField) out.add(titleField);
  out.delete("id");
  return [...out];
}

/** Separators left dangling at either end once a placeholder rendered empty. */
const DANGLING = /^[\s\-–—:|/·,]+|[\s\-–—:|/·,]+$/g;

/**
 * A node's label under `settings.title`. A template's empty placeholder renders
 * as nothing, taking the empty brackets and end separators it leaves with it,
 * so "{acronym} - {title}" reads "Delivery" without an acronym; a label that
 * ends up blank falls back to the node's title.
 */
export function treeLabel(node: TreeNode, settings?: TreeViewSettings): string {
  const title = settings?.title;
  if (!title) return node.title;
  const values: Record<string, any> = { id: node.id, ...node.values };
  let label: string;
  if (typeof title === "function") {
    label = String(title(values) ?? "");
  } else {
    let empty = false;
    label = title.replace(PLACEHOLDER, (_, f: string) => {
      const v = values[f] == null ? "" : String(values[f]);
      if (!v) empty = true;
      return v;
    });
    if (empty) label = label.replace(/\(\s*\)|\[\s*\]/g, "").replace(DANGLING, "");
  }
  return label.trim() || node.title;
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

/**
 * One step of "expand all": opens every group loaded so far and names the ones
 * whose children still have to be fetched. The view calls it again after each
 * batch arrives, one level deeper each time, until nothing is left to fetch.
 */
export function expandAll(state: TreeState): { state: TreeState; toLoad: string[] } {
  const expanded = new Set(state.expanded);
  const loading = new Set(state.loading);
  const toLoad: string[] = [];
  for (const node of state.nodes.values()) {
    if (!node.is_group) continue;
    expanded.add(node.id);
    if (node.children > 0 && !state.children.has(node.id) && !loading.has(node.id)) {
      loading.add(node.id);
      toLoad.push(node.id);
    }
  }
  return { state: { ...state, expanded, loading }, toLoad };
}

/** Closes every branch; what was loaded stays, so reopening costs no request. */
export function collapseAll(state: TreeState): TreeState {
  return { ...state, expanded: new Set() };
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
  const filters: any[] = [["is_group", "=", true]];
  if (doc.id && !doc.__islocal) {
    filters.push(["id", "!=", doc.id], ["id", "not descendants of", doc.id]);
  }
  return { filters };
}
