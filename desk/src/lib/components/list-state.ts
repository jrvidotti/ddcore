import type { Field } from "$lib/meta";

export const listPageSizes = [20, 50, 100, 500] as const;

export interface ListUrlState {
  filters: Record<string, any>;
  search: string;
  docstatusFilter: string;
  orderBy: string;
  page: number;
  pageSize: number;
  view?: string;
}

type ListViewSettings = { views?: string[]; calendar?: { field?: string }; kanban?: { field?: string }; gantt?: { startField?: string; endField?: string } };

function viewIsConfigured(view: string, settings?: ListViewSettings, isTree?: boolean): boolean {
  if (view === "calendar") return !!settings?.calendar?.field;
  if (view === "kanban") return !!settings?.kanban?.field;
  if (view === "gantt") return !!(settings?.gantt?.startField && settings.gantt.endField);
  if (view === "tree") return !!isTree;
  return true;
}

export function resolveAllowedViews(settings?: ListViewSettings, isTree?: boolean): string[] {
  if (settings?.views && settings.views.length > 0) {
    // An explicit list still has to meet the same configuration requirements as
    // the derived one, so a view never gets a button that silently does nothing.
    return settings.views.filter((view) => viewIsConfigured(view, settings, isTree));
  }
  // A hierarchy reads as a hierarchy first: the tree comes before the flat
  // list, and so is what a DocType with `isTree` opens on.
  const views = isTree ? ["tree", "list"] : ["list"];
  if (settings?.calendar?.field) views.push("calendar");
  if (settings?.kanban?.field) views.push("kanban");
  if (settings?.gantt?.startField && settings.gantt.endField) views.push("gantt");
  views.push("cards");
  return views;
}

export function resolveActiveView(
  allowedViews: string[],
  urlView?: string | null,
  storedView?: string | null,
  isMobile?: boolean,
): string {
  if (urlView && allowedViews.includes(urlView)) {
    return urlView;
  }
  if (storedView && allowedViews.includes(storedView)) {
    return storedView;
  }
  if (isMobile) {
    // A tree is already a phone-shaped layout: one column of indented rows.
    if (allowedViews.includes("tree")) return "tree";
    if (allowedViews.includes("cards")) return "cards";
  }
  return allowedViews[0] || "list";
}

function filterValue(field: Field, value: string): any {
  if (field.fieldtype === "Check") return value === "true" || value === "1";
  if (["Int", "Float", "Currency", "Percent"].includes(field.fieldtype)) {
    const n = Number(value);
    return Number.isFinite(n) ? n : null;
  }
  return value;
}

function positiveInt(value: string | null, fallback: number): number {
  const n = Number(value);
  return Number.isInteger(n) && n > 0 ? n : fallback;
}

export function listStateFromSearchParams(params: URLSearchParams, fields: Field[], defaults: ListUrlState): ListUrlState {
  const filters = { ...defaults.filters };
  for (const field of fields) {
    if (!field.fieldname || !params.has(field.fieldname)) continue;
    const value = filterValue(field, params.get(field.fieldname) || "");
    if (value === null || value === "") delete filters[field.fieldname];
    else filters[field.fieldname] = value;
  }
  const requestedSize = positiveInt(params.get("page_size"), defaults.pageSize);
  return {
    filters,
    search: params.get("q") ?? defaults.search,
    docstatusFilter: params.get("docstatus") ?? defaults.docstatusFilter,
    orderBy: params.get("order_by") ?? defaults.orderBy,
    page: positiveInt(params.get("page"), 1),
    pageSize: listPageSizes.includes(requestedSize as (typeof listPageSizes)[number]) ? requestedSize : defaults.pageSize,
    view: params.get("view") ?? undefined,
  };
}

export function listStateToSearchParams(state: ListUrlState, fields: Field[]): URLSearchParams {
  const params = new URLSearchParams();
  for (const field of fields) {
    const name = field.fieldname;
    const value = name ? state.filters[name] : undefined;
    if (name && value !== null && value !== undefined && value !== "") params.set(name, String(value));
  }
  if (state.search) params.set("q", state.search);
  if (state.docstatusFilter) params.set("docstatus", state.docstatusFilter);
  if (state.orderBy) params.set("order_by", state.orderBy);
  if (state.page > 1) params.set("page", String(state.page));
  params.set("page_size", String(state.pageSize));
  if (state.view && state.view !== "list") params.set("view", state.view);
  return params;
}

export function clearListFilters(state: ListUrlState): ListUrlState {
  return { ...state, filters: {}, search: "", docstatusFilter: "", page: 1 };
}
