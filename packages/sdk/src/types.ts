// Types shared by the meta model, the server bridge and the desk.

export type FieldType =
  | "Data" | "Email" | "Small Text" | "Text" | "Text Editor" | "Int" | "Float" | "Currency" | "Percent"
  | "Check" | "Date" | "Month" | "Datetime" | "Time" | "Select" | "Link" | "Dynamic Link" | "Table"
  | "Attach" | "JSON" | "Password" | "Vault" | "Section Break" | "Tab Break" | "HTML";

export type FieldWidth = "sm" | "md" | "lg" | "full";

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
  /**
   * Field permission level (0–9, default 0). A field above 0 is read and
   * written only by roles granted that level in `permissions` — the server
   * omits it from every response and refuses changes from anyone else, which
   * `hidden` and `readOnly` never do. It cannot be the title, naming or search
   * field. See `field-permissions`.
   */
  permlevel?: number;
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
  /**
   * How much of a form line the control takes: a form line is four slots, `sm`
   * and `md` take one, `lg` two and `full` all four. Fields fill each line in
   * order, so a form is laid out by sizing its fields, not by splitting it into
   * columns.
   *
   * Defaults by fieldtype, so a Date or a Percent is already right without a
   * declaration: `sm`/`md` for dates and numbers, `full` for text, JSON, tables
   * and HTML, `lg` for everything else.
   */
  width?: FieldWidth;
  /** Table editing mode; defaults to inline */
  gridEditMode?: "inline" | "dialog";
  collapsible?: boolean;
  bold?: boolean;
  /**
   * The fieldname this field used to have. `migrate` renames the column
   * instead of adding an empty one next to it, so the data survives.
   *
   * A list carries a chain: `["a", "b"]` on a field now called `c` still
   * reaches a database that stopped at `a`. Keep the declaration until every
   * environment has migrated — `ddcore doctor` lists what this database has
   * already applied. See `migrations`.
   */
  renamedFrom?: string | string[];
  /**
   * Authorises a column-type change that is not a widening to text.
   *
   * Without it `migrate` refuses the change rather than running a blind cast
   * that can abort the migration or truncate a value. `from` is the fieldtype
   * whose column the database still has, so the declaration cannot quietly
   * authorise a different conversion later — and it carries no SQL: a
   * conversion a plain cast cannot express goes through
   * expand → backfill → validate → contract. See `migrations`.
   */
  convert?: { from: FieldType };
}

/**
 * One piece of a message body. App code never writes HTML: it returns a list of
 * these, and the core renders both the plain-text and the HTML part from the
 * same list, escaping as it goes.
 */
export type MailBlock =
  | { type: "p"; text: string }
  | { type: "h"; text: string }
  | { type: "button"; text: string; url: string }
  | { type: "table"; head: string[]; rows: string[][] }
  | { type: "rule" };

/** The block builders handed to a template's `body`. */
export interface MailBlocks {
  /** A paragraph. */
  p(text: string): MailBlock;
  /** A heading. */
  h(text: string): MailBlock;
  /** A call to action. The plain-text part always spells the address out. */
  button(text: string, url: string): MailBlock;
  /** A table. Cells are stringified; the text part aligns the columns. */
  table(head: string[], rows: (string | number)[][]): MailBlock;
  /** A horizontal rule. */
  rule(): MailBlock;
}

/**
 * A message an app can send, declared in `mail/<name>.mail.ts`.
 *
 * Both `subject` and `body` run at delivery time, in the *reader's* language —
 * so every string in them must be a literal `_("…")` call. The extractor
 * collects `_(…)` by syntax: a string built through a helper is reported as
 * `dynamic`, `make check` sees nothing missing, and the message goes out in
 * English on a translated site.
 */
export interface MailTemplateDef<A = any> {
  /** Unique across every installed app, like a DocType name. */
  name: string;
  subject: (args: A) => string;
  body: (args: A, b: MailBlocks) => MailBlock[];
  /**
   * Declares that `args` carry a credential — a recovery link, a one-time
   * token — and must never be stored.
   *
   * A sensitive message keeps only its metadata in the delivery record: who it
   * went to, its subject, whether it arrived. Its arguments travel in the job
   * payload and nowhere else, and it cannot be re-rendered after the fact.
   *
   * Leaving this off a template that mails a credential writes that credential
   * into `tab_email_delivery`, where every System Manager can read it.
   */
  sensitive?: boolean;
}

/** What `ddcore.sendMail` takes. */
export interface SendMailArgs {
  /** The `name` of a registered mail template. */
  template: string;
  to: string | string[];
  /** Whatever `subject` and `body` read. Stored unless the template is sensitive. */
  args?: Record<string, any>;
  /** Overrides the reader's language. Defaults to the recipient's, then the site's. */
  lang?: string;
  /** `File` document names, or their `file_url`. Authorized against the caller. */
  attach?: string[];
  /** The document this message is about, for the delivery record. */
  reference?: { doctype: string; name: string };
  /**
   * Makes the send idempotent: a second call with the same key is refused by a
   * unique index rather than delivered twice.
   */
  key?: string;
}

/**
 * A block returned by a print template body.
 */
export interface PrintBlock {
  type: string;
  [key: string]: any;
}

/**
 * Vocabulary of blocks available to print templates.
 */
export interface PrintBlockBuilder {
  header(title: string, opts?: { subtitle?: string; badge?: string; badgeColor?: string }): PrintBlock;
  keyValues(pairs: [label: string, value: any][], opts?: { columns?: 2 | 3 | 4 }): PrintBlock;
  section(title?: string, blocks?: PrintBlock[]): PrintBlock;
  table(headers: string[], rows: (string | number | null | undefined)[][], opts?: { aligns?: ("left" | "right" | "center")[] }): PrintBlock;
  totals(rows: [label: string, value: string][]): PrintBlock;
  p(text: string): PrintBlock;
  h(level: 1 | 2 | 3 | 4, text: string): PrintBlock;
  h1(text: string): PrintBlock;
  h2(text: string): PrintBlock;
  h3(text: string): PrintBlock;
  rule(): PrintBlock;
  divider(): PrintBlock;
  pageBreak(): PrintBlock;
  raw(html: string): PrintBlock;
  html(html: string): PrintBlock;
  columns(cols: PrintBlock[][]): PrintBlock;
}

/**
 * Context helpers passed to print templates.
 */
export interface PrintContext {
  formatCurrency(val: any): string;
  formatDate(val: any): string;
  formatDateTime(val: any): string;
  formatNumber(val: any, decimals?: number): string;
}

/**
 * A document print template declared in `print/<name>.print.ts`.
 */
export interface PrintTemplateDef<T = any> {
  name: string;
  doctype: string;
  label: string;
  body: (doc: T, b: PrintBlockBuilder, ctx: PrintContext) => PrintBlock[];
}

export interface PermDef {
  role: string;
  read?: boolean; write?: boolean; create?: boolean; delete?: boolean;
  submit?: boolean; cancel?: boolean; amend?: boolean; report?: boolean; export?: boolean;
  ifOwner?: boolean;
  /**
   * The field level this row grants (default 0). A row above 0 grants only
   * `read` and `write` on that level's fields, and never access to the
   * document itself: pair it with a level-0 row for the same role.
   */
  permlevel?: number;
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

export interface UniqueKeyDef {
  /**
   * Names the key. ascii snake_case, unique within the DocType, and the
   * durable half of the index name (`tab_<snake>_uk_<name>`) — which is why
   * reordering or renaming a field does not rebuild the index, and why a
   * duplicate error can say which business key was violated.
   */
  name: string;
  /**
   * The columns the key spans, two or more. A row is constrained only when
   * *every* component has a value: leave one empty and the row is outside the
   * key, exactly as a `unique` field with no value is. See `fieldtypes`.
   */
  fields: string[];
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
  /**
   * Compound business keys, enforced by a partial unique index each.
   *
   * `unique` on a field covers one column; this covers the keys that span
   * several — `[{ name: "customer_invoice_no", fields: ["customer", "invoice_no"] }]`.
   * `migrate` creates, rebuilds and removes the index as the declaration
   * changes, and the database is what makes it hold under concurrency.
   *
   * Like `titleField` and `searchFields`, the field list names fields by
   * string, so a renamed field has to be changed here too.
   */
  uniqueKeys?: UniqueKeyDef[];
  fields: FieldDef[];
  permissions?: PermDef[];
  description?: string;
  icon?: string;
  /**
   * The name this DocType used to have. `migrate` renames the table, its
   * indexes and every stored reference to the old name, instead of leaving
   * the old table behind and creating an empty one.
   */
  renamedFrom?: string | string[];
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
  /** the language of this request */
  lang: string;
  /** every language the site serves — one per translations/<lang>.csv, plus "en" */
  langs: string[];
  /** in a job / migrate / test — no HTTP request */
  request?: { method: string; path: string; ip?: string };
  /**
   * Correlates this unit of work with the access log line, the Error Log row
   * and the `X-Request-Id` the caller saw. Log it alongside anything you want
   * to find again. Empty in a job, a migration or a test.
   */
  requestId?: string;
}

/**
 * What a patch receives: the session, plus the only write-SQL there is.
 *
 * `ddcore.db.sql` is read-only everywhere else. A patch gets `sql` because a
 * backfill over a real table cannot be a document-by-document loop through the
 * lifecycle: that rewrites `modified` on every row and stops on legacy data
 * that no longer validates.
 */
export interface PatchContext extends Context {
  /** Runs one statement, reads or writes, and returns the rows it produced. */
  sql(query: string, params?: any[]): Record<string, any>[];
}

/**
 * When a patch runs, relative to the schema change of the same migration.
 *
 * `beforeSchema` sees the old columns and is where you make the data fit what
 * the DDL is about to do. `afterSchema` (the default) sees the new ones, and
 * runs before `--prune` drops anything, so it can still read a column the same
 * migration is about to remove.
 */
export type PatchPhase = "beforeSchema" | "afterSchema";

export interface PatchDef {
  phase?: PatchPhase;
  /** one line, shown by `ddcore migrate` and `ddcore doctor` */
  description?: string;
  execute(ctx: PatchContext): void;
}

export type DocEvent =
  | "beforeValidate" | "validate" | "beforeSave" | "afterInsert" | "onUpdate"
  | "beforeSubmit" | "onSubmit" | "beforeCancel" | "onCancel" | "onUpdateAfterSubmit"
  | "onTrash" | "afterDelete" | "beforeRename" | "afterRename" | "beforeInsert";

export type DocHook<T> = (doc: T & Document<T>, ctx: Context) => void;

/**
 * A field one app adds to another app's DocType.
 *
 * `insertAfter` places it after an existing fieldname; without it the field
 * goes to the end, after the host's own.
 */
export interface ExtensionFieldDef extends FieldDef {
  insertAfter?: string;
}

/**
 * What one app changes on another app's DocType.
 *
 * Everything here is additive or an override of a property the host already
 * declares. Two apps changing the same property is a conflict, and refuses to
 * load: the effective meta must not depend on the order the apps happen to be
 * installed in.
 */
export interface ExtensionDef<T extends BaseDoc = BaseDoc> {
  /** custom fields — a fieldname the host (or another extension) already uses is an error */
  fields?: ExtensionFieldDef[];
  /** property setters, per field: `{ status: { reqd: true } }` */
  set?: Record<string, Partial<FieldDef>>;
  /** property setters on the DocType itself */
  doctype?: Partial<DoctypeDef>;
  /** extra role permissions; a role the host already lists is an error */
  permissions?: PermDef[];
  /** chained after the host's: any `false` denies, `undefined` is no opinion */
  hasPermission?: (doc: T, ptype: string, user: string) => boolean | undefined;
  /** AND-ed with the host's: an extension can only narrow what is visible */
  permissionQuery?: (user: string) => Filters | undefined;
}

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
  /** apps that must be installed (and load) before this one; core is implicit */
  requires?: string[];
  /** this app's own release, `MAJOR.MINOR.PATCH` */
  version?: string;
  /**
   * The ddcore releases this app supports, e.g. `">=0.14.0 <1.0.0"` or `"^0.14.0"`.
   * A binary outside the range refuses to load the app. See `docs/agent/conventions.md`.
   */
  ddcore?: string;
  docEvents?: Record<string, Partial<Record<DocEvent, (doc: BaseDoc & Document<any>, ctx: Context) => void>>>;
  scheduler?: {
    cron?: Record<string, string[]>;
    all?: string[]; hourly?: string[]; daily?: string[]; weekly?: string[]; monthly?: string[];
  };
  afterInstall?: (ctx: Context) => void;
  afterMigrate?: (ctx: Context) => void;
  desk?: {
    /** client scripts (relative to app dir) loaded in every desk page */
    include?: string[];
    /** the workspace `/app` opens on */
    home?: string;
    /**
     * The square mark shown beside the site's name in the sidebar and on the
     * sign-in screens: one letter or one emoji, not an image URL. The first
     * app in load order that declares one wins, as with `home`. Left out, the
     * mark is the initial of `title` above.
     */
    logo?: string;
  };
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
  /** runs a workflow action on the saved document, as the Desk's action buttons do */
  applyWorkflow(action: string): this;
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

/** Persistent notifications. Recipients are active User names, never external addresses. */
export interface NotificationDef<D = Record<string, any>> {
  name: string;
  doctype: string;
  /** Exactly one of event or date must be declared. */
  event?: "on_insert" | "on_update" | "on_submit" | "on_cancel";
  /** Offset in calendar days in the site's timezone; overdue matches are recovered. */
  date?: { field: string; days: number };
  condition?: (doc: D, before: D | null) => boolean;
  recipients: (doc: D, before: D | null) => string[];
  /** Plain text rendered in the recipient's language. */
  desk?: { title: (doc: D, before: D | null) => string; message: (doc: D, before: D | null) => string };
  email?: { template: string; args: (doc: D, before: D | null) => Record<string, any> };
}

export interface WorkflowStateDef {
  state: string;
  docstatus?: 0 | 1 | 2;
  allowEdit?: string;
  updateFields?: Record<string, any>;
}

export interface WorkflowTransitionDef<D = Record<string, any>> {
  state: string;
  action: string;
  nextState: string;
  allowed: string | string[];
  allowSelfApproval?: boolean;
  condition?: (doc: D) => boolean;
}

export interface WorkflowDef<D = Record<string, any>> {
  name: string;
  doctype: string;
  stateField?: string;
  initialState: string;
  states: WorkflowStateDef[];
  transitions: WorkflowTransitionDef<D>[];
}

