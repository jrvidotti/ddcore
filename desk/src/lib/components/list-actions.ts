import type { ListAction, ListToolbarAction } from "$lib/desk-sdk";
import type { GridAction } from "$lib/form.svelte";

/** Whether a row passes an action's `condition`; one that throws counts as false. */
function passes(action: { label: string; condition?: (row: any) => boolean }, row: any, kind: string): boolean {
  if (!action.condition) return true;
  try {
    return !!action.condition(row);
  } catch (e) {
    console.error(`${kind} action "${action.label}" condition`, e);
    return false;
  }
}

/** The selected rows, in list order, that pass the action's `condition`; one that throws counts as false. */
export function actionRows<T extends { id: string }>(action: ListAction<any>, rows: T[], selected: Set<string>): T[] {
  return rows.filter((row) => selected.has(row.id) && passes(action, row, "list"));
}

/** The actions at least one selected row applies to, each with those rows. */
export function visibleActions<T extends { id: string }>(actions: ListAction<any>[], rows: T[], selected: Set<string>) {
  if (!selected.size) return [];
  return actions.map((action) => ({ action, rows: actionRows(action, rows, selected) })).filter((a) => a.rows.length > 0);
}

/** The toolbar actions whose `condition` is missing or true; one that throws hides its button. */
export function visibleToolbarActions(actions: ListToolbarAction[]): ListToolbarAction[] {
  return actions.filter((action) => {
    if (!action.condition) return true;
    try {
      return !!action.condition();
    } catch (e) {
      console.error(`list toolbar action "${action.label}" condition`, e);
      return false;
    }
  });
}

/** A grid's actions at least one of the selected rows (in the order given) applies to, each with those rows. */
export function visibleGridActions<T>(actions: GridAction<T>[], selected: T[]) {
  if (!selected.length) return [];
  return actions.map((action) => ({ action, rows: selected.filter((row) => passes(action, row, "grid")) })).filter((a) => a.rows.length > 0);
}
