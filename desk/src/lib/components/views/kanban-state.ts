// Pure logic behind the Kanban view: its columns, which card sits in which,
// and the optimistic move a drop makes before the server answers.

export interface KanbanColumn {
  /** The stored value; "" is the column of cards with no value. */
  value: string;
  /** The label shown; for a value outside the options this is the value itself. */
  label: string;
  /** Whether the value is declared (an option or a configured column) rather than found in the data. */
  declared: boolean;
}

/**
 * Columns in order: the configured ones, else the field's options (with their
 * labels); then any value the loaded rows hold that is neither, so no card is
 * hidden; then an empty column when some row has no value.
 */
export function kanbanColumns(
  rows: Record<string, any>[],
  field: string,
  source: { options: string[]; labels?: string[]; columns?: string[] },
): KanbanColumn[] {
  const labelOf = (v: string) => {
    const i = source.options.indexOf(v);
    return i >= 0 && source.labels?.[i] ? source.labels[i] : v;
  };
  const values = (source.columns?.length ? source.columns : source.options).filter((v) => v !== "");
  const out: KanbanColumn[] = [...new Set(values)].map((v) => ({ value: v, label: labelOf(v), declared: true }));
  const seen = new Set(out.map((c) => c.value));
  let empty = false;
  for (const row of rows) {
    const v = kanbanValue(row, field);
    if (v === "") { empty = true; continue; }
    if (!seen.has(v)) { seen.add(v); out.push({ value: v, label: labelOf(v), declared: false }); }
  }
  if (empty) out.push({ value: "", label: "", declared: false });
  return out;
}

export function kanbanValue(row: Record<string, any>, field: string): string {
  const v = row[field];
  return v === null || v === undefined ? "" : String(v);
}

/** Rows per column value, keeping the order the rows were loaded in. */
export function groupKanbanRows<T extends Record<string, any>>(rows: T[], field: string): Map<string, T[]> {
  const groups = new Map<string, T[]>();
  for (const row of rows) {
    const v = kanbanValue(row, field);
    const group = groups.get(v) || [];
    group.push(row);
    groups.set(v, group);
  }
  return groups;
}

/** The rows with one card moved to another column; the same array when nothing changes. */
export function moveKanbanRow<T extends Record<string, any>>(rows: T[], name: string, field: string, value: string): T[] {
  const i = rows.findIndex((r) => r.name === name);
  if (i < 0 || kanbanValue(rows[i], field) === value) return rows;
  const next = rows.slice();
  next[i] = { ...rows[i], [field]: value === "" ? null : value };
  return next;
}

/** Whether a card may be dragged: a writable field, and a draft document. */
export function canDragKanban(row: Record<string, any>, writable: boolean): boolean {
  return writable && Number(row.docstatus || 0) === 0;
}
