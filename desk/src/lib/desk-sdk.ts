// The runtime that app client bundles import as "@ddcore/desk-sdk" (resolved
// by the Go bundler to window.__ddcoreDesk). Keep it in sync with
// packages/desk-sdk/src/index.ts, which holds the public types.
import { seg } from "./routes";
import { api } from "./api";
import { subscribe, unsubscribe } from "./events";
import { registerForm, type FormHandlers, FormController } from "./form.svelte";
import { dialog, toast, confirm, prompt, showError } from "./ui.svelte";
import { __, boot, hasRole } from "./boot.svelte";
import { formatCurrency, formatDate, formatNumber, formatValue, roundCurrency, statusColor } from "./format";
import { getMeta, type Field } from "./meta";
import { addDays, addMonths, dateDiff, monthDiff, monthEnd, monthStart, today } from "./datetime";
import { getRememberedWorkspace } from "./components/sidebar-workspace";
import { refreshPendingCount } from "./assignments.svelte";
import { openFeedback, type FeedbackPreset } from "./feedback.svelte";


function withPendingCount<T>(result: T): T {
  void refreshPendingCount();
  return result;
}

export type BaseDoc = Record<string, any>;
export type DeskViewMode = "list" | "calendar" | "cards" | "kanban" | "gantt" | "tree";

export interface CalendarNewOption {
  /** DocType the day's **+** creates a record in */
  doctype: string;
  /** Its Date or Datetime field set to the day */
  field: string;
  /** Optional: a second field set to the same day (a Datetime one to its last minute), e.g. an end date */
  endField?: string;
  /** Menu entry, shown as given — wrap it in `__()` (defaults to the DocType's label) */
  label?: string;
}

export interface CalendarViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: Date or Datetime field to plot records on the calendar */
  field: keyof T & string;
  /** Optional: Datetime or Date field for range spans */
  endField?: keyof T & string;
  /** Field shown as label inside the calendar chip (defaults to titleField or id) */
  titleField?: keyof T & string;
  /** Field determining chip color (e.g. "status", uses optionColors automatically) */
  colorField?: keyof T & string;
  /**
   * What the **+** in a day's corner creates, in place of a record of this DocType with `field`
   * set to the day. One option links straight to it; several open a menu. An option shows only
   * when the user may create its DocType, and the **+** hides when none remains.
   */
  newOptions?: CalendarNewOption[];
}

export interface CardViewOptions<T extends BaseDoc = BaseDoc> {
  title?: keyof T & string;
  subtitle?: keyof T & string;
  dateField?: keyof T & string;
  image?: keyof T & string;
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
  badges?: (row: T) => { label: string; color: string }[] | null | undefined;
}

export interface KanbanViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: the Select field whose values are the columns; dragging a card writes it. */
  field: keyof T & string;
  /** Columns in order; defaults to the field's options. */
  columns?: string[];
  /** Field shown as the card title (defaults to titleField or id) */
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
  /** Field shown as the row label (defaults to titleField or id) */
  titleField?: keyof T & string;
  /** Field determining bar color (e.g. "status", uses optionColors automatically) */
  colorField?: keyof T & string;
  /** Numeric field from 0 to 100 drawn as progress inside the bar */
  progressField?: keyof T & string;
}

export interface TreeViewOptions<T extends BaseDoc = BaseDoc> {
  /**
   * The node label: a template such as "{acronym} - {title}" (its placeholders
   * are fetched; an empty one renders as nothing, with the empty brackets and
   * end separators it leaves) or a function of the row, which sees `id`, the
   * title field and `fields`. Defaults to the title field.
   */
  title?: string | ((row: T) => string);
  /** Extra fields a `title` function reads. */
  fields?: (keyof T & string)[];
  /** Order within each level, e.g. "title asc"; replaces the default groups-first order. */
  orderBy?: string;
}

export interface ListActionContext<T extends BaseDoc = BaseDoc> {
  doctype: string;
  rows: T[];
  refresh(): void;
}

export interface ListAction<T extends BaseDoc = BaseDoc> {
  label: string;
  condition?: (row: T) => boolean;
  primary?: boolean;
  onClick(ids: string[], list: ListActionContext<T>): any;
}

export interface ListToolbarContext {
  doctype: string;
  filters: [string, string, any][];
  refresh(): void;
}

export interface ListToolbarAction {
  label: string;
  condition?: () => boolean;
  primary?: boolean;
  onClick(list: ListToolbarContext): any;
}

export interface ListSummaryCard {
  label: string;
  aggregate?: string;
  filters?: [string, string, any][] | Record<string, any>;
  datatype?: string;
  indicator?: "red" | "green" | "orange" | "blue";
}

export interface ListViewOptions<T extends BaseDoc = BaseDoc> {
  /** Allowed views for this DocType; defaults to ["list", "cards"] plus "calendar", "kanban" and "gantt" for each one configured */
  views?: DeskViewMode[];
  calendar?: CalendarViewOptions<T>;
  card?: CardViewOptions<T>;
  kanban?: KanbanViewOptions<T>;
  gantt?: GanttViewOptions<T>;
  tree?: TreeViewOptions<T>;
  columns?: (keyof T & string)[];
  filters?: Partial<Record<keyof T & string, any>>;
  orderBy?: string;
  pageSize?: number;
  formatters?: Partial<Record<keyof T & string, (value: any, row: T) => string>>;
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
  docstatusFilter?: boolean;
  filtersCollapsed?: boolean;
  modifiedColumn?: boolean;
  idColumn?: boolean;
  plainLinks?: (keyof T & string)[];
  fields?: (keyof T & string)[];
  badges?: (row: T) => { label: string; color: string }[] | null | undefined;
  filterOptions?: Partial<Record<keyof T & string, { value: string; label: string; filters: [string, string, any][] }[]>>;
  actions?: ListAction<T>[];
  toolbarActions?: ListToolbarAction[];
  summary?: ListSummaryCard[];
}

/** A card an app adds to the profile page; see `ProfileSection` in packages/desk-sdk. */
export interface ProfileSection<D = any> {
  id: string;
  title: string;
  load?: () => D | null | undefined | Promise<D | null | undefined>;
  info?: (data: D) => { label: string; value: any }[];
  description?: string;
  fields?: Field[];
  primaryLabel?: string;
  submit?: (values: Record<string, any>, data: D) => any;
  successMessage?: string;
}

const listRegistry = new Map<string, any>();
// A list, as several apps may each add a card; includes finish loading before
// the desk renders, so the profile page reads it once.
const profileRegistry: ProfileSection[] = [];

export const deskSDK = {
  defineForm(doctype: string, handlers: FormHandlers) { registerForm(doctype, handlers); },
  defineListView(doctype: string, opts: any) { listRegistry.set(doctype, opts); },
  listSettings(doctype: string) { return listRegistry.get(doctype); },
  defineProfileSection(section: ProfileSection) {
    const i = profileRegistry.findIndex((s) => s.id === section.id);
    if (i >= 0) profileRegistry[i] = section;
    else profileRegistry.push(section);
  },
  profileSections(): ProfileSection[] { return [...profileRegistry]; },
  FormController,
  ddcore: {
    _: __,
    __,
    // read from boot on every access, so a script sees the session the desk is on
    get session() {
      const b = boot.data;
      const user = b?.user || "Guest";
      return { user, fullName: b?.userDoc?.full_name || user, roles: [...(b?.roles || [])], lang: b?.lang || "" };
    },
    hasRole,
    call: (path: string, args?: any) => api.call(path, args),
    report: (name: string, filters: Record<string, any> = {}) => api.report(name, filters),
    api,
    notifications: api.notifications,
    // a change to a task moves the sidebar's pending count, which only follows notifications
    assignments: {
      ...api.assignments,
      assign: (...a: Parameters<typeof api.assignments.assign>) => api.assignments.assign(...a).then(withPendingCount),
      complete: (id: string) => api.assignments.complete(id).then(withPendingCount),
      revoke: (id: string) => api.assignments.revoke(id).then(withPendingCount),
      reopen: (id: string) => api.assignments.reopen(id).then(withPendingCount),
    },
    shares: api.shares,
    search: { global: (txt: string, limit?: number) => api.globalSearch(txt, limit) },
    // events an app sends with ddcore.publish; a form script uses frm.onRealtime,
    // which drops the handler when the form closes
    realtime: { on: subscribe, off: unsubscribe },
    db: {
      getValue: async (doctype: string, id: string | Record<string, any>, field: string | string[]) => {
        const fields = Array.isArray(field) ? field : [field];
        const rows = await api.list(doctype, { filters: typeof id === "string" ? { id } : id, fields, limit: 1 });
        const row = rows[0];
        if (!row) return null;
        return Array.isArray(field) ? row : row[field];
      },
      getList: (doctype: string, args: any = {}) => api.list(doctype, { filters: args.filters, fields: args.fields, order_by: args.orderBy, limit: args.limit, start: args.start }),
      count: (doctype: string, filters?: any) => api.count(doctype, filters),
      getSingle: (doctype: string) => api.getSingle(doctype),
      getDoc: (doctype: string, id: string) => api.getDoc(doctype, id),
      setValue: (doctype: string, id: string, values: any) => api.update(doctype, id, values),
      insert: (doc: any) => api.insert(doc.doctype, doc),
    },
    ui: { Dialog: dialog, dialog, msgprint: (m: string, o: any = {}) => toast(m, { title: o.title, indicator: o.indicator || "blue" }), alert: (m: string) => toast(m, { indicator: "blue" }), confirm, prompt, showError, toast,
      // a site with feedback off has no modal to open
      openFeedback: (opts?: FeedbackPreset) => { if (boot.data?.site?.feedback) openFeedback(opts); } },
    // roundCurrency is here so a form script that totals a grid rounds the way
    // the server is about to store it, rather than the way toFixed happens to
    format: { currency: formatCurrency, date: formatDate, number: formatNumber, value: formatValue, roundCurrency, statusColor },
    // civil dates with the same semantics as `ddcore.utils` no servidor (ver $lib/datetime)
    datetime: { today: () => today(), addMonths, addDays, dateDiff, monthDiff, monthStart, monthEnd },
    meta: getMeta,
    route: (path: string) => import("$app/navigation").then((n) => n.goto(path)),
    setRoute: (...parts: string[]) => {
      const rem = getRememberedWorkspace();
      const first = parts[0] || "";
      // the first part is a name (a DocType, or the workspace itself); what follows is a record id
      const tail = parts.map((part, i) => (i === 0 ? seg(part) : encodeURIComponent(part))).join("/");
      const path = rem && first !== rem ? `/app/${seg(rem)}/${tail}` : `/app/${tail}`;
      return import("$app/navigation").then((n) => n.goto(path));
    },
  },
};

export function installDeskSDK() {
  (window as any).__ddcoreDesk = deskSDK;
  (window as any).ddcore = deskSDK.ddcore;
  (window as any).__ = __;
}

const loadedIncludes = { desk: new Set<string>(), portal: new Set<string>() };
/** Loads each named app's desk or portal include bundle once (client/*.ts declared in ddcore.app.ts). */
export async function loadAppIncludes(apps: string[], version: number, kind: "desk" | "portal") {
  (window as any).__ddcoreLoaded = version;
  for (const name of apps) {
    if (loadedIncludes[kind].has(name)) continue;
    loadedIncludes[kind].add(name);
    try { await import(/* @vite-ignore */ `/assets/apps/${name}/${kind}.js?v=${version}`); } catch (e) { console.error(`${kind} include`, name, e); }
  }
}
