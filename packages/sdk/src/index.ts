// @ddcore/sdk — the API apps use on the server (runs inside the ddcore binary).
import type {
  AppDef, BaseDoc, ControllerDef, Context, DoctypeDef, Document, Filters, ListArgs,
  ReportDef, WorkspaceDef,
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
   * Advisory lock por chave, válido até o fim da transação: outra requisição
   * que peça a mesma chave espera. Use para tornar uma operação idempotente sob
   * concorrência, ex.: `ddcore.db.lock("faturamento:" + contrato)`.
   */
  lock(key: string): void;
  getSingleValue(doctype: string, field: string): any;
}

export interface DDCoreAPI {
  db: DDCoreDB;
  session: Context;
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
    roundTo(v: number, precision?: number): number;
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

export interface WhitelistOpts { allowGuest?: boolean; methods?: ("GET" | "POST")[]; roles?: string[] }

/** Marks a function as callable via POST /api/method/<app>.<path>.<name>. */
export function whitelisted<F extends (...a: any[]) => any>(fn: F, opts: WhitelistOpts = {}): F {
  (fn as any).__whitelisted = opts;
  return fn;
}

export const _ = (text: string, args?: any[]) => ddcore._(text, args);

export type { Document, Context };
