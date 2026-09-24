// A Dynamic Link's options name a sibling field holding the target DocType.
// That field is a DocType picker: its values are DocType names, never free text.
import type { DocTypeMeta, Field, Meta } from "./meta";

export interface DocTypeChoices { options: string[]; optionLabels: string[] }

/** The DocTypes the user can see, sorted by label, as Select options. */
export function docTypeChoices(doctypes: Record<string, { label?: string }> | undefined): DocTypeChoices {
  const entries = Object.entries(doctypes || {}).map(([name, d]) => [name, d?.label || name] as const);
  entries.sort((a, b) => a[1].localeCompare(b[1]));
  return { options: entries.map((e) => e[0]), optionLabels: entries.map((e) => e[1]) };
}

/** Fieldnames that some Dynamic Link of the DocType names as its type field. */
export function dynamicLinkSelectors(d: DocTypeMeta): Set<string> {
  const out = new Set<string>();
  for (const f of d.fields) {
    if (f.fieldtype === "Dynamic Link" && typeof f.options === "string" && f.options) out.add(f.options);
  }
  return out;
}

/** The Dynamic Links whose type field is `fieldname`: they change meaning when it does. */
export function dynamicLinksOf(d: DocTypeMeta, fieldname: string): string[] {
  return d.fields
    .filter((f) => f.fieldtype === "Dynamic Link" && f.options === fieldname && f.fieldname)
    .map((f) => f.fieldname!);
}

/** A Data field as a Select of DocTypes. */
export function asDocTypeSelect(f: Field, choices: DocTypeChoices): Field {
  return { ...f, fieldtype: "Select", options: choices.options, optionLabels: choices.optionLabels };
}

/**
 * Turns every Data field a Dynamic Link names into a DocType Select, in the
 * DocType and its child tables, so the form, the grid and the list filters
 * all offer a list instead of a text box.
 */
export function applyDocTypeSelectors(m: Meta, doctypes: Record<string, { label?: string }> | undefined): Meta {
  const choices = docTypeChoices(doctypes);
  const shape = (d: DocTypeMeta): DocTypeMeta => {
    const selectors = dynamicLinkSelectors(d);
    if (!selectors.size) return d;
    return { ...d, fields: d.fields.map((f) => (f.fieldtype === "Data" && f.fieldname && selectors.has(f.fieldname) ? asDocTypeSelect(f, choices) : f)) };
  };
  const children: Record<string, DocTypeMeta> = {};
  for (const [name, child] of Object.entries(m.children || {})) children[name] = shape(child);
  return { ...m, doctype: shape(m.doctype), children };
}
