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
    setRoute: (...parts: string[]) => import("$app/navigation").then((n) => n.goto("/app/" + parts.map(encodeURIComponent).join("/"))),
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
