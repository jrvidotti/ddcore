import type { CardViewOptions } from "../../desk-sdk";
import { isLayout, type DocTypeMeta, type Field } from "../../meta";

export interface CardFields {
  title: string;
  subtitle?: string;
  dateField?: string;
  /** the image field when this reader has it; undefined shows the avatar */
  image?: string;
  /** whether a card has a picture slot at all (image or initials avatar) */
  hasImage: boolean;
  keyFields: Field[];
  fetchFields: string[];
}

/** Resolve presentation and query fields together so card values are fetched. */
export function resolveCardFields(doctype: DocTypeMeta, card: CardViewOptions = {}): CardFields {
  const title = card.title || doctype.titleField || "id";
  const dateField = card.dateField || doctype.fields.find((f) => f.fieldname && !f.hidden && !isLayout(f) && (f.fieldtype === "Date" || f.fieldtype === "Datetime"))?.fieldname;
  // A field above the reader's permlevel is not in the meta, so it resolves to the avatar.
  const imageName = card.image || doctype.imageField;
  const image = imageName && doctype.fields.some((f) => f.fieldname === imageName) ? imageName : undefined;
  const keyFields = doctype.fields.filter((f) => f.inListView && f.fieldname && !isLayout(f) && f.fieldtype !== "Table" && ![title, card.subtitle, dateField, imageName, "status"].includes(f.fieldname)).slice(0, 3);
  return {
    title, subtitle: card.subtitle, dateField, image, hasImage: !!imageName, keyFields,
    fetchFields: [title, card.subtitle, dateField, image, ...keyFields.map((f) => f.fieldname)].filter((field): field is string => !!field),
  };
}
