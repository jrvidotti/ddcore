// Turns the list's filter state into the API's filter tuples.

/** An extra choice on a standard Select filter, registered with defineListView({ filterOptions }). */
export interface ListFilterOption {
  value: string;
  label: string;
  /** Applied in place of `[field, "=", value]` when this option is chosen. */
  filters: any[][];
}

export function buildListFilters(
  filters: Record<string, any>,
  options: Record<string, ListFilterOption[] | undefined> = {},
): any[][] {
  const out: any[][] = [];
  for (const [k, v] of Object.entries(filters)) {
    if (v === null || v === undefined || v === "") continue;
    const option = options[k]?.find((o) => o.value === v);
    if (option) out.push(...option.filters);
    else out.push([k, "=", v]);
  }
  return out;
}
