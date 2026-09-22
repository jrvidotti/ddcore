import { __ } from "../boot.svelte";
// Version history processing and diff utilities for ddcore desk forms.
import type { FormController } from "../form.svelte";
import type { DocTypeMeta, Field } from "../meta";
import { formatDate, formatDatetime, formatCurrency, formatNumber, formatValue, timeAgo } from "../format.ts";
import { htmlToText } from "../richtext.ts";

export const IGNORED_CHILD_FIELDS = new Set([
  "idx",
  "id",
  "owner",
  "parent",
  "parentfield",
  "parenttype",
  "doctype",
  "creation",
  "modified",
  "modified_by",
  "docstatus",
  "__islocal",
  "__unsaved",
]);

export interface FormattedValue {
  value: any;
  formatted: string;
  isAttach: boolean;
  url?: string;
}

export interface TableFieldDiff {
  field: string;
  label: string;
  fieldtype: string;
  from: any;
  to: any;
  formattedFrom: string;
  formattedTo: string;
  isAttach: boolean;
  fromUrl?: string;
  toUrl?: string;
}

export interface TableRowSummary {
  rowIdx: number;
  rowName: string;
  fields: Record<string, { label: string; formatted: string; isAttach: boolean; url?: string }>;
}

export interface TableDiff {
  added: TableRowSummary[];
  removed: TableRowSummary[];
  modified: Array<{
    rowIdx: number;
    rowName: string;
    changes: TableFieldDiff[];
  }>;
}

export interface ParsedChange {
  field: string;
  label: string;
  fieldtype: string;
  isTable: boolean;
  oldValue: any;
  newValue: any;
  formattedOld: string;
  formattedNew: string;
  isAttach: boolean;
  oldUrl?: string;
  newUrl?: string;
  tableDiff?: TableDiff;
}

export interface ParsedVersion {
  id: string;
  owner: string;
  creation: string;
  relativeTime: string;
  fullTime: string;
  changes: ParsedChange[];
  totalChanges: number;
}

export function isAttachValue(val: any, fieldtype?: string): boolean {
  if (fieldtype === "Attach" || fieldtype === "Attach Image") return true;
  if (typeof val === "string" && (val.startsWith("/files/") || val.startsWith("/private/files/"))) return true;
  return false;
}

export function extractFileName(val: any): string {
  if (!val) return "";
  const str = String(val);
  const name = str.split("/").pop() || str;
  try {
    return decodeURIComponent(name);
  } catch {
    return name;
  }
}

export function humanize(str: string): string {
  if (!str) return "";
  return str
    .replace(/_/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

export function formatDiffValue(val: any, field?: Partial<Field>, fieldname?: string): FormattedValue {
  if (val === null || val === undefined || val === "") {
    return { value: val, formatted: "—", isAttach: false };
  }

  const isAttach = isAttachValue(val, field?.fieldtype);
  if (isAttach) {
    return {
      value: val,
      formatted: extractFileName(val),
      isAttach: true,
      url: String(val),
    };
  }

  if (fieldname === "docstatus" || field?.fieldname === "docstatus") {
    const s = Number(val);
    const label = s === 1 ? "Enviado" : s === 2 ? "Cancelado" : "Rascunho";
    return { value: val, formatted: label, isAttach: false };
  }

  if (field?.fieldtype === "Check" || typeof val === "boolean") {
    return { value: val, formatted: val ? __("Yes") : __("No"), isAttach: false };
  }

  if (field?.fieldtype === "Date") {
    return { value: val, formatted: formatDate(val), isAttach: false };
  }

  if (field?.fieldtype === "Datetime") {
    return { value: val, formatted: formatDatetime(val), isAttach: false };
  }

  if (field?.fieldtype === "Currency") {
    return { value: val, formatted: formatCurrency(val), isAttach: false };
  }

  if (field?.fieldtype === "Percent") {
    return { value: val, formatted: formatNumber(val, field.precision ?? 2) + "%", isAttach: false };
  }

  if (field?.fieldtype === "Float") {
    return { value: val, formatted: formatNumber(val, field.precision), isAttach: false };
  }

  if (field?.fieldtype === "Int") {
    return { value: val, formatted: String(Math.round(Number(val))), isAttach: false };
  }

  // Rich text and Markdown diff as what they read as: a diff of raw markup
  // shows tags nobody wrote and hides the sentence that changed.
  if (field?.fieldtype === "Text Editor") {
    return { value: val, formatted: htmlToText(String(val)), isAttach: false };
  }

  if (field?.fieldtype === "Markdown Editor" || field?.fieldtype === "Code") {
    return { value: val, formatted: String(val), isAttach: false };
  }

  if (field?.fieldtype === "Duration" || field?.fieldtype === "Rating" || field?.fieldtype === "Color") {
    return { value: val, formatted: formatValue(val, field), isAttach: false };
  }

  if (typeof val === "object") {
    return { value: val, formatted: JSON.stringify(val), isAttach: false };
  }

  return { value: val, formatted: String(val), isAttach: false };
}

function summarizeRow(row: any, childMeta?: DocTypeMeta): TableRowSummary {
  const fields: Record<string, { label: string; formatted: string; isAttach: boolean; url?: string }> = {};
  for (const [k, v] of Object.entries(row)) {
    if (IGNORED_CHILD_FIELDS.has(k) || v === null || v === undefined || v === "") continue;
    const f = childMeta?.fields?.find((x) => x.fieldname === k);
    const formatted = formatDiffValue(v, f, k);
    fields[k] = {
      label: f?.label || humanize(k),
      formatted: formatted.formatted,
      isAttach: formatted.isAttach,
      url: formatted.url,
    };
  }
  return {
    rowIdx: Number(row.idx) || 1,
    rowName: String(row.id || ""),
    fields,
  };
}

// A Version written before 0.17 keeps its child rows as they were then, keyed
// by `name`. A row with no `id` but a `name` is one of those: read the key from
// where it was, so old history still pairs its rows instead of showing every
// row as removed and added again.
function legacyRow(r: any): any {
  if (!r || typeof r !== "object" || r.id !== undefined || r.name === undefined) return r;
  const { name, ...rest } = r;
  return { ...rest, id: name };
}

export function diffTable(beforeRows: any[], afterRows: any[], childMeta?: DocTypeMeta): TableDiff {
  const before = (Array.isArray(beforeRows) ? beforeRows : []).map(legacyRow);
  const after = (Array.isArray(afterRows) ? afterRows : []).map(legacyRow);

  const beforeMap = new Map<string, any>();
  before.forEach((r, i) => beforeMap.set(r?.id || `idx_${r?.idx || i + 1}`, r));

  const afterMap = new Map<string, any>();
  after.forEach((r, i) => afterMap.set(r?.id || `idx_${r?.idx || i + 1}`, r));

  const added: TableRowSummary[] = [];
  const removed: TableRowSummary[] = [];
  const modified: Array<{ rowIdx: number; rowName: string; changes: TableFieldDiff[] }> = [];

  for (const [key, row] of afterMap.entries()) {
    if (!beforeMap.has(key)) {
      added.push(summarizeRow(row, childMeta));
    } else {
      const prev = beforeMap.get(key);
      const changes: TableFieldDiff[] = [];
      const allKeys = new Set([...Object.keys(prev || {}), ...Object.keys(row || {})]);

      for (const k of allKeys) {
        if (IGNORED_CHILD_FIELDS.has(k)) continue;
        const a = prev ? prev[k] : undefined;
        const b = row ? row[k] : undefined;

        if (JSON.stringify(a ?? null) !== JSON.stringify(b ?? null)) {
          const f = childMeta?.fields?.find((x) => x.fieldname === k);
          const fOld = formatDiffValue(a, f, k);
          const fNew = formatDiffValue(b, f, k);
          changes.push({
            field: k,
            label: f?.label || humanize(k),
            fieldtype: f?.fieldtype || "Data",
            from: a,
            to: b,
            formattedFrom: fOld.formatted,
            formattedTo: fNew.formatted,
            isAttach: fOld.isAttach || fNew.isAttach,
            fromUrl: fOld.url,
            toUrl: fNew.url,
          });
        }
      }

      if (changes.length > 0) {
        modified.push({
          rowIdx: Number(row.idx) || Number(prev.idx) || 1,
          rowName: String(row.id || prev.id || key),
          changes,
        });
      }
    }
  }

  for (const [key, row] of beforeMap.entries()) {
    if (!afterMap.has(key)) {
      removed.push(summarizeRow(row, childMeta));
    }
  }

  return { added, removed, modified };
}

export function parseVersion(v: any, frm?: FormController): ParsedVersion {
  let rawData: any = {};
  try {
    rawData = typeof v.data === "string" ? JSON.parse(v.data) : v.data || {};
  } catch {
    rawData = {};
  }

  const changed = rawData.changed || {};
  const changes: ParsedChange[] = [];

  for (const [field, pair] of Object.entries(changed)) {
    if (!Array.isArray(pair)) continue;
    const [oldVal, newVal] = pair;

    const fieldDef = frm?.field(field);
    const isTable = fieldDef?.fieldtype === "Table" || (Array.isArray(oldVal) && Array.isArray(newVal));

    if (isTable) {
      const childDoctype = fieldDef?.options;
      const childMeta = childDoctype ? frm?.meta?.children?.[childDoctype] : undefined;
      const tableDiff = diffTable(oldVal, newVal, childMeta);

      const hasChanges = tableDiff.added.length > 0 || tableDiff.removed.length > 0 || tableDiff.modified.length > 0;
      if (hasChanges) {
        changes.push({
          field,
          label: fieldDef?.label || humanize(field),
          fieldtype: "Table",
          isTable: true,
          oldValue: oldVal,
          newValue: newVal,
          formattedOld: `${Array.isArray(oldVal) ? oldVal.length : 0} itens`,
          formattedNew: `${Array.isArray(newVal) ? newVal.length : 0} itens`,
          isAttach: false,
          tableDiff,
        });
      }
    } else {
      const fOld = formatDiffValue(oldVal, fieldDef, field);
      const fNew = formatDiffValue(newVal, fieldDef, field);
      changes.push({
        field,
        label: fieldDef?.label || (field === "docstatus" ? __("Document status") : humanize(field)),
        fieldtype: fieldDef?.fieldtype || "Data",
        isTable: false,
        oldValue: oldVal,
        newValue: newVal,
        formattedOld: fOld.formatted,
        formattedNew: fNew.formatted,
        isAttach: fOld.isAttach || fNew.isAttach,
        oldUrl: fOld.url,
        newUrl: fNew.url,
      });
    }
  }

  return {
    id: String(v.id || ""),
    owner: String(v.owner || "Sistema"),
    creation: String(v.creation || ""),
    relativeTime: timeAgo(v.creation),
    fullTime: formatDatetime(v.creation),
    changes,
    totalChanges: changes.length,
  };
}
