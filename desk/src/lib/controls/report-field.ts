import type { Field } from "$lib/meta";

/** A Report field's filters: each report filter takes the value of the document field it names. */
export function reportFiltersFor(field: Pick<Field, "reportFilters">, doc: Record<string, any>): Record<string, any> {
  const out: Record<string, any> = {};
  for (const [filter, from] of Object.entries(field.reportFilters || {})) out[filter] = doc?.[from] ?? null;
  return out;
}
