// Duplicate: the open record as an unsaved copy, without the values that
// belong to the original alone.
import { isLayout, isTableType, newDoc, type DocTypeMeta, type Field, type Meta } from "./meta";

/**
 * Whether Duplicate leaves a field's value behind: `noCopy`, a `unique` field
 * (the copy could never be saved with it) and a `readOnly` one, which the
 * server writes — unless it is a `fetchFrom`, which follows a Link the copy
 * keeps.
 */
export const skipsOnCopy = (f: Field) => !!f.noCopy || !!f.unique || (!!f.readOnly && !f.fetchFrom);

/** What a new document of the DocType starts with: its defaults, the same as newDoc. */
const fresh = (d: DocTypeMeta) => newDoc({ doctype: d } as Meta);

function copyFields(d: DocTypeMeta, src: any, into: any, children: Record<string, DocTypeMeta>) {
  for (const f of d.fields) {
    if (!f.fieldname || isLayout(f)) continue;
    if (skipsOnCopy(f)) continue; // keeps `into`'s default
    const v = src[f.fieldname];
    if (!isTableType(f.fieldtype)) { into[f.fieldname] = v; continue; }
    const child = children[f.options];
    into[f.fieldname] = (v || []).map((r: any) => {
      const row = { ...r, id: undefined, parent: undefined, creation: undefined, modified: undefined, owner: undefined };
      if (!child) return row;
      const { __islocal, doctype, ...defaults } = fresh(child);
      const out = { ...row, ...defaults };
      copyFields(child, row, out, children);
      return out;
    });
  }
}

/** The copy Duplicate opens: a new draft with the original's copyable values and every other field at its default. */
export function duplicateDoc(meta: Meta, doc: any): any {
  const copy = { ...fresh(meta.doctype) };
  copyFields(meta.doctype, doc, copy, meta.children || {});
  if ("amended_from" in copy) copy.amended_from = null; // a copy is not an amendment
  return copy;
}
