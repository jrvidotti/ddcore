import type { ListAction, ListToolbarAction } from "$lib/desk-sdk";

/** The selected rows, in list order, that pass the action's `condition`; one that throws counts as false. */
export function actionRows<T extends { id: string }>(action: ListAction<any>, rows: T[], selected: Set<string>): T[] {
  return rows.filter((row) => {
    if (!selected.has(row.id)) return false;
    if (!action.condition) return true;
    try {
      return !!action.condition(row);
    } catch (e) {
      console.error(`list action "${action.label}" condition`, e);
      return false;
    }
  });
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
