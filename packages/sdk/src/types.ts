// Types shared by the meta model, the server bridge and the desk.

export type FieldType =
  | "Data" | "Email" | "Small Text" | "Text" | "Text Editor" | "Int" | "Float" | "Currency" | "Percent"
  | "Check" | "Date" | "Month" | "Datetime" | "Time" | "Select" | "Link" | "Dynamic Link" | "Table"
  | "Attach" | "JSON" | "Password" | "Section Break" | "Column Break" | "Tab Break" | "HTML";

export interface FieldDef {
  fieldname?: string;
  fieldtype: FieldType;
  label?: string;
  /**
   * Link/Table/Dynamic Link: DocType or fieldname; Select: list of options.
   *
   * A Select's options are its canonical values — English, and what the
   * database holds. Their display text comes from the catalogue, so a
   * translation never changes what is stored or compared.
   */
  options?: string | string[];
  /**
   * Select only: indicator colour per option, keyed by the canonical value.
   *
   * Key it by the value, never by the label: a colour decided by the text a
   * reader happens to see is a colour that changes with the language.
   */
  optionColors?: Record<string, "blue" | "green" | "orange" | "red" | "purple" | "gray">;
  reqd?: boolean;
  unique?: boolean;
  default?: string | number | boolean;
  readOnly?: boolean;
  hidden?: boolean;
  /** "link_field.target_field" — copied from the linked doc on save */
  fetchFrom?: string;
  /** JS expression over `doc`, evaluated on desk and server */
  dependsOn?: string;
  readOnlyDependsOn?: string;
  mandatoryDependsOn?: string;
  allowOnSubmit?: boolean;
  inListView?: boolean;
  inStandardFilter?: boolean;
  searchIndex?: boolean;
  length?: number;
  precision?: number;
  description?: string;
  /** grid column width (1-12) */
  columns?: number;
  /** Table editing mode; defaults to inline */
  gridEditMode?: "inline" | "dialog";
  collapsible?: boolean;
  bold?: boolean;
}

export interface PermDef {
  role: string;
  read?: boolean; write?: boolean; create?: boolean; delete?: boolean;
  submit?: boolean; cancel?: boolean; amend?: boolean; report?: boolean; export?: boolean;
  ifOwner?: boolean;
}

export interface NamingDef {
  /** e.g. "CTR-.YYYY.-.####" */
  series?: string;
  field?: string;
  hash?: boolean;
  prompt?: boolean;
  /** e.g. "{imovel}-{locatario}" */
  format?: string;
}

export interface DoctypeDef {
  name: string;
  module?: string;
  label?: string;
  naming?: NamingDef;
  submittable?: boolean;
  isChild?: boolean;
  isSingle?: boolean;
  trackChanges?: boolean;
  allowRename?: boolean;
  titleField?: string;
  sortField?: string;
  sortOrder?: "asc" | "desc";
  searchFields?: string[];
  fields: FieldDef[];
  permissions?: PermDef[];
  description?: string;
  icon?: string;
}

export interface BaseDoc {
  doctype: string;
  name: string;
  owner?: string;
  creation?: string;
  modified?: string;
  modified_by?: string;
  docstatus: 0 | 1 | 2;
  __islocal?: boolean;
  __unsaved?: boolean;
  [key: string]: any;
}

export interface ChildDoc extends BaseDoc {
  parent: string;
  parenttype: string;
  parentfield: string;
  idx: number;
}

export type FilterOp = "=" | "!=" | ">" | ">=" | "<" | "<=" | "like" | "not like" | "in" | "not in" | "between" | "is" | "set" | "not set";
export type FilterTuple = [string, FilterOp, any] | [string, any] | [string, string, FilterOp, any];
export type Filters = FilterTuple[] | Record<string, any>;

export interface ListArgs {
  filters?: Filters;
  orFilters?: Filters;
  fields?: string[];
  orderBy?: string;
  limit?: number;
  start?: number;
  /** "count(name) as total" style aggregates are allowed in fields */
  groupBy?: string;
}

export interface ReportColumn {
  fieldname: string;
  label: string;
  fieldtype?: FieldType;
  options?: string;
  width?: number;
}

export interface ReportResult {
  columns: ReportColumn[];
  rows: Record<string, any>[];
  /** optional totals row */
  totals?: Record<string, any>;
  chart?: ChartData;
  /** summary cards shown above the table */
  summary?: { label: string; value: any; datatype?: "Currency" | "Int" | "Float" | "Data"; indicator?: string }[];
}

export interface ChartData {
  type: "bar" | "line" | "pie" | "donut";
  labels: string[];
  datasets: { name: string; values: number[] }[];
}

export interface ReportDef {
  name: string;
  label?: string;
  refDoctype?: string;
  roles?: string[];
  filters?: FieldDef[];
  execute: (filters: Record<string, any>, ctx: Context) => ReportResult;
}

export interface NumberCardDef {
  name: string;
  label: string;
  doctype?: string;
  filters?: Filters;
  /** "count" (default) or "sum:fieldname" */
  aggregate?: string;
  /** alternative: method returning { value, formatted?, color? } */
  method?: () => { value: number; formatted?: string; color?: string };
  color?: string;
  route?: string;
}

export interface ChartDef {
  name: string;
  label: string;
  type: "bar" | "line" | "pie" | "donut";
  method: () => ChartData;
}

export interface WorkspaceDef {
  name: string;
  label?: string;
  icon?: string;
  /** shown in the sidebar for these roles ("*" = all) */
  roles?: string[];
  shortcuts?: { label: string; doctype?: string; report?: string; route?: string; icon?: string; filters?: Filters }[];
  numberCards?: NumberCardDef[];
  charts?: ChartDef[];
  reports?: string[];
  /** grouped links for the workspace page */
  links?: { label: string; items: { label: string; doctype?: string; report?: string }[] }[];
  sidebar?: { label: string; doctype?: string; report?: string; route?: string; icon?: string }[];
}

export interface Context {
  user: string;
  roles: string[];
  lang: string;
  /** in a job / migrate / test — no HTTP request */
  request?: { method: string; path: string; ip?: string };
}

export type DocEvent =
  | "beforeValidate" | "validate" | "beforeSave" | "afterInsert" | "onUpdate"
  | "beforeSubmit" | "onSubmit" | "beforeCancel" | "onCancel" | "onUpdateAfterSubmit"
  | "onTrash" | "afterDelete" | "beforeRename" | "afterRename" | "beforeInsert";

export type DocHook<T> = (doc: T & Document<T>, ctx: Context) => void;

export interface ControllerDef<T extends BaseDoc = BaseDoc> extends Partial<Record<DocEvent, DocHook<T>>> {
  /** callable from the desk/API via POST /api/resource/:doctype/:name/:method */
  methods?: Record<string, (doc: T & Document<T>, args: Record<string, any>, ctx: Context) => any>;
  hasPermission?: (doc: T, ptype: string, user: string) => boolean | undefined;
  permissionQuery?: (user: string) => Filters | undefined;
  /** computed on list/form load, not stored */
  onLoad?: DocHook<T>;
}

export interface AppDef {
  name: string;
  title: string;
  description?: string;
  requires?: string[];
  version?: string;
  docEvents?: Record<string, Partial<Record<DocEvent, (doc: BaseDoc & Document<any>, ctx: Context) => void>>>;
  scheduler?: {
    cron?: Record<string, string[]>;
    all?: string[]; hourly?: string[]; daily?: string[]; weekly?: string[]; monthly?: string[];
  };
  afterInstall?: (ctx: Context) => void;
  afterMigrate?: (ctx: Context) => void;
  /** client scripts (relative to app dir) loaded in every desk page */
  desk?: { include?: string[]; home?: string; logo?: string };
  /** extra roles created on install */
  roles?: string[];
  fixtures?: Record<string, Record<string, any>[]>;
}

/** Methods every server-side document has. */
export interface Document<T = any> {
  insert(opts?: { ignorePermissions?: boolean }): this;
  save(opts?: { ignorePermissions?: boolean; ignoreVersion?: boolean }): this;
  submit(): this;
  cancel(): this;
  delete(): void;
  reload(): this;
  /** write columns directly, bypassing validate (allowed after submit) */
  dbSet(field: string | Record<string, any>, value?: any): this;
  append(fieldname: string, row?: Record<string, any>): ChildDoc;
  get(fieldname: string): any;
  set(fieldname: string, value: any): this;
  getDoc(): T;
  isNew(): boolean;
  hasValueChanged(fieldname: string): boolean;
  getDocBeforeSave(): T | undefined;
  runMethod(name: string, args?: Record<string, any>): any;
  toJSON(): any;
  flags: Record<string, any>;
}
