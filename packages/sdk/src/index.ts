// @ddcore/sdk — the API apps use on the server (runs inside the ddcore binary).
import type {
  AppDef, BaseDoc, ControllerDef, Context, DoctypeDef, Document, ExtensionDef, Filters, ListArgs,
  MailTemplateDef, PatchDef, ReportDef, SendMailArgs, WorkspaceDef,
} from "./types";
export * from "./types";

declare const __ddcore: { register(kind: string, value: any): void; current: string };

// The host injects `ddcore` (bridge to Go) before any module runs.
export interface DDCoreDB {
  getValue<T = any>(doctype: string, name: string | Filters, field: string): T;
  getValue<T = Record<string, any>>(doctype: string, name: string | Filters, fields: string[]): T | null;
  getList<T = Record<string, any>>(doctype: string, args?: ListArgs): T[];
  getAll<T = Record<string, any>>(doctype: string, args?: ListArgs): T[];
  setValue(doctype: string, name: string, field: string | Record<string, any>, value?: any): void;
  count(doctype: string, filters?: Filters, orFilters?: Filters): number;
  exists(doctype: string, nameOrFilters: string | Filters): string | null;
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

export interface DDCoreAPI {
  db: DDCoreDB;
  session: Context;
  /** For Single DocTypes, omit the name to load settings (defaults before the first save). */
  getDoc<T extends BaseDoc = BaseDoc>(doctype: string, name?: string | Filters): T & Document<T>;
  newDoc<T extends BaseDoc = BaseDoc>(doctype: string, values?: Partial<T>): T & Document<T>;
  deleteDoc(doctype: string, name: string, opts?: { ignorePermissions?: boolean; force?: boolean }): void;
  getMeta(doctype: string): DoctypeDef;
  hasPermission(doctype: string, ptype?: string, doc?: BaseDoc | string, user?: string): boolean;
  throw(message: string, opts?: { title?: string; type?: string }): never;
  msgprint(message: string, opts?: { title?: string; indicator?: string; alert?: boolean }): void;
  _(text: string, args?: any[]): string;
  bold(v: any): string;
  cache: { get(key: string): any; set(key: string, value: any, ttlSeconds?: number): void; del(key: string): void };
  http: {
    get(url: string, opts?: { headers?: Record<string, string>; timeout?: number }): { status: number; body: string; json(): any };
    post(url: string, body: any, opts?: { headers?: Record<string, string>; timeout?: number }): { status: number; body: string; json(): any };
  };
  enqueue(method: string, args?: Record<string, any>, opts?: { queue?: string; runAfter?: string; timeout?: number }): number;
  /**
   * Queues one message from a registered template and returns the name of its
   * `Email Delivery` record.
   *
   * Synchronous, like everything else here, but delivery is not: the message is
   * written onto *this* transaction and handed to a worker. A request that
   * rolls back sends nothing, which is the whole reason it works this way.
   */
  sendMail(args: SendMailArgs): { delivery: string };
  /** The site's title, as the desk and the framework's own mail display it. */
  siteName(): string;
  publish(event: string, payload: any, opts?: { user?: string; doctype?: string; name?: string }): void;
  log: { info(...a: any[]): void; warn(...a: any[]): void; error(...a: any[]): void; debug(...a: any[]): void };
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
  form: { addComment?(doctype: string, name: string, text: string): void };
  callMethod(method: string, args?: Record<string, any>): any;
  rename(doctype: string, oldName: string, newName: string): string;
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
