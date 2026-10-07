// Types shared by the meta model, the server bridge and the desk.

export type FieldType =
  | "Data" | "Email" | "Small Text" | "Text" | "Text Editor" | "Markdown Editor" | "Code"
  | "Int" | "Float" | "Currency" | "Percent" | "Rating" | "Duration" | "Color"
  | "Check" | "Date" | "Month" | "Datetime" | "Time" | "Select" | "Autocomplete" | "Barcode" | "Signature" | "Geolocation" | "Link" | "Dynamic Link" | "Table"
  | "Table MultiSelect" | "Attach" | "Attach Image" | "JSON" | "Password" | "Vault" | "Section Break" | "Tab Break" | "HTML" | "Report";

/** A GeoJSON position: `[longitude, latitude]`, longitude first. */
export type GeoPosition = [number, number];
export interface GeoPoint { type: "Point"; coordinates: GeoPosition }
export interface GeoMultiPoint { type: "MultiPoint"; coordinates: GeoPosition[] }
export interface GeoLineString { type: "LineString"; coordinates: GeoPosition[] }
/** Rings of positions, each closed: its last position repeats the first. */
export interface GeoPolygon { type: "Polygon"; coordinates: GeoPosition[][] }
export type GeoGeometry = GeoPoint | GeoMultiPoint | GeoLineString | GeoPolygon;
/** A stored feature keeps no properties: they are always `{}`. */
export interface GeoFeature { type: "Feature"; geometry: GeoGeometry; properties: Record<string, never> }
/**
 * The value of a Geolocation field, as the server stores it: a GeoJSON
 * FeatureCollection with at least one feature (an empty one is `null`),
 * coordinates rounded to 7 decimals, no altitude.
 */
export interface GeoFeatureCollection { type: "FeatureCollection"; features: GeoFeature[] }

export type FieldWidth = "sm" | "md" | "lg" | "full";

/** One preset filter of a form grid (see `FieldDef.gridFilters`). */
export interface GridFilter {
  label: string;
  /** `[["in_class", "=", 1]]` or `{ in_class: 1 }`; no tree operators */
  filters: [string, FilterOp, any][] | [string, any][] | Record<string, any>;
  default?: boolean;
}

export interface FieldDef {
  fieldname?: string;
  fieldtype: FieldType;
  label?: string;
  /**
   * Link/Table/Dynamic Link: DocType or fieldname; Select: list of options.
   * Table MultiSelect: a child DocType with exactly one Link field, which
   * holds each chosen value.
   *
   * A Select's options are its canonical values — English, and what the
   * database holds. Their display text comes from the catalogue, so a
   * translation never changes what is stored or compared.
   *
   * Report: the name of a `defineReport`, shown as a grid inside the form.
   *
   * The other types that read it: `Rating` takes the number of stars (1–10,
   * default 5), `Code` the language (`"sql"`, `"ts"`) and `Duration` the
   * display flags `["hideDays", "hideSeconds"]`. `Autocomplete` takes the
   * suggestions, a list or one per line — free text is still accepted, and
   * `frm.setDfProperty(field, "options", list)` replaces them at runtime.
   * `Barcode` takes its symbology: `"Code128"` (the default), `"EAN-13"` or
   * `"QR"`. `Signature` takes none: its value is a PNG data URL drawn on the
   * form. `Geolocation` takes none either: its value is a
   * `GeoFeatureCollection`.
   * None of those are catalogue keys — they are never translated.
   */
  options?: string | string[] | number;
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
   * `hidden` and `readOnly` never do. It cannot be the title, idGeneration or search
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
  /**
   * The value cannot change once the document exists — refused by the server
   * on every write path (save, REST, `setValue`/`dbSet`, import). An empty
   * value may be filled once. The desk shows the field read-only after insert.
   */
  setOnlyOnce?: boolean;
  /**
   * The desk's Duplicate leaves the value behind: the copy takes the field's
   * default. A `unique` field, and a `readOnly` one without `fetchFrom`, are
   * never copied anyway. On a Table, the copy has no rows.
   */
  noCopy?: boolean;
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
  /**
   * Table or Report: the order the grid shows its rows in. `field` is a child
   * field (or `idx`) on a Table, a report column on a Report. Display only: a
   * child row's `idx` stays what the user saved.
   */
  gridSort?: { field: string; order?: "asc" | "desc" };
  /** Table or Report: clicking a column header sorts the grid by it */
  gridSortable?: boolean;
  /** Table or Report: CSV/XLSX export of the grid, for users who may export the DocType */
  gridExport?: boolean;
  /** Table or Report: row checkboxes; a Table also gets "Delete selected" */
  gridSelect?: boolean;
  /**
   * Table or Report: preset toggles above the grid, each showing only the rows
   * that match its `filters` (list-style tuples or an object, evaluated in the
   * browser on the loaded rows, `computed` fields included). Toggles that are
   * on combine with AND; `default: true` turns one on when the form opens.
   * Display only: rows and their `idx` are untouched. `label` is a catalogue key.
   */
  gridFilters?: GridFilter[];
  /**
   * Table or Report: a search box above the grid that shows only the rows
   * where one of these columns contains the text typed (case- and
   * accent-insensitive; each word must match some column). A Link matches by
   * its id and its title, a Select by its value and its label. On a Table the
   * fields may be hidden or off the grid. Combines with `gridFilters` (AND);
   * display only, like them.
   */
  gridSearch?: string[];
  /** Table only: `false` hides the `#` column (each row's stored `idx`) */
  gridIndex?: boolean;
  /**
   * Report only: the report's filters, each taking the value of a field of
   * this document (or `id`). `{ course: "id" }` runs the report for this course.
   */
  reportFilters?: Record<string, string>;
  /**
   * No column: never stored, always read-only. The controller's `onLoad`
   * sets its value each time the form loads — on the parent or on a child
   * row — from other documents, say. It cannot be filtered or sorted on in a
   * list; in a Table grid it sorts and exports like any column.
   */
  computed?: boolean;
  /**
   * Attach / Attach Image: show the file's name next to its icon or thumbnail.
   * Off by default — uploads are stored under a random name, and the icon or
   * thumbnail already opens the file and shows its name, size and type on hover.
   */
  showFileName?: boolean;
  collapsible?: boolean;
  bold?: boolean;
  /**
   * Keeps the label off the form (screen readers still get it). It still
   * names the field in exports, the row dialog and error messages. Not for
   * a Section or Tab Break.
   */
  hideLabel?: boolean;
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
/** What a share grants. `read` is implied; `overrideScope` needs an unscoped System Manager. */
export interface ShareRights {
  read?: boolean;
  write?: boolean;
  share?: boolean;
  overrideScope?: boolean;
}

/** One user's share on one document (the `Document Share` row). */
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

export interface DocShares {
  shares: DocShare[];
  canShare: boolean;
  canOverrideScope: boolean;
}

export interface SendMailArgs {
  /** The `name` of a registered mail template. */
  template: string;
  to: string | string[];
  /** Whatever `subject` and `body` read. Stored unless the template is sensitive. */
  args?: Record<string, any>;
  /** Overrides the reader's language. Defaults to the recipient's, then the site's. */
  lang?: string;
  /** `File` document ids, or their `file_url`. Authorized against the caller. */
  attach?: string[];
  /** The document this message is about, for the delivery record. */
  reference?: { doctype: string; id: string };
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
  /** A Text Editor value, rendered as markup and cleaned by the server's allowlist. */
  richText(html: string, title?: string): PrintBlock;
  /** A Markdown Editor source, rendered and cleaned the same way. */
  markdown(text: string, title?: string): PrintBlock;
  /** Preformatted text, escaped, keeping its whitespace (a Code field). */
  pre(text: string, title?: string): PrintBlock;
  /**
   * A barcode drawn as vectors: `symbology` is `"Code128"` (the default),
   * `"EAN-13"` or `"QR"` — a Barcode field's is `field.options`. A value the
   * symbology refuses prints as text.
   */
  barcode(value: string, symbology?: "Code128" | "EAN-13" | "QR", title?: string): PrintBlock;
  /**
   * A Signature field's image, at most 25 mm tall. Anything but a PNG data URL
   * within the field's limits prints as nothing.
   */
  signature(dataUrl: string, title?: string): PrintBlock;
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
  /**
   * Lets the role load rows from a CSV/XLSX file (see `docs/agent/data-import.md`).
   * Inserting also needs `create`, updating needs `write`.
   */
  import?: boolean;
  /** Lets the role share one document with another user (see `docs/agent/sharing.md`). */
  share?: boolean;
  ifOwner?: boolean;
  /**
   * The field level this row grants (default 0). A row above 0 grants only
   * `read` and `write` on that level's fields, and never access to the
   * document itself: pair it with a level-0 row for the same role.
   */
  permlevel?: number;
}

/** How a new document gets its `id`. With none of these set, it is a random hash. */
export interface IDGenerationDef {
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

/** The sources of a virtual DocType. */
export interface VirtualDef {
  sources: VirtualSourceDef[];
}

/** One DocType feeding a virtual DocType. */
export interface VirtualSourceDef {
  doctype: string;
  /**
   * Virtual fieldname → this source's fieldname. A virtual field left out
   * reads as null on this source's rows. The column types must match, and a
   * Link must map to a Link to the same DocType.
   */
  fields: Record<string, string>;
}

export interface DoctypeDef {
  name: string;
  module?: string;
  label?: string;
  /**
   * What the desk calls the document's `id`, a catalogue key like `label`:
   * `"Contract No."` heads the list's id column instead of "ID". Display
   * only — filters, `orderBy` and the API still address it as `id`.
   */
  idLabel?: string;
  idGeneration?: IDGenerationDef;
  submittable?: boolean;
  isChild?: boolean;
  isSingle?: boolean;
  /**
   * A hierarchical DocType (DAT-07): its documents form a tree through a
   * self-referencing Link — `parentField`, `parent_<snake(name)>` by default —
   * and only a document with `is_group` set may have children. Both fields are
   * added for you unless you declare them yourself, which is how you place or
   * relabel them. See `trees`.
   */
  isTree?: boolean;
  /** The Link field holding the parent; `parent_<snake(name)>` by default. Needs `isTree`. */
  parentField?: string;
  /**
   * On a site with `tenancy` on, every DocType belongs to a tenant: each
   * tenant has its own documents, ids, numbering and unique values. `shared`
   * opts out — one set of documents for the whole site (countries, units),
   * readable from every tenant and written only from the platform space. A
   * shared DocType cannot Link to a tenant-owned one. A child table follows
   * the DocTypes that use it. Ignored without tenancy. See `tenancy`.
   */
  shared?: boolean;
  /**
   * On a site with `tenancy` on: `"tenant"` keeps a tenant-owned DocType out of
   * the platform space altogether. There it is left out of boot (no menu entry,
   * search row or Link), and every read and write is refused with a message
   * that says to enter a tenant (`--tenant` on the command line), so nothing
   * lands where no tenant sees it. Inside a tenant, `ddcore.tenant.run`, a job
   * enqueued from inside a tenant and a migration patch are unaffected.
   * `"any"` opts out of the app's `space: "tenant"` default. Not for a shared,
   * child or virtual DocType. Ignored without tenancy. See `tenancy`.
   */
  space?: "tenant" | "any";
  /**
   * A virtual DocType (DAT-07): no table and no writes; its rows are the union
   * of its sources' documents, each read through that source's own
   * permissions. A row's id — and the value a Link to this DocType stores — is
   * `"<Source DocType>:<source id>"`. A `source_doctype` field is added unless
   * you declare it. See `virtual-doctypes`.
   */
  virtual?: VirtualDef;
  trackChanges?: boolean;
  allowRename?: boolean;
  titleField?: string;
  /**
   * Shows the id translated: the desk treats it as a catalogue key wherever it
   * shows the document's title — a Link, a grid cell, the list, the form
   * header — and a Link search also matches the translated text. The stored
   * value stays the canonical English id. For DocTypes whose ids are fixed
   * keys declared in code, such as Role; ignored when `titleField` is set.
   */
  translateId?: boolean;
  /**
   * The Attach Image (or Attach) field that pictures a document — a person's
   * photo, a company's logo. The Desk's Cards view shows it on each card, with
   * the title's initials when it is empty. It may be restricted by `permlevel`:
   * a reader who cannot see it gets the initials. Like `titleField`, it names
   * the field by string, so a renamed field has to be changed here too.
   */
  imageField?: string;
  sortField?: string;
  sortOrder?: "asc" | "desc";
  searchFields?: string[];
  /**
   * The fields a Link dropdown shows under each option's title, in order,
   * joined with " · " — e.g. `["cpf"]` to show the CPF and not the id. Left out,
   * it shows the id and the `searchFields`. `"id"` may be listed. Shown, not
   * searched: list a field in `searchFields` too for typing it to find the row.
   */
  linkSubtitle?: string[];
  /**
   * The order a Link dropdown (and a Table MultiSelect's picker) lists the
   * options in: `"field [asc|desc], ..."`, e.g. `"unit_name asc"`. Left out,
   * it is the `titleField` A to Z, else `sortField`. Text compares ignoring
   * case and accents. Typed text still puts the exact and prefix matches of
   * the id or title first. The list view keeps `sortField`/`sortOrder`.
   */
  linkOrderBy?: string;
  /**
   * Whether the Desk's global search looks into this DocType. By default it
   * does when the DocType declares `titleField` or `searchFields` (never for
   * a child table or a Single); `true` includes it anyway (matching `id`),
   * `false` leaves it out.
   */
  globalSearch?: boolean;
  /**
   * `false` when only server code creates this DocType's documents — a
   * controller method, a job, a service — and nobody types one in. The desk
   * then offers no way to make one, to Admin either: no New in the list, no
   * `+` on a Link, no Duplicate or Amend, no insert from a spreadsheet, and
   * `/new` shows a notice. `POST /api/resource/<doctype>` is refused too.
   * Server code inserts as before. Pair it with a `description` that says
   * where the documents come from.
   */
  allowCreate?: boolean;
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
  /** A line shown under the list's title, e.g. where the documents come from. A catalogue key, like `label`. */
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
  id: string;
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

export type FilterOp =
  | "=" | "!=" | ">" | ">=" | "<" | "<=" | "like" | "not like" | "in" | "not in"
  | "between" | "is" | "set" | "not set"
  /**
   * Tree operators (DAT-07). They address the `id` of a tree DocType or a Link
   * pointing at one, and the value is one id or a list of them:
   * `["id", "descendants of", "Brazil"]`, `["territory", "descendants of (inclusive)", "Brazil"]`.
   */
  | "descendants of" | "descendants of (inclusive)" | "not descendants of"
  | "ancestors of" | "not ancestors of";
export type FilterTuple = [string, FilterOp, any] | [string, any] | [string, string, FilterOp, any];
/**
 * ORs groups of filters inside a filter list: a row matches when every filter
 * of at least one group does. `{ any: [[["status", "=", "Open"]], [["priority", "=", "High"]]] }`.
 */
export type FilterAny = { any: Filters[] };
export type Filters = (FilterTuple | FilterAny)[] | Record<string, any>;

export interface ListArgs {
  filters?: Filters;
  orFilters?: Filters;
  fields?: string[];
  orderBy?: string;
  limit?: number;
  start?: number;
  /** "count(id) as total" style aggregates are allowed in fields */
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
  summary?: { label: string; value: any; datatype?: "Currency" | "Int" | "Float" | "Data" | "Date" | "Datetime"; indicator?: string }[];
}

export interface ChartData {
  type: "bar" | "line" | "pie" | "donut";
  labels: string[];
  datasets: { name: string; values: number[] }[];
}

export interface ReportDef {
  name: string;
  label?: string;
  /** A line shown under the report's title: what it shows and how to read it. A catalogue key, like `label`. */
  description?: string;
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
  /**
   * alternative: method returning { value, formatted?, color? }. It runs as the
   * calling user with no DocType check: the workspace `roles` are its only gate.
   */
  method?: () => { value: number; formatted?: string; color?: string };
  /** The DocType a `method` reads: the card answers 403 to a user without read on it. */
  refDoctype?: string;
  color?: string;
  route?: string;
  /** on a site with tenancy: show only in this space (default: both) */
  space?: WorkspaceSpace;
}

export interface ChartDef {
  name: string;
  label: string;
  type: "bar" | "line" | "pie" | "donut";
  /** Runs as the calling user with no DocType check: the workspace `roles` are its only gate. */
  method: () => ChartData;
  /** The DocType `method` reads: the chart answers 403 to a user without read on it. */
  refDoctype?: string;
}

/**
 * On a site with tenancy, the space a workspace or one of its items belongs
 * to: "platform" (the operator's, outside any tenant) or "tenant" (inside
 * one). Without it, it shows in both; without tenancy it is ignored.
 */
export type WorkspaceSpace = "platform" | "tenant";

export interface WorkspaceDef {
  name: string;
  label?: string;
  icon?: string;
  /** shown in the sidebar for these roles ("*" = all) */
  roles?: string[];
  /** on a site with tenancy: show the whole workspace only in this space (default: both) */
  space?: WorkspaceSpace;
  shortcuts?: { label: string; doctype?: string; report?: string; route?: string; icon?: string; filters?: Filters; space?: WorkspaceSpace }[];
  numberCards?: NumberCardDef[];
  charts?: ChartDef[];
  reports?: string[];
  /** grouped links for the workspace page */
  links?: { label: string; items: { label: string; doctype?: string; report?: string }[] }[];
  sidebar?: { label: string; doctype?: string; report?: string; route?: string; icon?: string; space?: WorkspaceSpace }[];
}

export interface Context {
  user: string;
  roles: string[];
  /** the language of this request */
  lang: string;
  /** every language the site serves — one per translations/<lang>.csv, plus "en" */
  langs: string[];
  /** in a job / migrate / test — no HTTP request */
  request?: {
    method: string;
    path: string;
    ip?: string;
    /** the body exactly as received (UTF-8), which is what a signature covers */
    rawBody?: string;
    /**
     * Request headers, names lower-cased. `cookie` and `authorization` are
     * left out: the credential that authenticated the call is not app data.
     */
    headers?: Record<string, string>;
    /**
     * What followed the method's path in the URL, percent-decoded, without the
     * leading "/" (`""` when there was nothing). Set only on a method
     * whitelisted with `pathTail: true`.
     */
    pathTail?: string;
  };
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

/**
 * `onLoad` runs when the form loads a document (and after a save or a
 * method), to fill its `computed` fields; only those survive it, nothing is
 * written.
 */
export type DocEvent =
  | "beforeValidate" | "validate" | "beforeSave" | "afterInsert" | "onUpdate"
  | "beforeSubmit" | "onSubmit" | "beforeCancel" | "onCancel" | "onUpdateAfterSubmit"
  | "onTrash" | "afterDelete" | "beforeRename" | "afterRename" | "beforeInsert" | "onLoad";

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
  /** callable from the desk/API via POST /api/resource/:doctype/:id/:method */
  methods?: Record<string, (doc: T & Document<T>, args: Record<string, any>, ctx: Context) => any>;
  hasPermission?: (doc: T, ptype: string, user: string) => boolean | undefined;
  permissionQuery?: (user: string) => Filters | undefined;
}

/**
 * One scheduled method: its dotted path, or the path with the user it runs
 * as. A bare path runs as `Admin` with permissions ignored; with `runAs` it
 * runs under that user's roles and access scopes.
 */
export type ScheduledEntry = string | { method: string; runAs?: string };

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
    cron?: Record<string, ScheduledEntry[]>;
    all?: ScheduledEntry[]; hourly?: ScheduledEntry[]; daily?: ScheduledEntry[]; weekly?: ScheduledEntry[]; monthly?: ScheduledEntry[];
  };
  afterInstall?: (ctx: Context) => void;
  afterMigrate?: (ctx: Context) => void;
  /**
   * On a site with tenancy: `"tenant"` makes `space: "tenant"` the default of
   * every DocType of this app, which the platform space then neither lists,
   * reads nor writes. A DocType opts out with `space: "any"`; a shared one is
   * never covered. The app's fixtures cannot include such a DocType: give each
   * tenant its records in `onTenantCreate`. See `tenancy`.
   */
  space?: "tenant";
  /**
   * Runs inside each tenant when it is created (tenancy). Fixtures and
   * `afterInstall` fill the platform space only, so this is where the app
   * gives a new tenant the records it cannot start without.
   */
  onTenantCreate?: (ctx: Context) => void;
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
  portal?: {
    /**
     * Client scripts (relative to app dir) loaded on every portal page, for a
     * Website User and for a desk user inside "My portal". Form scripts
     * (`*.form.ts`) do not run in the portal; these do.
     */
    include?: string[];
  };
  /**
   * Static sites the app serves to anyone, by URL prefix: `{ "/r": "checkout/build" }`
   * serves the files of `<app>/checkout/build` under `/r/`. Meant for the build of
   * a SPA that calls the app's guest methods. The prefix is one lowercase segment
   * and not one ddcore uses (`/api`, `/app`, `/login`, `/portal`, `/_app`, …).
   *
   * - `dir`: relative to the app, inside it. It may not exist yet; until it does
   *   the prefix answers 404.
   * - `fallback`: the file served for a path that names no file (a client-side
   *   route); default `"index.html"`, `null` for a plain 404.
   * - `frame`: allow other pages to embed the site; default `false`
   *   (`X-Frame-Options: DENY`).
   *
   * See `www`.
   */
  www?: Record<string, string | { dir: string; fallback?: string | null; frame?: boolean }>;
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
  /** reads the stored document again; `ignorePermissions` as in `ddcore.getDoc` */
  reload(opts?: { ignorePermissions?: boolean }): this;
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
  /**
   * Context for one write: what is set before insert/save/submit/cancel/delete
   * is what the hooks of that write see, and what they set comes back. JSON
   * values only; never stored, never sent to a client.
   */
  flags: Record<string, any>;
}

/** Persistent notifications. Recipients are active User ids, never external addresses. */
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

/** What `ddcore.users.invite` and `resendInvite` return. */
export interface InviteResult {
  user: string;
  expires: string;
  /** Only when the site does not really deliver mail. */
  link?: string;
}

/** A key issued by `ddcore.users.createApiKey`; it signs in as `Authorization: token key:secret`. */
export interface CreatedApiKey {
  key: string;
  /** Shown this once: only its hash is stored. */
  secret: string;
  /** Null when the key never expires. */
  expires: string | null;
}

/**
 * A self-service portal (OPS-10): the only thing a Website User can reach.
 * See `docs/agent/portal.md`.
 */
export interface PortalDef {
  /** Unique across apps; its URL name is the name in lower case with dashes. */
  name: string;
  title?: string;
  /** The roles that reach this portal. */
  roles: string[];
  /**
   * The record that stands for the signed-in user: the rows of `doctype`
   * whose `userField` (a Link to User) is them. Over User itself, use
   * `{ doctype: "User", userField: "id" }`.
   */
  identity: { doctype: string; userField: string; filters?: Record<string, any> };
  pages: PortalPageDef[];
}

export interface PortalPageDef {
  /** Lowercase letters, digits and dashes; the page's URL segment. */
  name: string;
  label: string;
  description?: string;
  doctype: string;
  /** `list` (default) shows the user's rows; `record` shows their one row. */
  kind?: "list" | "record";
  /**
   * What makes a row the user's own: page field → identity field. Every pair
   * must hold. `{ employee: "id" }` reads "rows whose employee is my identity's id".
   */
  match: Record<string, string>;
  /** The fields shown, level 0 only; no Table, Password or Vault. */
  fields: string[];
  /** The list's columns; the first four fields when omitted. */
  listFields?: string[];
  /** The fields the user may type into; a subset of `fields`, never a match field. */
  editable?: string[];
  create?: boolean;
  write?: boolean;
  /** A `whitelisted(fn, { portal: true })` method whose result prefills a new document. */
  defaultsMethod?: string;
  /** Buttons leading to other pages of the same portal; `new` opens its creation form. */
  actions?: { label: string; page: string; new?: boolean }[];
}
