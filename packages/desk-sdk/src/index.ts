// @ddcore/desk-sdk — types for app client scripts (*.form.ts and client/*.ts).
// At runtime the ddcore bundler resolves this module to window.__ddcoreDesk,
// implemented in desk/src/lib/desk-sdk.ts.
import type { BaseDoc, FieldDef, FieldWidth, Filters } from "@ddcore/sdk";
export type { FieldWidth };

/**
 * A button rendered inside a field's control, beside its input — an action on
 * *this field*, where `addButton` puts an action on the document. The desk owns
 * the markup, so the label is escaped for you and the field's own
 * `hidden`/`dependsOn` decide whether the button is on screen at all.
 */
export interface FieldButton {
  /** visible text, or the accessible name and tooltip when `icon` is set */
  label: string;
  /** an icon name: renders the button icon-only, with `label` as its tooltip */
  icon?: string;
  onClick: () => any;
  /** identity within the field; a second call with the same key replaces the button */
  key?: string;
}

export interface Frm<T extends BaseDoc = BaseDoc> {
  doc: T;
  doctype: string;
  meta: { doctype: any; children: Record<string, any>; permissions: Record<string, boolean> };
  /** `false` on a Single, even before its first save. */
  readonly isNew: boolean;
  readonly isDirty: boolean;
  readonly docstatus: number;
  readonly perm: Record<string, boolean>;
  isNewDoc(): boolean;
  getValue(field: string): any;
  setValue(field: string | Record<string, any>, value?: any): this;
  set(field: string, value: any): this;
  field(fieldname: string): FieldDef | undefined;
  setDfProperty(fieldname: string, prop: string, value: any): void;
  setQuery(fieldname: string, fn: () => { filters?: Filters }): void;
  /** re-runs a Report field's report; it also re-runs by itself after a save or a reload */
  refreshField(fieldname: string): void;
  toggleDisplay(fieldname: string, show: boolean): void;
  toggleReqd(fieldname: string, reqd: boolean): void;
  toggleEnable(fieldname: string, enable: boolean): void;
  addButton(label: string, action: () => any, group?: string): this;
  removeButton(label: string): void;
  /** attaches a button to a field's control; adding twice with the same `key` replaces it */
  addFieldButton(fieldname: string, button: FieldButton): this;
  /** removes one button by `key`, or every button on the field when no key is given */
  removeFieldButton(fieldname: string, key?: string): void;
  setPrimaryAction(label: string, action: () => any): void;
  setInnerGroupAsPrimary(group: string): void;
  addIndicator(label: string, color?: string): void;
  addChild(fieldname: string, values?: Record<string, any>): any;
  removeChild(fieldname: string, idx: number): void;
  /**
   * sets fields of one row of a Table, found by the row object or its id: the
   * grid, the dirty state and the table's onChange follow, as after an edit in the grid
   */
  setRowValue(table: string, row: any, field: string | Record<string, any>, value?: any): this;
  trigger(fieldname: string, cdt?: string, cdn?: string, row?: any): Promise<void>;
  save(): Promise<boolean>;
  submit(): Promise<boolean>;
  cancel(): Promise<boolean>;
  reload(): Promise<void>;
  /** throws the unsaved edits away and goes back to the document as it was loaded */
  discardChanges(): Promise<void>;
  /** calls a controller method (POST /api/resource/:doctype/:id/:method) and reloads the doc */
  call(method: string, args?: Record<string, any>, opts?: { freeze?: boolean; reload?: boolean }): Promise<any>;
}

export interface FormHandlers<T extends BaseDoc = BaseDoc> {
  setup?: (frm: Frm<T>) => void;
  onload?: (frm: Frm<T>) => void;
  refresh?: (frm: Frm<T>) => void;
  validate?: (frm: Frm<T>) => void | boolean;
  beforeSave?: (frm: Frm<T>) => void;
  afterSave?: (frm: Frm<T>) => void;
  /**
   * keyed by fieldname. On a Table it fires for a change in any of its rows:
   * cdt is the child DocType, cdn the row's id (none before its first save) and
   * row the row itself; adding or removing a row fires it with none of the three
   */
  onChange?: { [K in keyof T & string]?: (frm: Frm<T>) => void } & Record<string, (frm: Frm<T>, cdt?: string, cdn?: string, row?: any) => void>;
  /** per Table field: `onCellClick` runs when a read-only cell of that child field is clicked */
  grids?: Record<string, { onCellClick?: Record<string, (frm: Frm<T>, row: any) => void> }>;
}

export interface DialogHandle {
  values: Record<string, any>;
  setValue(f: string, v: any): void;
  getValue(f: string): any;
  setHtml(f: string, html: string): void;
  setDfProperty(fieldname: string, prop: string, value: any): void;
  hide(): void;
  show(): void;
  busy: boolean;
}

export interface DialogSpec {
  title: string;
  fields?: FieldDef[];
  values?: Record<string, any>;
  primaryLabel?: string;
  secondaryLabel?: string;
  primaryAction?: (values: Record<string, any>, dialog: DialogHandle) => any;
  dangerLabel?: string;
  dangerAction?: (values: Record<string, any>, dialog: DialogHandle) => any;
  /** the Delete key runs the danger action too, and its button shows the key */
  dangerShortcut?: boolean;
  onChange?: (fieldname: string, values: Record<string, any>, dialog: DialogHandle) => void;
  message?: string;
  size?: "sm" | "md" | "lg";
}

/** A persisted notification visible to the current authenticated user. */
export interface DeskNotification {
  id: string;
  title: string;
  message: string;
  creation: string;
  read: boolean;
  reference_doctype: string;
  reference_id: string;
}
export interface NotificationListOptions { limit?: number; offset?: number; read?: boolean }
export interface NotificationPage { data: DeskNotification[]; total: number }

/** A ToDo document representing an assignment or pending task. */
export interface ToDoDoc {
  id: string;
  status: "Open" | "Closed" | "Cancelled";
  priority: "Low" | "Medium" | "High" | "Urgent";
  date?: string;
  allocated_to: string;
  assigned_by: string;
  description?: string;
  reference_type?: string;
  reference_id?: string;
  creation?: string;
  modified?: string;
}
/** One user's share on one document (SEC-03). */
export interface DocShare {
  id: string;
  user: string;
  share_doctype: string;
  share_id: string;
  read: boolean;
  write: boolean;
  share: boolean;
  override_scope: boolean;
  owner: string;
}
export interface DocSharesInfo {
  shares: DocShare[];
  canShare: boolean;
  canOverrideScope: boolean;
}
export interface ShareArgs {
  user: string;
  write?: boolean;
  share?: boolean;
  overrideScope?: boolean;
}
export interface AssignArgs {
  allocated_to: string;
  date?: string;
  priority?: "Low" | "Medium" | "High" | "Urgent";
  description?: string;
}
export interface PendingWorkOptions {
  limit?: number;
  offset?: number;
  status?: string;
  scope?: "assigned_to_me" | "assigned_by_me";
  priority?: string;
  /** Inclusive bounds on the due date. */
  date_from?: string;
  date_to?: string;
  /** 1 keeps only the tasks without a due date. */
  no_date?: number;
  /** 1 keeps the tasks without a due date too, alongside date_from/date_to. */
  undated?: number;
  /** The counterpart: the assigner in assigned_to_me, the assignee in assigned_by_me. */
  user?: string;
  /** Matches the description and the referenced document. */
  q?: string;
  /** "field asc|desc" on date, priority, status, description, creation, modified, allocated_to or assigned_by. */
  order_by?: string;
}
export interface PendingWorkPage {
  data: ToDoDoc[];
  total: number;
  /** Link titles of the page's rows (the users), by doctype then id. */
  titles?: Record<string, Record<string, string>>;
}

export interface DeskAPI {
  notifications: {
    list(options?: NotificationListOptions): Promise<NotificationPage>;
    count(): Promise<number>;
    setRead(id: string, read: boolean): Promise<DeskNotification>;
  };
  assignments: {
    assign(doctype: string, id: string, args: AssignArgs): Promise<ToDoDoc>;
    complete(id: string): Promise<ToDoDoc>;
    revoke(id: string): Promise<{ success: boolean }>;
    reopen(id: string): Promise<ToDoDoc>;
    forDoc(doctype: string, id: string): Promise<ToDoDoc[]>;
    pending(options?: PendingWorkOptions): Promise<PendingWorkPage>;
  };
  /** Document sharing (SEC-03). Read is always granted; the server checks the sharer. */
  shares: {
    forDoc(doctype: string, id: string): Promise<DocSharesInfo>;
    add(doctype: string, id: string, args: ShareArgs): Promise<DocShare>;
    remove(doctype: string, id: string, user: string): Promise<{ ok: boolean }>;
  };
  /** Global search (OPS-08): documents the user can read whose id, title or search fields contain txt. */
  search: {
    global(txt: string, limit?: number): Promise<GlobalSearchHit[]>;
  };
  _(s: string, args?: any[]): string;
  __(s: string, args?: any[]): string;
  call(path: string, args?: Record<string, any>): Promise<any>;
  /** Runs a `defineReport` (GET /api/report/:name): its columns and rows, and whether the user may export them. */
  report(name: string, filters?: Record<string, any>): Promise<{ meta: { canExport?: boolean; [k: string]: any }; result: { columns: any[]; rows: any[]; [k: string]: any } }>;
  db: {
    getValue(doctype: string, id: string | Record<string, any>, field: string): Promise<any>;
    getValue(doctype: string, id: string | Record<string, any>, fields: string[]): Promise<Record<string, any> | null>;
    getList(doctype: string, args?: { filters?: Filters; fields?: string[]; orderBy?: string; limit?: number; start?: number }): Promise<any[]>;
    count(doctype: string, filters?: Filters): Promise<number>;
    getSingle(doctype: string): Promise<any>;
    getDoc(doctype: string, id: string): Promise<any>;
    setValue(doctype: string, id: string, values: Record<string, any>): Promise<any>;
    insert(doc: Record<string, any> & { doctype: string }): Promise<any>;
  };
  ui: {
    Dialog(spec: DialogSpec): DialogHandle;
    dialog(spec: DialogSpec): DialogHandle;
    msgprint(message: string, opts?: { title?: string; indicator?: string }): void;
    alert(message: string): void;
    /** `destructive`: the confirm button is a danger one and "No" is the primary, so Enter keeps the data and Delete confirms. */
    confirm(message: string, title?: string, opts?: { destructive?: boolean }): Promise<boolean>;
    prompt(title: string, fields: FieldDef[], primaryLabel?: string): Promise<Record<string, any> | null>;
    showError(e: any): void;
    toast(message: string, opts?: { title?: string; indicator?: string; timeout?: number }): void;
  };
  format: {
    currency(v: any, precision?: number): string;
    /**
     * Rounds a value exactly the way the server is about to store it: the
     * site's currency precision and its rounding rule.
     */
    roundCurrency(v: any, precision?: number): number;
    date(v: any): string;
    number(v: any, precision?: number): string;
    value(v: any, field?: Partial<FieldDef>): string;
    /**
     * Indicator colour for a status value: the field's `optionColors` first,
     * then the framework's canonical statuses, then a stable hash. Pass the
     * field so a declared colour wins.
     */
    statusColor(v: string, field?: Partial<FieldDef>): string;
  };
  datetime: { today(): string; addMonths(d: string, n: number): string; addDays(d: string, n: number): string; monthStart(d?: string): string; monthEnd(d?: string): string };
  meta(doctype: string): Promise<any>;
  route(path: string): Promise<void>;
  setRoute(...parts: string[]): Promise<void>;
}

export interface GlobalSearchHit {
  doctype: string;
  /** The DocType's translated label. */
  label: string;
  id: string;
  /** The title field's value, or the id. */
  title: string;
}

export declare function defineForm<T extends BaseDoc = BaseDoc>(doctype: string, handlers: FormHandlers<T>): void;
export type DeskViewMode = "list" | "calendar" | "cards" | "kanban" | "gantt" | "tree";

export interface CalendarViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: Date or Datetime field to plot records on the calendar */
  field: keyof T & string;
  /** Optional: Datetime or Date field for range spans */
  endField?: keyof T & string;
  /** Field shown as label inside the calendar chip (defaults to titleField or id) */
  titleField?: keyof T & string;
  /** Field determining chip color (e.g. "status", uses optionColors automatically) */
  colorField?: keyof T & string;
}

export interface CardViewOptions<T extends BaseDoc = BaseDoc> {
  /** Field shown as the card title (defaults to titleField or id) */
  title?: keyof T & string;
  /** Field shown under the title */
  subtitle?: keyof T & string;
  /** Date or Datetime shown in the footer (defaults to the first visible one) */
  dateField?: keyof T & string;
  /**
   * Attach Image (or Attach) field shown as a square thumbnail beside the title
   * (defaults to the DocType's `imageField`). When it is empty, fails to load or
   * is above the reader's permission level, the card shows the title's initials.
   */
  image?: keyof T & string;
  /** Indicator beside the title (defaults to the list's `indicator`, then `status`) */
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
  /** Badges in the footer (defaults to the list's `badges`) */
  badges?: (row: T) => { label: string; color: string }[] | null | undefined;
}

export interface KanbanViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: the Select field whose values are the columns; dragging a card writes it. */
  field: keyof T & string;
  /** Columns in order; defaults to the field's options. */
  columns?: string[];
  /** Field shown as the card title (defaults to titleField or id) */
  titleField?: keyof T & string;
  /** Field shown under the title */
  subtitleField?: keyof T & string;
  /** Field determining the card's indicator color (defaults to the column field) */
  colorField?: keyof T & string;
}

export interface GanttViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: Date or Datetime field where a bar starts */
  startField: keyof T & string;
  /** Required: Date or Datetime field where a bar ends; rows without one are not drawn */
  endField: keyof T & string;
  /** Field shown as the row label (defaults to titleField or id) */
  titleField?: keyof T & string;
  /** Field determining bar color (e.g. "status", uses optionColors automatically) */
  colorField?: keyof T & string;
  /** Numeric field from 0 to 100 drawn as progress inside the bar */
  progressField?: keyof T & string;
}

export interface TreeViewOptions<T extends BaseDoc = BaseDoc> {
  /**
   * The node label: a template such as "{acronym} - {title}" (its placeholders
   * are fetched; an empty one renders as nothing, with the empty brackets and
   * end separators it leaves) or a function of the row, which sees `id`, the
   * title field and `fields`. Defaults to the title field.
   */
  title?: string | ((row: T) => string);
  /** Extra fields a `title` function reads. */
  fields?: (keyof T & string)[];
  /** Order within each level, e.g. "title asc"; replaces the default groups-first order. */
  orderBy?: string;
}

/** Adjustments for a DocType's list view (see docs/agent/form-api.md). */
export interface ListViewOptions<T extends BaseDoc = BaseDoc> {
  /**
   * Allowed views for this DocType; defaults to ["list", "cards"] plus
   * "calendar", "kanban" and "gantt" for each one configured. A tree DocType
   * (`isTree`) gets "tree" as well, first and so by default.
   */
  views?: DeskViewMode[];
  calendar?: CalendarViewOptions<T>;
  card?: CardViewOptions<T>;
  kanban?: KanbanViewOptions<T>;
  gantt?: GanttViewOptions<T>;
  tree?: TreeViewOptions<T>;
  /** Displayed columns, overriding `inListView` from meta. */
  columns?: (keyof T & string)[];
  /** Initial filters; URL query string still takes precedence. */
  filters?: Partial<Record<keyof T & string, any>>;
  /** Initial sorting, e.g. "due_date asc". */
  orderBy?: string;
  /** Initial page size. */
  pageSize?: number;
  /** Cell text formatter (plain text, not HTML). */
  formatters?: Partial<Record<keyof T & string, (value: any, row: T) => string>>;
  /** Replaces the status column; return null to show nothing. */
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
  /** `false` hides the docstatus filter of a submittable DocType. Default `true`. */
  docstatusFilter?: boolean;
  /** `false` hides the trailing "Modified" column. Default `true`. */
  modifiedColumn?: boolean;
  /**
   * Shows (`true`) or hides (`false`) the leading document-id column. Left
   * out, the column is hidden when the id is a hash (or the title field is
   * the id and a column) and shown otherwise. Hidden, the row stays
   * clickable, and the title field's cell links to the document.
   */
  idColumn?: boolean;
  /** Fields fetched beyond the columns, for `indicator`, `badges` and `formatters`. */
  fields?: (keyof T & string)[];
  /** Extra indicators shown after the status, in the same cell. */
  badges?: (row: T) => { label: string; color: string }[] | null | undefined;
  /**
   * Choices appended after a divider to a standard Select filter, keyed by fieldname. Choosing
   * one applies its `filters` in place of `[field, "=", value]`.
   */
  filterOptions?: Partial<Record<keyof T & string, { value: string; label: string; filters: [string, string, any][] }[]>>;
}

export declare function defineListView<T extends BaseDoc = BaseDoc>(doctype: string, opts: ListViewOptions<T>): void;
export declare const ddcore: DeskAPI;
export declare const _: DeskAPI["_"];

declare global {
  const __: (s: string, args?: any[]) => string;
}
