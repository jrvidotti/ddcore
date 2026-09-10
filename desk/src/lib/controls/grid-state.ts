import type { Field } from "$lib/meta";

export type GridEditMode = "inline" | "dialog";

export function gridEditMode(field: Pick<Field, "gridEditMode">): GridEditMode {
  return field.gridEditMode === "dialog" ? "dialog" : "inline";
}

export function createChildDraft(fieldname: string, childDoctype: string, parenttype: string, rowCount: number) {
  return { doctype: childDoctype, parentfield: fieldname, parenttype, idx: rowCount + 1, __islocal: true };
}

export function applyRowChanges(row: Record<string, any>, values: Record<string, any>): void {
  Object.assign(row, values);
}

export async function confirmRowRemoval(confirmDelete: () => Promise<boolean>, remove: () => void): Promise<boolean> {
  if (!(await confirmDelete())) return false;
  remove();
  return true;
}

export function fileNameParts(value: unknown): { full: string; stem: string; extension: string } {
  const path = String(value ?? "").split(/[?#]/, 1)[0];
  const encoded = path.split("/").pop() || "";
  let full = encoded;
  try { full = decodeURIComponent(encoded); } catch { /* keep the original text */ }
  const dot = full.lastIndexOf(".");
  if (dot <= 0 || dot === full.length - 1) return { full, stem: full, extension: "" };
  return { full, stem: full.slice(0, dot), extension: full.slice(dot) };
}
