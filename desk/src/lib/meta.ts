import { __ } from "./boot.svelte";
// DocType meta as served by /api/meta, cached per session (invalidated on reload events).
import { api } from "./api";

export type FieldWidth = "sm" | "md" | "lg" | "full";

export interface Field {
  fieldname?: string; fieldtype: string; label?: string; options?: any; reqd?: boolean; unique?: boolean; default?: any;
  readOnly?: boolean; hidden?: boolean; fetchFrom?: string; dependsOn?: string; readOnlyDependsOn?: string; mandatoryDependsOn?: string;
  allowOnSubmit?: boolean; inListView?: boolean; inStandardFilter?: boolean; length?: number; precision?: number; description?: string;
  columns?: number; width?: FieldWidth; gridEditMode?: "inline" | "dialog"; collapsible?: boolean; bold?: boolean;
  /** Field permission level; see `applyFieldLevels`. */
  permlevel?: number;
  /** Display text for a Select, aligned with `options`; filled by the server. */
  optionLabels?: string[];
  /** Indicator colour per canonical (English) Select value. */
  optionColors?: Record<string, string>;
}

export interface DocTypeMeta {
  name: string; app: string; label: string; module?: string; naming: any; submittable?: boolean; isChild?: boolean; isSingle?: boolean; trackChanges?: boolean;
  allowRename?: boolean; titleField?: string; sortField?: string; sortOrder?: string; searchFields?: string[]; fields: Field[];
  permissions?: any[]; icon?: string; methods?: string[];
  /** Compound business keys; enforced on the server, shown here only for reference. */
  uniqueKeys?: { name: string; fields: string[] }[];
  /** apps shipping a form script for this DocType: the owner, then each extension */
  formApps?: string[];
}

export interface Meta {
  doctype: DocTypeMeta;
  children: Record<string, DocTypeMeta>;
  permissions: Record<string, boolean>;
  /** Field permission levels this user reads and writes, for the DocType and its children. */
  fieldLevels?: FieldLevels;
  series: string[] | null;
  linkTitles: Record<string, string>;
}

export interface FieldLevels { read: number[]; write: number[] }

/**
 * Shapes the meta to the user's field permission levels (SEC-02): a field they
 * cannot read is removed — the server never sends its value, and a form, grid,
 * list column or filter built on it would only show a blank or earn a 403 —
 * and a field they cannot write is read-only. The server enforces both; this
 * only keeps the screen honest. Child tables follow the parent's levels.
 */
export function applyFieldLevels(m: Meta): Meta {
  const levels = m.fieldLevels;
  if (!levels) return m;
  const read = new Set(levels.read), write = new Set(levels.write);
  const shape = (d: DocTypeMeta): DocTypeMeta => ({
    ...d,
    fields: d.fields
      .filter((f) => read.has(f.permlevel || 0))
      .map((f) => (write.has(f.permlevel || 0) ? f : { ...f, readOnly: true })),
  });
  const children: Record<string, DocTypeMeta> = {};
  for (const [name, child] of Object.entries(m.children || {})) children[name] = shape(child);
  return { ...m, doctype: shape(m.doctype), children };
}

const cache = new Map<string, Promise<Meta>>();

export function getMeta(doctype: string): Promise<Meta> {
  let p = cache.get(doctype);
  if (!p) {
    p = api
      .meta(doctype)
      .then((raw) => {
        const m = applyFieldLevels(raw);
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

export const isLayout = (f: Field) => ["Section Break", "Tab Break", "HTML"].includes(f.fieldtype);
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

export const DEFAULT_FIELD_WIDTH: Record<string, FieldWidth> = {
  Date: "sm",
  Month: "sm",
  Time: "sm",
  Int: "sm",
  Percent: "sm",
  Datetime: "md",
  Float: "md",
  Currency: "md",
  // types that only read well across the whole line
  Text: "full",
  "Small Text": "full",
  "Text Editor": "full",
  JSON: "full",
  Table: "full",
  HTML: "full",
};

/** How many quarters of a form line each width takes. See `form-layout.ts`. */
export const FIELD_WIDTH_SLOTS: Record<FieldWidth, number> = { sm: 1, md: 1, lg: 2, full: 4 };

/**
 * Resolves the visual width for a field's control inside a form.
 * Child table grids (`inGrid: true`) always resolve to "full" because `columns`
 * already sizes grid columns.
 */
export function resolveFieldWidth(field?: Field | null, inGrid = false): FieldWidth {
  if (!field || inGrid) return "full";
  if (field.width && (field.width === "sm" || field.width === "md" || field.width === "lg" || field.width === "full")) {
    return field.width;
  }
  return DEFAULT_FIELD_WIDTH[field.fieldtype] || "lg";
}

