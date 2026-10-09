// The totals a list shows above its rows, registered with defineListView({ summary }).
import type { ListSummaryCard } from "$lib/desk-sdk";
import type { Field } from "$lib/meta";
import { buildListFilters } from "./list-filters";

const FIELDNAME = /^[a-z_][a-z0-9_]*$/i;

/** The field a card sums, "" for a count, or null when the aggregate is not `count` or `sum:<field>`. */
export function summaryField(aggregate?: string): string | null {
  const agg = aggregate || "count";
  if (agg === "count") return "";
  if (agg.startsWith("sum:") && FIELDNAME.test(agg.slice(4))) return agg.slice(4);
  return null;
}

/** The select expression the list API computes the card with, or null for an aggregate it does not know. */
export function summaryExpression(aggregate?: string): string | null {
  const field = summaryField(aggregate);
  if (field === null) return null;
  return field ? `sum(${field}) as value` : "count(*) as value";
}

/** The list's filters, then the card's own: triples as they are, an object as equalities. */
export function summaryFilters(listFilters: any[][], card: Pick<ListSummaryCard, "filters">): any[][] {
  const own = card.filters;
  if (!own) return listFilters;
  return [...listFilters, ...(Array.isArray(own) ? own : buildListFilters(own))];
}

/** How the value is formatted: the card's `datatype`, else Int for a count and the summed field's type. */
export function summaryDatatype(card: ListSummaryCard, fields: Field[]): string | undefined {
  if (card.datatype) return card.datatype;
  const field = summaryField(card.aggregate);
  if (field === "") return "Int";
  return fields.find((f) => f.fieldname === field)?.fieldtype;
}

/**
 * One card's value across the filter sets the view loads with (a spanning calendar fetches
 * several, disjoint): their sum, with an empty set counting as 0.
 */
export function combineSummary(parts: any[][]): number {
  let total = 0;
  for (const rows of parts) {
    const v = Number(rows?.[0]?.value ?? 0);
    if (Number.isFinite(v)) total += v;
  }
  return total;
}
