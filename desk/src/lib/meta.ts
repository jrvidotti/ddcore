import { __ } from "./boot.svelte";
// DocType meta as served by /api/meta, cached per session (invalidated on reload events).
import { api } from "./api";

export interface Field {
  fieldname?: string; fieldtype: string; label?: string; options?: any; reqd?: boolean; unique?: boolean; default?: any;
  readOnly?: boolean; hidden?: boolean; fetchFrom?: string; dependsOn?: string; readOnlyDependsOn?: string; mandatoryDependsOn?: string;
  allowOnSubmit?: boolean; inListView?: boolean; inStandardFilter?: boolean; length?: number; precision?: number; description?: string;
  columns?: number; gridEditMode?: "inline" | "dialog"; collapsible?: boolean; bold?: boolean;
  /** Display text for a Select, aligned with `options`; filled by the server. */
  optionLabels?: string[];
  /** Indicator colour per canonical (English) Select value. */
  optionColors?: Record<string, string>;
}

export interface DocTypeMeta {
  name: string; app: string; label: string; module?: string; naming: any; submittable?: boolean; isChild?: boolean; trackChanges?: boolean;
  allowRename?: boolean; titleField?: string; sortField?: string; sortOrder?: string; searchFields?: string[]; fields: Field[];
  permissions?: any[]; icon?: string; methods?: string[];
  /** apps shipping a form script for this DocType: the owner, then each extension */
  formApps?: string[];
}

export interface Meta {
  doctype: DocTypeMeta;
  children: Record<string, DocTypeMeta>;
  permissions: Record<string, boolean>;
  series: string[] | null;
  linkTitles: Record<string, string>;
}

const cache = new Map<string, Promise<Meta>>();

export function getMeta(doctype: string): Promise<Meta> {
  let p = cache.get(doctype);
  if (!p) {
    p = api
      .meta(doctype)
      .then((m) => {
        if (m.doctype.naming?.prompt && !m.doctype.fields.some((f: Field) => f.fieldname === "name")) {
          const nameField: Field = {
            fieldname: "name",
            fieldtype: "Data",
            label: m.doctype.label || __("Name"),
            reqd: true,
          };
          m.doctype.fields = [nameField, ...m.doctype.fields];
        }
        return m;
      })
      .catch((e) => {
        cache.delete(doctype);
        throw e;
      });
    cache.set(doctype, p);
  }
  return p;
}

export function clearMetaCache() { cache.clear(); }

export const isLayout = (f: Field) => ["Section Break", "Column Break", "Tab Break", "HTML"].includes(f.fieldtype);
export const selectOptions = (f: Field): string[] => (Array.isArray(f.options) ? f.options.map(String) : typeof f.options === "string" ? f.options.split("\n") : []);

/**
 * Display text for a Select, positionally aligned with selectOptions.
 *
 * The server fills `optionLabels` when the catalogue had something to say; the
 * fallback is the value itself, which is a key too. The value is never
 * touched: it is canonical English, and it is what the database holds.
 */
export const selectLabels = (f: Field): string[] => {
  const opts = selectOptions(f);
  const labels = f.optionLabels;
  return opts.map((o, i) => labels?.[i] || __(o));
};

/** Default value for a new doc, mirroring the server. */
export function newDoc(meta: Meta): any {
  const d: any = { doctype: meta.doctype.name, docstatus: 0, __islocal: true };
  for (const f of meta.doctype.fields) {
    if (!f.fieldname || isLayout(f)) continue;
    if (f.fieldtype === "Table") d[f.fieldname] = [];
    else if (f.default !== undefined && f.default !== null) d[f.fieldname] = f.default === "Today" ? (f.fieldtype === "Month" ? thisMonth() : today()) : f.default === "This Month" ? thisMonth() : f.default;
    else if (f.fieldtype === "Check") d[f.fieldname] = false;
    else d[f.fieldname] = null;
  }
  return d;
}

export const today = () => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`; };
export const thisMonth = () => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-01`; };
