// The runtime that app client bundles import as "@ddcore/desk-sdk" (resolved
// by the Go bundler to window.__ddcoreDesk). Keep it in sync with
// packages/desk-sdk/src/index.ts, which holds the public types.
import { api } from "./api";
import { registerForm, type FormHandlers, FormController } from "./form.svelte";
import { dialog, toast, confirm, prompt, showError } from "./ui.svelte";
import { __ } from "./boot.svelte";
import { formatCurrency, formatDate, formatNumber, formatValue, roundCurrency, statusColor } from "./format";
import { getMeta } from "./meta";
import { addDays, addMonths, monthEnd, monthStart, today } from "./datetime";
import { getRememberedWorkspace } from "./components/sidebar-workspace";

export type BaseDoc = Record<string, any>;
export type DeskViewMode = "list" | "calendar" | "cards" | "kanban" | "gantt";

export interface CalendarViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: Date or Datetime field to plot records on the calendar */
  field: keyof T & string;
  /** Optional: Datetime or Date field for range spans */
  endField?: keyof T & string;
  /** Field shown as label inside the calendar chip (defaults to titleField or name) */
  titleField?: keyof T & string;
  /** Field determining chip color (e.g. "status", uses optionColors automatically) */
  colorField?: keyof T & string;
}

export interface CardViewOptions<T extends BaseDoc = BaseDoc> {
  title?: keyof T & string;
  subtitle?: keyof T & string;
  dateField?: keyof T & string;
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
  badges?: (row: T) => { label: string; color: string }[] | null | undefined;
}

export interface KanbanViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: the Select field whose values are the columns; dragging a card writes it. */
  field: keyof T & string;
  /** Columns in order; defaults to the field's options. */
  columns?: string[];
  /** Field shown as the card title (defaults to titleField or name) */
  titleField?: keyof T & string;
  /** Field shown under the title */
  subtitleField?: keyof T & string;
  /** Field determining the card's indicator color (defaults to the column field) */
  colorField?: keyof T & string;
}

export interface GanttViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: Date or Datetime field where a bar starts */
  startField: keyof T & string;
  /** Required: Date or Datetime field where a bar ends; rows without one are not drawn */
  endField: keyof T & string;
  /** Field shown as the row label (defaults to titleField or name) */
  titleField?: keyof T & string;
  /** Field determining bar color (e.g. "status", uses optionColors automatically) */
  colorField?: keyof T & string;
  /** Numeric field from 0 to 100 drawn as progress inside the bar */
  progressField?: keyof T & string;
}

export interface ListViewOptions<T extends BaseDoc = BaseDoc> {
  /** Allowed views for this DocType; defaults to ["list", "cards"] plus "calendar", "kanban" and "gantt" for each one configured */
  views?: DeskViewMode[];
  calendar?: CalendarViewOptions<T>;
  card?: CardViewOptions<T>;
  kanban?: KanbanViewOptions<T>;
  gantt?: GanttViewOptions<T>;
  columns?: (keyof T & string)[];
  filters?: Partial<Record<keyof T & string, any>>;
  orderBy?: string;
  pageSize?: number;
  formatters?: Partial<Record<keyof T & string, (value: any, row: T) => string>>;
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
  docstatusFilter?: boolean;
  modifiedColumn?: boolean;
  fields?: (keyof T & string)[];
  badges?: (row: T) => { label: string; color: string }[] | null | undefined;
  filterOptions?: Partial<Record<keyof T & string, { value: string; label: string; filters: [string, string, any][] }[]>>;
}

const listRegistry = new Map<string, any>();

export const deskSDK = {
  defineForm(doctype: string, handlers: FormHandlers) { registerForm(doctype, handlers); },
  defineListView(doctype: string, opts: any) { listRegistry.set(doctype, opts); },
  listSettings(doctype: string) { return listRegistry.get(doctype); },
  FormController,
  ddcore: {
    _: __,
    __,
    call: (path: string, args?: any) => api.call(path, args),
    api,
    notifications: api.notifications,
    assignments: api.assignments,
    shares: api.shares,
    search: { global: (txt: string, limit?: number) => api.globalSearch(txt, limit) },
    db: {
      getValue: async (doctype: string, name: string | Record<string, any>, field: string | string[]) => {
        const fields = Array.isArray(field) ? field : [field];
        const rows = await api.list(doctype, { filters: typeof name === "string" ? { name } : name, fields, limit: 1 });
        const row = rows[0];
        if (!row) return null;
        return Array.isArray(field) ? row : row[field];
      },
      getList: (doctype: string, args: any = {}) => api.list(doctype, { filters: args.filters, fields: args.fields, order_by: args.orderBy, limit: args.limit, start: args.start }),
      count: (doctype: string, filters?: any) => api.count(doctype, filters),
      getSingle: (doctype: string) => api.getSingle(doctype),
      getDoc: (doctype: string, name: string) => api.getDoc(doctype, name),
      setValue: (doctype: string, name: string, values: any) => api.update(doctype, name, values),
      insert: (doc: any) => api.insert(doc.doctype, doc),
    },
    ui: { Dialog: dialog, dialog, msgprint: (m: string, o: any = {}) => toast(m, { title: o.title, indicator: o.indicator || "blue" }), alert: (m: string) => toast(m, { indicator: "blue" }), confirm, prompt, showError, toast },
    // roundCurrency is here so a form script that totals a grid rounds the way
    // the server is about to store it, rather than the way toFixed happens to
    format: { currency: formatCurrency, date: formatDate, number: formatNumber, value: formatValue, roundCurrency, statusColor },
    // civil dates with the same semantics as `ddcore.utils` no servidor (ver $lib/datetime)
    datetime: { today: () => today(), addMonths, addDays, monthStart, monthEnd },
    meta: getMeta,
    route: (path: string) => import("$app/navigation").then((n) => n.goto(path)),
    setRoute: (...parts: string[]) => {
      const rem = getRememberedWorkspace();
      const first = parts[0] || "";
      const path = rem && first !== rem ? `/app/${encodeURIComponent(rem)}/` + parts.map(encodeURIComponent).join("/") : "/app/" + parts.map(encodeURIComponent).join("/");
      return import("$app/navigation").then((n) => n.goto(path));
    },
  },
};

export function installDeskSDK() {
  (window as any).__ddcoreDesk = deskSDK;
  (window as any).ddcore = deskSDK.ddcore;
  (window as any).__ = __;
}

const loadedIncludes = new Set<string>();
/** Loads every app's desk include bundle once (client/*.ts declared in ddcore.app.ts). */
export async function loadAppIncludes(apps: { name: string; hasDeskInclude: boolean }[], version: number) {
  (window as any).__ddcoreLoaded = version;
  for (const a of apps) {
    if (!a.hasDeskInclude || loadedIncludes.has(a.name)) continue;
    loadedIncludes.add(a.name);
    try { await import(/* @vite-ignore */ `/assets/apps/${a.name}/desk.js?v=${version}`); } catch (e) { console.error("desk include", a.name, e); }
  }
}
