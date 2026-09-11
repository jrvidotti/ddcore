// @ddcore/desk-sdk — types for app client scripts (*.form.ts and client/*.ts).
// At runtime the ddcore bundler resolves this module to window.__ddcoreDesk,
// implemented in desk/src/lib/desk-sdk.ts.
import type { BaseDoc, FieldDef, Filters } from "@ddcore/sdk";

export interface Frm<T extends BaseDoc = BaseDoc> {
  doc: T;
  doctype: string;
  meta: { doctype: any; children: Record<string, any>; permissions: Record<string, boolean> };
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
  toggleDisplay(fieldname: string, show: boolean): void;
  toggleReqd(fieldname: string, reqd: boolean): void;
  toggleEnable(fieldname: string, enable: boolean): void;
  addButton(label: string, action: () => any, group?: string): this;
  removeButton(label: string): void;
  setPrimaryAction(label: string, action: () => any): void;
  setInnerGroupAsPrimary(group: string): void;
  addIndicator(label: string, color?: string): void;
  addChild(fieldname: string, values?: Record<string, any>): any;
  removeChild(fieldname: string, idx: number): void;
  trigger(fieldname: string): Promise<void>;
  save(): Promise<boolean>;
  submit(): Promise<boolean>;
  cancel(): Promise<boolean>;
  reload(): Promise<void>;
  /** calls a controller method (POST /api/resource/:doctype/:name/:method) and reloads the doc */
  call(method: string, args?: Record<string, any>, opts?: { freeze?: boolean; reload?: boolean }): Promise<any>;
}

export interface FormHandlers<T extends BaseDoc = BaseDoc> {
  setup?: (frm: Frm<T>) => void;
  onload?: (frm: Frm<T>) => void;
  refresh?: (frm: Frm<T>) => void;
  validate?: (frm: Frm<T>) => void | boolean;
  beforeSave?: (frm: Frm<T>) => void;
  afterSave?: (frm: Frm<T>) => void;
  onChange?: { [K in keyof T & string]?: (frm: Frm<T>) => void } & Record<string, (frm: Frm<T>, cdt?: string, cdn?: string) => void>;
}

export interface DialogHandle {
  values: Record<string, any>;
  setValue(f: string, v: any): void;
  getValue(f: string): any;
  setHtml(f: string, html: string): void;
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
  onChange?: (fieldname: string, values: Record<string, any>, dialog: DialogHandle) => void;
  message?: string;
  size?: "sm" | "md" | "lg";
}

export interface DeskAPI {
  _(s: string, args?: any[]): string;
  __(s: string, args?: any[]): string;
  call(path: string, args?: Record<string, any>): Promise<any>;
  db: {
    getValue(doctype: string, name: string | Record<string, any>, field: string): Promise<any>;
    getValue(doctype: string, name: string | Record<string, any>, fields: string[]): Promise<Record<string, any> | null>;
    getList(doctype: string, args?: { filters?: Filters; fields?: string[]; orderBy?: string; limit?: number; start?: number }): Promise<any[]>;
    count(doctype: string, filters?: Filters): Promise<number>;
    getDoc(doctype: string, name: string): Promise<any>;
    setValue(doctype: string, name: string, values: Record<string, any>): Promise<any>;
    insert(doc: Record<string, any> & { doctype: string }): Promise<any>;
  };
  ui: {
    Dialog(spec: DialogSpec): DialogHandle;
    dialog(spec: DialogSpec): DialogHandle;
    msgprint(message: string, opts?: { title?: string; indicator?: string }): void;
    alert(message: string): void;
    confirm(message: string, title?: string): Promise<boolean>;
    prompt(title: string, fields: FieldDef[], primaryLabel?: string): Promise<Record<string, any> | null>;
    showError(e: any): void;
    toast(message: string, opts?: { title?: string; indicator?: string; timeout?: number }): void;
  };
  format: {
    currency(v: any): string;
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

export declare function defineForm<T extends BaseDoc = BaseDoc>(doctype: string, handlers: FormHandlers<T>): void;
/** Ajustes da listagem de um DocType (ver docs/agent/form-api.md). */
export interface ListViewOptions<T extends BaseDoc = BaseDoc> {
  /** Colunas exibidas, no lugar do `inListView` da meta. */
  columns?: (keyof T & string)[];
  /** Filtros iniciais; a query string da URL ainda tem precedência. */
  filters?: Partial<Record<keyof T & string, any>>;
  /** Ordenação inicial, ex.: "vencimento asc". */
  orderBy?: string;
  /** Tamanho de página inicial. */
  pageSize?: number;
  /** Texto da célula (texto puro, não HTML). */
  formatters?: Partial<Record<keyof T & string, (value: any, row: T) => string>>;
  /** Substitui a coluna de status; devolva null para não mostrar nada. */
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
}

export declare function defineListView<T extends BaseDoc = BaseDoc>(doctype: string, opts: ListViewOptions<T>): void;
export declare const ddcore: DeskAPI;
export declare const _: DeskAPI["_"];

declare global {
  const __: (s: string, args?: any[]) => string;
}
