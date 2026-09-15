import type { CardViewOptions } from "../../desk-sdk";
import { isLayout, type DocTypeMeta, type Field } from "../../meta";

export interface CardFields {
  title: string;
  subtitle?: string;
  dateField?: string;
  keyFields: Field[];
  fetchFields: string[];
}

/** Resolve presentation and query fields together so card values are fetched. */
export function resolveCardFields(doctype: DocTypeMeta, card: CardViewOptions = {}): CardFields {
  const title = card.title || doctype.titleField || "name";
  const dateField = card.dateField || doctype.fields.find((f) => f.fieldname && !f.hidden && !isLayout(f) && (f.fieldtype === "Date" || f.fieldtype === "Datetime"))?.fieldname;
  const keyFields = doctype.fields.filter((f) => f.inListView && f.fieldname && !isLayout(f) && f.fieldtype !== "Table" && ![title, card.subtitle, dateField, "status"].includes(f.fieldname)).slice(0, 3);
  return {
    title, subtitle: card.subtitle, dateField, keyFields,
    fetchFields: [title, card.subtitle, dateField, ...keyFields.map((f) => f.fieldname)].filter((field): field is string => !!field),
  };
}
