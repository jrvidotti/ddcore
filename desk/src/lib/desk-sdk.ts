// The runtime that app client bundles import as "@ddcore/desk-sdk" (resolved
// by the Go bundler to window.__ddcoreDesk). The public types come from
// packages/desk-sdk/src/index.ts.
import type {
  CalendarNewOption, CalendarViewOptions, CardViewOptions, DeskViewMode, GanttViewOptions, KanbanViewOptions,
  ListAction, ListActionContext, ListSummaryCard, ListToolbarAction, ListToolbarContext, ListViewOptions, ProfileSection,
  TreeViewOptions,
} from "@ddcore/desk-sdk";
import { seg } from "./routes";
import { api } from "./api";
import { subscribe, unsubscribe } from "./events";
import { registerForm, type FormHandlers, FormController } from "./form.svelte";
import { dialog, toast, confirm, prompt, showError, ui } from "./ui.svelte";
import { __, boot, hasRole } from "./boot.svelte";
import { formatCurrency, formatDate, formatNumber, formatValue, roundCurrency, statusColor } from "./format";
import { getMeta } from "./meta";
import { addDays, addMonths, dateDiff, monthDiff, monthEnd, monthStart, today } from "./datetime";
import { getRememberedWorkspace } from "./components/sidebar-workspace";
import { refreshPendingCount } from "./assignments.svelte";
import { openFeedback, type FeedbackPreset } from "./feedback.svelte";
import { impersonate } from "./impersonation";


function withPendingCount<T>(result: T): T {
  void refreshPendingCount();
  return result;
}

export type {
  CalendarNewOption, CalendarViewOptions, CardViewOptions, DeskViewMode, GanttViewOptions, KanbanViewOptions,
  ListAction, ListActionContext, ListSummaryCard, ListToolbarAction, ListToolbarContext, ListViewOptions, ProfileSection,
  TreeViewOptions,
};

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
      return { user, fullName: b?.userDoc?.full_name || user, roles: [...(b?.roles || [])], lang: b?.lang || "",
        impersonatedBy: b?.impersonatedBy?.id };
    },
    impersonate,
    hasRole,
    // the bar at the top runs while it is out, as it does for frm.call
    call: async (path: string, args?: any) => {
      ui.busy++;
      try { return await api.call(path, args); } finally { ui.busy--; }
    },
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
