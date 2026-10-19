// The state a grid keeps over rows it already holds: sort, preset filters, search and selection.
import { filterRows, nextSort, searchRows, sortRows, type GridSortState } from "./grid-rows";
import type { GridFilter } from "./meta";
import { getLinkTitle } from "./titles.svelte";

interface Source { rows: () => any[]; columns: () => any[]; presets: () => GridFilter[]; search: () => string[]; baseSort: () => GridSortState | null }

export class GridRowsState {
  userSort = $state<GridSortState | null>(null);
  query = $state("");
  selected = $state<Set<any>>(new Set());
  #activeFilters = $state<Set<number> | null>(null);
  #src!: Source;

  constructor(src: Source) { this.#src = src; }

  sort = $derived(this.userSort ?? this.#src.baseSort());
  active = $derived(this.#activeFilters ?? new Set(this.#src.presets().flatMap((f, i) => (f.default ? [i] : []))));
  // selection, "select all", totals and export follow the rows on screen
  visibleRows = $derived(searchRows(
    filterRows(this.#src.rows(), [...this.active].map((i) => this.#src.presets()[i]?.filters).filter(Boolean), this.#src.columns()),
    this.query, this.#src.search(), this.#src.columns(), getLinkTitle));
  viewRows = $derived(sortRows(this.visibleRows, this.sort, this.#src.columns(), getLinkTitle));
  // only rows still in the table count: a removed row, or every row after a reload, drops out
  chosen = $derived(this.visibleRows.filter((r) => this.selected.has(r)));
  chosenOnScreen = $derived(this.viewRows.filter((r) => this.selected.has(r)));
  allSelected = $derived(this.visibleRows.length > 0 && this.chosen.length === this.visibleRows.length);
  /** What export takes: the selection, else every row on screen. */
  forExport = $derived(this.chosen.length ? this.chosenOnScreen : this.viewRows);

  toggleSort(field: string) { this.userSort = nextSort(this.sort, field, this.#src.baseSort()); }
  toggle(row: any) { const s = new Set(this.selected); if (s.has(row)) s.delete(row); else s.add(row); this.selected = s; }
  toggleAll(on: boolean) { this.selected = on ? new Set(this.visibleRows) : new Set(); }
  clearSelection() { this.selected = new Set(); }
  toggleFilter(i: number) { const s = new Set(this.active); if (s.has(i)) s.delete(i); else s.add(i); this.#activeFilters = s; }
}
