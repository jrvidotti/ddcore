// The columns of a list's table: what the app declares, and what each user
// changed with the Columns menu.
//
// The app's `defineListView` (or the meta's `inListView`) is the default. A
// user who hides or adds a column gets a choice kept in the browser, per user,
// tenant and DocType — tenants share one origin, so the tenant is in the key.
// "Reset to default" deletes it. Pure, so it can be tested without a component.

import type { ListViewOptions } from "../desk-sdk";
import { isLayout, isTableType, type DocTypeMeta, type Field } from "../meta";
import { showIDColumn } from "./views/id-column";

/** What a user changed: the visible fields in order, and the id and Modified columns. */
export interface ColumnChoice {
  fields: string[];
  id?: boolean;
  modified?: boolean;
}

/** The table's columns once the app's defaults and the user's choice are combined. */
export interface ResolvedColumns {
  columns: Field[];
  showID: boolean;
  showModified: boolean;
  /** Whether a user choice is in force, so "Reset to default" has something to undo. */
  customized: boolean;
}

/** The app's columns: `defineListView({ columns })`, else the `inListView` fields with the title first, at most 7. */
export function defaultListColumns(doctype: DocTypeMeta, settings: ListViewOptions = {}): Field[] {
  const byName = (n: string) => doctype.fields.find((f) => f.fieldname === n);
  if (settings.columns?.length) return settings.columns.map(byName).filter(Boolean) as Field[];
  const cols = doctype.fields.filter((f) => f.inListView && !isLayout(f) && !isTableType(f.fieldtype));
  if (doctype.titleField && !cols.some((c) => c.fieldname === doctype.titleField)) {
    const tf = byName(doctype.titleField);
    if (tf) cols.unshift(tf);
  }
  return cols.slice(0, 7);
}

// No column (computed, Vault), never leaves the server (Password), or too
// bulky for a list cell (Signature, Geolocation).
const NOT_A_COLUMN = new Set(["Password", "Vault", "Signature", "Geolocation"]);

/**
 * The fields a user may show: every data field the reader can see (the meta
 * already lacks the ones their field level hides), less layout, tables, hidden
 * and computed fields, and the synthetic `id` of a prompted id. An app's
 * default column is always one, whatever it is.
 */
export function columnCandidates(doctype: DocTypeMeta, defaults: Field[] = []): Field[] {
  const keep = new Set(defaults.map((f) => f.fieldname));
  return doctype.fields.filter((f) => {
    if (!f.fieldname || f.fieldname === "id") return false;
    if (keep.has(f.fieldname)) return true;
    return !isLayout(f) && !isTableType(f.fieldtype) && !f.hidden && !f.computed && !NOT_A_COLUMN.has(f.fieldtype);
  });
}

/** Combines the app's columns with a user's choice; a field the meta no longer offers drops out. */
export function resolveColumns(doctype: DocTypeMeta, settings: ListViewOptions = {}, choice: ColumnChoice | null = null): ResolvedColumns {
  const defaults = defaultListColumns(doctype, settings);
  if (!choice) {
    return { columns: defaults, showID: showIDColumn(doctype, defaults, settings), showModified: settings.modifiedColumn !== false, customized: false };
  }
  const candidates = columnCandidates(doctype, defaults);
  const columns = choice.fields.map((n) => candidates.find((f) => f.fieldname === n)).filter(Boolean) as Field[];
  return {
    columns,
    showID: choice.id ?? showIDColumn(doctype, columns, settings),
    showModified: choice.modified ?? settings.modifiedColumn !== false,
    customized: true,
  };
}

/** The choice after the user checks or unchecks one column ("id" and "modified" name the fixed ones). */
export function toggleColumn(current: ResolvedColumns, column: string, on: boolean): ColumnChoice {
  const choice: ColumnChoice = { fields: current.columns.map((c) => c.fieldname!), id: current.showID, modified: current.showModified };
  if (column === "id") choice.id = on;
  else if (column === "modified") choice.modified = on;
  else if (on && !choice.fields.includes(column)) choice.fields.push(column);
  else if (!on) choice.fields = choice.fields.filter((n) => n !== column);
  return choice;
}

/** The fields a server export asks for: the visible columns, in screen order, those with a column of their own. */
export function exportFields(r: ResolvedColumns): string[] {
  return [
    ...(r.showID ? ["id"] : []),
    ...r.columns.filter((c) => !c.computed).map((c) => c.fieldname!),
    ...(r.showModified ? ["modified"] : []),
  ];
}

type Store = { getItem(k: string): string | null; setItem(k: string, v: string): void; removeItem(k: string): void };

function defaultStore(): Store | null {
  try {
    return (globalThis as any).localStorage ?? null;
  } catch {
    return null;
  }
}

/** Where a user's choice for a DocType is kept: per user and tenant ("" is the platform space). */
export const columnsKey = (user: string | null | undefined, tenant: string | null | undefined, doctype: string) =>
  `ddcore_columns:${user || "guest"}:${tenant || ""}:${doctype}`;

/** The stored choice, or `null` when there is none (or it is not one). */
export function loadColumnChoice(key: string, store = defaultStore()): ColumnChoice | null {
  try {
    const raw = store?.getItem(key);
    if (!raw) return null;
    const v = JSON.parse(raw);
    if (!v || !Array.isArray(v.fields)) return null;
    return {
      fields: v.fields.filter((n: unknown): n is string => typeof n === "string"),
      ...(typeof v.id === "boolean" ? { id: v.id } : {}),
      ...(typeof v.modified === "boolean" ? { modified: v.modified } : {}),
    };
  } catch {
    return null;
  }
}

/** Keeps a choice, or deletes it with `null` (back to the app's default). */
export function saveColumnChoice(key: string, choice: ColumnChoice | null, store = defaultStore()): void {
  try {
    if (choice) store?.setItem(key, JSON.stringify(choice));
    else store?.removeItem(key);
  } catch {
    /* private mode / SSR */
  }
}
