// @ddcore/sdk — the API apps use on the server (runs inside the ddcore binary).
import type {
  AppDef, BaseDoc, ControllerDef, Context, DoctypeDef, Document, ExtensionDef, Filters, ListArgs,
  MailTemplateDef, NotificationDef, PatchDef, PrintTemplateDef, ReportDef, SendMailArgs, WorkflowDef, WorkspaceDef,
  DocShare, DocShares, ShareRights,
} from "./types";
export * from "./types";

declare const __ddcore: { register(kind: string, value: any): void; current: string };

// The host injects `ddcore` (bridge to Go) before any module runs.
export interface DDCoreDB {
  getValue<T = any>(doctype: string, id: string | Filters, field: string): T;
  getValue<T = Record<string, any>>(doctype: string, id: string | Filters, fields: string[]): T | null;
  getList<T = Record<string, any>>(doctype: string, args?: ListArgs): T[];
  getAll<T = Record<string, any>>(doctype: string, args?: ListArgs): T[];
  setValue(doctype: string, id: string, field: string | Record<string, any>, value?: any): void;
  count(doctype: string, filters?: Filters, orFilters?: Filters): number;
  exists(doctype: string, idOrFilters: string | Filters): string | null;
  /** read-only SQL with $1.. placeholders */
  sql<T = Record<string, any>>(query: string, params?: any[]): T[];
  /**
   * Advisory lock per key, valid until the end of the transaction: another
   * request asking for the same key waits. Use to make an operation idempotent
   * under concurrency, e.g.: `ddcore.db.lock("billing:" + contract)`.
   */
  lock(key: string): void;
  getSingleValue(doctype: string, field: string): any;
}

export interface HttpOpts {
  headers?: Record<string, string>;
  /** Timeout in seconds; defaults to 15. */
  timeout?: number;
}

export interface HttpResponse {
  status: number;
  body: string;
  /** Response headers with canonical HTTP names; repeated values are comma-separated. */
  headers: Record<string, string>;
  json(): any;
}

export interface ExternalDbOpts {
  /** Timeout in seconds; defaults to 30. */
  timeout?: number;
}

/** A read-only external database; see `ddcore.externalDb`. */
export interface ExternalDb {
  /** Runs a SELECT with positional `@p1`, `@p2`, … parameters. */
  sql(query: string, params?: any[], opts?: ExternalDbOpts): Record<string, any>[];
}

export interface DDCoreAPI {
  db: DDCoreDB;
  session: Context;
  /** For Single DocTypes, omit the id to load settings (defaults before the first save). */
  getDoc<T extends BaseDoc = BaseDoc>(doctype: string, id?: string | Filters): T & Document<T>;
  newDoc<T extends BaseDoc = BaseDoc>(doctype: string, values?: Partial<T>): T & Document<T>;
  deleteDoc(doctype: string, id: string, opts?: { ignorePermissions?: boolean; force?: boolean }): void;
  getMeta(doctype: string): DoctypeDef;
  /**
   * `doc` carries only what the permission rules read — the doctype is already
   * the first argument, so an `{ id, owner }` pair is a complete call. The host
   * takes it as a plain map and never requires a whole document.
   */
  hasPermission(doctype: string, ptype?: string, doc?: Partial<BaseDoc> | string, user?: string): boolean;
  /**
   * A copy of `doc` as an API read would return it to the current user:
   * Password and Vault values blanked, and every field above the user's
   * permission level removed, child rows included. Server code sees whole
   * documents; call this before a whitelisted method, a report or a published
   * event hands one to a client. See `field-permissions`.
   */
  redact<T extends Partial<BaseDoc>>(doctype: string, doc: T): Partial<T>;
  throw(message: string, opts?: { title?: string; type?: string }): never;
  msgprint(message: string, opts?: { title?: string; indicator?: string; alert?: boolean }): void;
  _(text: string, args?: any[]): string;
  bold(v: any): string;
  cache: { get(key: string): any; set(key: string, value: any, ttlSeconds?: number): void; del(key: string): void };
  http: {
    get(url: string, opts?: HttpOpts): HttpResponse;
    post(url: string, body?: any, opts?: HttpOpts): HttpResponse;
    put(url: string, body?: any, opts?: HttpOpts): HttpResponse;
    patch(url: string, body?: any, opts?: HttpOpts): HttpResponse;
    del(url: string, opts?: HttpOpts): HttpResponse;
  };
  /**
   * Queues a job. It is written on the current transaction, so the job only
   * exists if the request commits.
   *
   * `maxAttempts` is how many times a failing job is retried before it is left
   * as failed; the default is 3. Set it to 1 for work whose failure is
   * permanent, or whose effects outside the database must not be repeated.
   *
   * `backoff` is how long a failed attempt waits: `"fixed"` (the default) is
   * thirty seconds every time; `"exponential"` doubles from thirty seconds up to
   * an hour, which suits work that talks to somebody else's server.
   */
  enqueue(method: string, args?: Record<string, any>, opts?: { queue?: string; runAfter?: string; timeout?: number; maxAttempts?: number; backoff?: "fixed" | "exponential" }): number;
  /**
   * Queues one message from a registered template and returns the id of its
   * `Email Delivery` record.
   *
   * Synchronous, like everything else here, but delivery is not: the message is
   * written onto *this* transaction and handed to a worker. A request that
   * rolls back sends nothing, which is the whole reason it works this way.
   */
  sendMail(args: SendMailArgs): { delivery: string };
  webhooks: {
    /**
     * Emits an app event to every enabled `Webhook` whose custom event is
     * `event`, and returns the ids of the `Webhook Delivery` records written.
     *
     * Written on *this* transaction and sent by a worker after it commits, so a
     * request that rolls back tells no receiver anything. `data` becomes the
     * payload's `data`, frozen now. `key` makes the emit idempotent per
     * webhook: a second emit with the same key fails instead of sending twice.
     */
    emit(event: string, data?: any, opts?: { key?: string; reference?: { doctype: string; id: string } }): { deliveries: string[] };
  };
  /** The site's title, as the desk and the framework's own mail display it. */
  siteName(): string;
  publish(event: string, payload: any, opts?: { user?: string; doctype?: string; id?: string }): void;
  log: { info(...a: any[]): void; warn(...a: any[]): void; error(...a: any[]): void; debug(...a: any[]): void };
  /**
   * Document sharing (SEC-03): per-user grants on one document, checked with
   * the current user as the sharer. See `docs/agent/sharing.md`.
   */
  share: {
    /** Grants or updates `user`'s share. Read is always granted. */
    add(doctype: string, id: string, user: string, rights?: ShareRights): DocShare;
    /** Removes `user`'s share. The recipient may always drop their own. */
    remove(doctype: string, id: string, user: string): void;
    /** Who the document is shared with; without the share right, only your own share. */
    list(doctype: string, id: string): DocShares;
  };
  /**
   * Records that the current user did something sensitive to a target (PRD-06).
   * Written on the caller's transaction. Sensitive keys are redacted automatically.
   */
  audit(action: string, targetDoctype?: string, targetID?: string, detail?: Record<string, any>): void;
  /**
   * Records a refused sensitive action. Written directly to the pool so it survives rollback.
   */
  auditDenied(action: string, targetDoctype?: string, targetID?: string, detail?: Record<string, any>): void;
  utils: {
    flt(v: any, precision?: number): number;
    cint(v: any): number;
    cstr(v: any): string;
    getdate(v?: any): Date;
    nowdate(): string;
    now(): string;
    today(): string;
    formatDate(v: any, fmt?: string): string;
    addDays(d: any, n: number): string;
    addMonths(d: any, n: number): string;
    addYears(d: any, n: number): string;
    getFirstDay(d?: any): string;
    getLastDay(d?: any): string;
    dateDiff(a: any, b: any): number;
    monthDiff(a: any, b: any): number;
    formatCurrency(v: any, currency?: string): string;
    /**
     * Rounds to `precision` decimal places under the site's rounding rule,
     * over the number's shortest decimal representation — so 1.005 rounds to
     * 1.01, and -1.005 to -1.01.
     */
    roundTo(v: number, precision?: number, mode?: "commercial" | "bankers"): number;
    /** How many decimal places a Currency value has on this site (2 for USD, 0 for JPY). */
    currencyPrecision(): number;
    /** Rounds a value exactly the way the server is about to store it. */
    roundCurrency(v: any): number;
    /**
     * Splits a total into `n` parts at the site's currency precision whose sum
     * is exactly the total. Rounding each of three thirds of 100.00 gives
     * 33.33 three times; this gives [33.34, 33.33, 33.33]. The residue lands
     * on the earliest parts.
     */
    splitAmount(total: any, n: number): number[];
    randomString(n?: number): string;
  };
  /** current authenticated user (Guest when anonymous) */
  user(): string;
  getRoles(user?: string): string[];
  /** true inside `ddcore test` */
  isTest(): boolean;
  /** true when running in a job/migrate rather than a request */
  isJob(): boolean;
  form: { addComment?(doctype: string, id: string, text: string): void };
  callMethod(method: string, args?: Record<string, any>): any;
  rename(doctype: string, oldID: string, newID: string): string;
  /**
   * An integration credential, read from the environment — `.env` in
   * development, the platform in production — and never from a column.
   *
   * `ddcore.secret("stripe_key")` reads `DDCORE_SECRET_STRIPE_KEY`. Returns
   * null when this site was not given it, so a caller can decide whether the
   * integration is simply switched off. Never write the value to a document,
   * a log or a message: the point of keeping it out of the database is that it
   * stays out of every backup, export and Version diff.
   */
  secret(name: string): string | null;
  /**
   * A read-only connection to a database that is not the site's own — only
   * SQL Server for now. See `external-db`.
   *
   * `ddcore.externalDb("sql_server")` reads `DDCORE_SECRET_SQL_SERVER_HOST`,
   * `_PORT` (default 1433), `_DATABASE`, `_USER`, `_PASSWORD` and the optional
   * `_ENCRYPT`; a missing one is a ValidationError naming the variable.
   *
   * `sql` accepts only SELECT/WITH and runs in a transaction that is always
   * rolled back. Parameters are positional — `@p1`, `@p2`, … — and never
   * interpolated. `opts.timeout` is in seconds (default 30). Rows come back
   * normalized like `ddcore.db.sql`: decimals as numbers, dates as strings.
   */
  externalDb(name: string): ExternalDb;
  /**
   * An encrypted credential vault, stored in the database but encrypted at
   * rest with `DDCORE_SECRET_KEY` and audited on every read, write and delete.
   *
   * Use this for machine credentials that belong to a row (e.g. one API token
   * per customer/tenant) where the set is dynamic and cannot be known at boot.
   * Values never appear in HTTP responses, MCP, Version diffs or exports.
   */
  vault: {
    set(name: string, value: string): void;
    get(name: string): string | null;
    del(name: string): void;
    list(prefix?: string): string[];
  };
  version: string;
}

declare global {
  const ddcore: DDCoreAPI;
}

export function defineDoctype<const D extends DoctypeDef>(def: D): D {
  __ddcore.register("doctype", def);
  return def;
}

export function defineController<T extends BaseDoc = BaseDoc>(doctype: string, ctrl: ControllerDef<T>): ControllerDef<T> {
  __ddcore.register("controller", { doctype, controller: ctrl });
  return ctrl;
}

/**
 * Adds fields to, and overrides properties of, a DocType another app owns.
 *
 * Declare `requires: ["<host app>"]` in `defineApp` (core is implicit): the
 * host has to be loaded before anything can extend it. See `docs/agent/extending.md`.
 */
export function extendDoctype<T extends BaseDoc = BaseDoc>(doctype: string, ext: ExtensionDef<T>): ExtensionDef<T> {
  __ddcore.register("extension", { doctype, ext });
  return ext;
}

/**
 * Declares a message the app can send: `export default defineMailTemplate({…})`
 * in `mail/<name>.mail.ts`. See `docs/agent/mail.md`.
 */
export function defineMailTemplate<A = any>(def: MailTemplateDef<A>): MailTemplateDef<A> {
  __ddcore.register("mail", def);
  return def;
}

/**
 * Declares a document print template: `export default definePrintTemplate({…})`
 * in `print/<name>.print.ts`. See `docs/agent/print.md`.
 */
export function definePrintTemplate<T = any>(def: PrintTemplateDef<T>): PrintTemplateDef<T> {
  __ddcore.register("print", def);
  return def;
}

export function defineReport(def: ReportDef): ReportDef {
  __ddcore.register("report", def);
  return def;
}

export function defineWorkspace(def: WorkspaceDef): WorkspaceDef {
  __ddcore.register("workspace", def);
  return def;
}

export function defineApp(def: AppDef): AppDef {
  __ddcore.register("app", def);
  return def;
}

/**
 * Declares a patch: `export default definePatch({ ... })` in
 * `patches/NNNN_name.ts`.
 *
 * Unlike the other `define*` helpers it registers nothing — a patch is
 * identified by its module path, which is what `migrate` records once it has
 * run, and the registry does not know it. The helper only types the object.
 */
export function definePatch(def: PatchDef): PatchDef {
  return def;
}

export interface WhitelistOpts { allowGuest?: boolean; methods?: ("GET" | "POST")[]; roles?: string[] }

/** Marks a function as callable via POST /api/method/<app>.<path>.<name>. */
export function whitelisted<F extends (...a: any[]) => any>(fn: F, opts: WhitelistOpts = {}): F {
  (fn as any).__whitelisted = opts;
  return fn;
}

export const _ = (text: string, args?: any[]) => ddcore._(text, args);

export type { Document, Context };

/** Declares a synchronous notification rule in notifications/*.notification.ts. */
export function defineNotification<D = Record<string, any>>(def: NotificationDef<D>): NotificationDef<D> {
  __ddcore.register("notification", def);
  return def;
}

/** Declares an approval workflow in workflows/*.workflow.ts. */
export function defineWorkflow<D = Record<string, any>>(def: WorkflowDef<D>): WorkflowDef<D> {
  __ddcore.register("workflow", def);
  return def;
}

