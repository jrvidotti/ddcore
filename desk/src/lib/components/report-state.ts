import type { Field } from "$lib/meta";
import { isNumericFieldtype } from "../meta";

function filterValue(field: Field, value: string): any {
  if (value === "") return "";
  if (field.fieldtype === "Check") return value === "true" || value === "1";
  if (isNumericFieldtype(field.fieldtype)) {
    const n = Number(value);
    return Number.isFinite(n) ? n : null;
  }
  return value;
}

export function reportFiltersFromSearchParams(params: URLSearchParams, fields: Field[], defaults: Record<string, any>): Record<string, any> {
  const filters = { ...defaults };
  for (const field of fields) {
    if (!field.fieldname || !params.has(field.fieldname)) continue;
    const value = filterValue(field, params.get(field.fieldname) || "");
    if (value === null) delete filters[field.fieldname];
    else filters[field.fieldname] = value;
  }
  return filters;
}

export function reportFiltersToSearchParams(filters: Record<string, any>, fields: Field[]): URLSearchParams {
  const params = new URLSearchParams();
  for (const field of fields) {
    const name = field.fieldname;
    const value = name ? filters[name] : undefined;
    if (name && value !== null && value !== undefined) params.set(name, String(value));
  }
  return params;
}
