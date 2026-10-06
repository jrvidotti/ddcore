// @ddcore/sdk — the API apps use on the server (runs inside the ddcore binary).
import type {
  AppDef, BaseDoc, ControllerDef, Context, DoctypeDef, Document, ExtensionDef, Filters, ListArgs,
  MailTemplateDef, NotificationDef, PatchDef, PortalDef, PrintTemplateDef, ReportDef, SendMailArgs, WorkflowDef, WorkspaceDef,
  CreatedApiKey, DocShare, DocShares, InviteResult, ShareRights,
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
   * Inside a tenant the key is scoped to it: two tenants locking the same key
   * do not wait for each other. A site-wide lock is taken from the platform.
   */
  lock(key: string): void;
  /**
   * Runs `fn` inside a savepoint. When it throws, only its writes (and its
   * messages and events) are rolled back, the error is rethrown, and the
   * transaction stays usable: catch a `DuplicateEntryError` from a unique
   * index and carry on. Returns what `fn` returns. Savepoints nest.
   */
  savepoint<T>(fn: () => T): T;
  getSingleValue(doctype: string, field: string): any;
}

/** One part of a `bodyEncoding: "multipart"` body: a text field, or a file given as base64. */
export type HttpMultipartPart =
  | { name: string; value: string }
  | { name: string; base64: string; filename?: string; contentType?: string };

export interface HttpOpts {
  headers?: Record<string, string>;
  /**
   * Timeout in seconds; defaults to 15. In a job, the job's own `timeout`, a cancellation and
   * the worker shutting down cut the call sooner.
   */
  timeout?: number;
  /** `"base64"` returns the body base64-encoded, for binary content; defaults to `"text"`. */
  responseType?: "text" | "base64";
  /**
   * How `body` is turned into bytes. `"base64"`: `body` is a base64 string, sent as the raw
   * bytes (`Content-Type` defaults to `application/octet-stream`). `"multipart"`: `body` is an
   * array of `HttpMultipartPart`, sent as multipart/form-data with the boundary in
   * `Content-Type`, which replaces any the call passes. A file part's `filename` defaults to
   * its `name` and its `contentType` to `application/octet-stream`. Without it, a string body
   * is sent as is and anything else as JSON.
   */
  bodyEncoding?: "base64" | "multipart";
  /** The largest response body accepted, in bytes; defaults to 10 MiB. A larger one throws. */
  maxBytes?: number;
  /**
   * How many redirects to follow; defaults to 10. `0` returns the 3xx itself, with its
   * `Location` in `headers`. A hop to another host, or from https to http, drops every
   * header in `headers` except `Content-Type`.
   */
  maxRedirects?: number;
  /**
   * A client certificate for mutual TLS: a PKCS#12 (`.pfx`) file, base64-encoded,
   * with its password, or a PEM certificate and key. Calls with the same
   * certificate share connections.
   */
  clientCert?: { pfx: string; password?: string } | { cert: string; key: string };
}

export interface HttpResponse {
  status: number;
  /** The body as text, or base64 with `responseType: "base64"`. */
  body: string;
  /** Response headers with canonical HTTP names; repeated values are comma-separated. */
  headers: Record<string, string>;
  json(): any;
}

/** What `ddcore.crypto.pfxInfo` and `certInfo` say of a certificate: never its key. */
export interface CertInfo {
  /** Start of validity, an RFC 3339 instant in UTC (`2026-01-31T12:00:00Z`). */
  notBefore: string;
  /** End of validity, an RFC 3339 instant in UTC. */
  notAfter: string;
  /** Who it was issued to, as a distinguished name (`CN=…,O=…`). */
  subject: string;
  /** Who issued it, as a distinguished name. */
  issuer: string;
  /** Serial number, as lower-case hex. */
  serial: string;
  /** How many certificates came along with it: its chain, 0 when it is alone. */
  chain: number;
}

/**
 * A browser's push subscription, as `PushSubscription.toJSON()` gives it:
 * keys in base64url.
 */
export interface PushSubscription {
  endpoint: string;
  keys: { p256dh: string; auth: string };
}

export interface PushOpts {
  /** How long, in seconds, the push service keeps the message for an offline browser; defaults to 86400 (a day). */
  ttl?: number;
  /** Sent as the `Urgency` header; left out, the push service treats the message as `normal`. */
  urgency?: "very-low" | "low" | "normal" | "high";
  /** A newer message with the same topic replaces one still waiting; up to 32 base64url characters. */
  topic?: string;
}

/** The push service's answer. Any status is returned, not thrown. */
export interface PushResult {
  /** 201 is accepted; 404 or 410 means the subscription is gone, and the app should delete it. */
  status: number;
  body: string;
  headers: Record<string, string>;
}

/**
 * `ddcore.files.save`'s argument. Exactly one of `content`, `contentBase64`
 * and `fromUrl` is the source of the bytes.
 */
export interface SaveFileOpts {
  /** The document to attach the file to; attaching needs write on it. */
  doctype?: string;
  id?: string;
  /** The Attach field the file is for, when it is one. */
  fieldname?: string;
  /** The name the file is known by; defaults to the last segment of `fromUrl`. */
  filename?: string;
  /** Defaults to true. A restricted field, or a Website User, forces it. */
  isPrivate?: boolean;
  /** Defaults to the download's Content-Type, then to the filename's extension. */
  contentType?: string;
  /** Text, stored as UTF-8. */
  content?: string;
  /** Bytes, base64-encoded: `ddcore.http.get(url, { responseType: "base64" }).body`. */
  contentBase64?: string;
  /** Downloaded by the server with GET. A non-2xx status throws. */
  fromUrl?: string;
  /** Request headers for `fromUrl`, such as `Authorization`. */
  headers?: Record<string, string>;
  /** The largest file accepted, in bytes; defaults to 50 MiB. */
  maxBytes?: number;
  /** Timeout of the `fromUrl` download in seconds; defaults to 15. */
  timeout?: number;
  /** Skip the write check on the document, for server code that must attach anyway. */
  ignorePermissions?: boolean;
}

/** A File document, as `ddcore.files.save` returns it. */
export interface FileDoc extends BaseDoc {
  file_name: string;
  file_url: string;
  file_size: number;
  content_type: string;
  is_private: 0 | 1 | boolean;
  attached_to_doctype?: string;
  attached_to_id?: string;
  attached_to_field?: string;
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

/** The attempt a job's `onStart` callback is told about, and what `ddcore.job.current()` returns. */
export interface JobInfo {
  id: number;
  method: string;
  queue: string;
  /**
   * 1 on the first attempt; 0 for a job cancelled before any attempt. A run interrupted by a worker
   * shutting down or by maintenance gives its attempt back, so the run after it has the same one.
   */
  attempt: number;
  maxAttempts: number;
  /**
   * How many times a worker began this job, this run included: 1 on the first run; more than 1
   * means an earlier run began and was given back, failed or timed out — it may already have made
   * its calls to other systems before it rolled back. Never goes down.
   */
  starts: number;
  /** The user the job acts as, when it was queued with `runAs`. */
  runAs?: string;
}

/** What a job's `onFailure` callback is told about the attempt that ended. */
export interface JobFailure extends JobInfo {
  error: string;
  reason: "error" | "timeout" | "cancelled";
  /** False while another attempt is still coming. */
  final: boolean;
}

export interface DDCoreAPI {
  db: DDCoreDB;
  session: Context;
  /**
   * For Single DocTypes, omit the id to load settings (defaults before the first save).
   * `ignorePermissions` skips the role permissions, as `insert`/`save` do; the user's
   * access scopes and the tenancy wall still apply. The document comes back whole, with
   * no field-level redaction: `ddcore.redact` it before handing it to a client.
   */
  getDoc<T extends BaseDoc = BaseDoc>(doctype: string, id?: string | Filters, opts?: { ignorePermissions?: boolean }): T & Document<T>;
  newDoc<T extends BaseDoc = BaseDoc>(doctype: string, values?: Partial<T>): T & Document<T>;
  deleteDoc(doctype: string, id: string, opts?: { ignorePermissions?: boolean; force?: boolean }): void;
  getMeta(doctype: string): DoctypeDef;
  /**
   * `doc` carries only what the permission rules read — the doctype is already
   * the first argument, so an `{ id, owner }` pair is a complete call. The host
   * takes it as a plain map and never requires a whole document. A string is
   * a document id, and the stored document is checked.
   *
   * `user` checks another user's permission — their roles, scopes and shares
   * — instead of the current one's.
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
  /** Files from server code; see `storage`. */
  files: {
    /**
     * Stores bytes as a File on the current transaction, with the rules of an
     * upload. If the transaction rolls back, the bytes are deleted too.
     */
    save(opts: SaveFileOpts): FileDoc;
    /**
     * A URL that serves the file to anyone holding it for `ttl` seconds
     * (default: DDCORE_S3_PRESIGN_TTL, at most 7 days), for a third party with
     * no session. Needs read on the File, and the s3 storage backend.
     */
    presign(fileUrl: string, opts?: { ttl?: number; ignorePermissions?: boolean }): string;
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
   *
   * `onStart` and `onFailure` are method paths, like `method`, called as
   * `fn(args, job)`. Each runs in a transaction of its own, committed apart from
   * the job's, which is what lets a document say its job is running, or that it
   * failed. `onStart` runs before the body on every attempt; if it throws, the
   * attempt fails and the body does not run. `onFailure` runs after the body
   * rolled back — on an error, a timeout, a cancellation, or a worker that died
   * — with `job.final` false while a retry is still coming. If it throws, the
   * error goes to the Error Log and the job stays failed.
   *
   * `uniqueKey` keeps a job from stacking: while a job with the same key is
   * still `queued`, the call queues nothing and returns that job's id. A job
   * that has been claimed no longer holds the key, so work arriving while it
   * runs queues a new one. The key is global; prefix it with your app's name.
   *
   * `runAs` is the user the job acts as. Without it a job runs as whoever
   * queued it with permissions ignored; with it the body and both callbacks run
   * under that user's roles and access scopes, as `ddcore.runAs` would. The user
   * must exist and be enabled, when the job is queued and when it runs.
   */
  enqueue(method: string, args?: Record<string, any>, opts?: { queue?: string; runAfter?: string; timeout?: number; maxAttempts?: number; backoff?: "fixed" | "exponential"; onStart?: string; onFailure?: string; uniqueKey?: string; runAs?: string }): number;
  job: {
    /**
     * The job this code runs in — its body or one of its callbacks — as the `JobInfo` `onStart` is
     * given, or `null` outside a job. `starts > 1` is how a body doing non-idempotent work learns
     * that an earlier run may already have done it.
     */
    current(): JobInfo | null;
  };
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
     * Verifies a Standard Webhooks request received on an inbound method: `headers` and
     * `rawBody` are those of `ctx.request`. Returns `false`, never throws, when a header is
     * missing, the timestamp is further than `toleranceSeconds` (default 300; `Infinity`
     * switches the window off) from now, or no `v1,` signature matches. A missing secret (`ddcore.secret` returns `null` for an
     * unset one) or an empty one verifies nothing: the result is `false`. A `whsec_<base64>` secret is decoded as the spec says;
     * several space-separated signatures are accepted; the comparison is constant-time.
     */
    verify(secret: string | null | undefined, headers?: Record<string, string>, rawBody?: string, opts?: { toleranceSeconds?: number }): boolean;
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
  /** The site's public address (`DDCORE_URL`), without a trailing slash. */
  siteUrl(): string;
  /**
   * The absolute desk address of a document, for a link in a message that is
   * read outside the desk — a mail template's `b.button`, a webhook payload.
   * The DocType goes in without its spaces: `…/app/SalesOrder/SO-1`.
   */
  docUrl(doctype: string, id: string): string;
  /**
   * Sends an event to the desk over SSE when the transaction commits. With
   * `user`, only that user's sessions receive it; with `doctype` (and `id`),
   * only sessions that may read that DocType (that document). A desk script
   * listens with `frm.onRealtime` or `ddcore.realtime.on`. The name is
   * letters, digits and `_ . : -`; prefix it with the app's name.
   */
  publish(event: string, payload: any, opts?: { user?: string; doctype?: string; id?: string }): void;
  /**
   * One structured record in the server log. A plain object's keys become fields of the record;
   * the other arguments are joined into the message (an Error as "Error: message", anything else
   * as JSON).
   */
  log: { info(...a: any[]): void; warn(...a: any[]): void; error(...a: any[]): void; debug(...a: any[]): void };
  /**
   * The Error Log, written by the app on purpose.
   */
  errorLog: {
    /**
     * Files an Error Log row about `error` on a transaction of its own and
     * returns the row's id, so a caught failure reaches the Error Log and
     * `ddcore doctor` while the work goes on: the row stays whether the
     * caller's transaction commits or rolls back. `error` may be an `Error`,
     * a `DDCoreError` or a string. The row carries the current request's id,
     * or `job:<id>` in a job, and lands in the current tenant. `method` names
     * the row's source (default `job:<method>` in a job, `app.record`
     * elsewhere); `context` is appended to the text as JSON. Never throws:
     * returns `""` when the row could not be written, which the process log
     * still records.
     */
    record(error: unknown, opts?: { method?: string; context?: Record<string, any> }): string;
  };
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
   * Accounts handed out by app code (OPS-10). A System Manager may invite any
   * user; anyone else only a `Website User`, never with a privileged role —
   * a Website User reaches the portals and nothing else. Who may call the
   * app's own method is the app's whitelist to decide. `link` comes back when
   * the site does not really deliver mail. See `docs/agent/portal.md`.
   */
  users: {
    invite(args: { email: string; fullName: string; roles?: string[]; userType?: "System User" | "Website User" }): InviteResult;
    resendInvite(user: string): InviteResult;
    /**
     * Issues an API key for `user`, as `ddcore apikey` does, and records an
     * `apikey.create` audit event without the secret. System Manager (outside
     * portal mode) or Admin only. Inside a tenant, only that tenant's users;
     * from the platform space, the key is made in the user's own tenant.
     * `days` defaults to the site's `apiKeyDays`.
     */
    createApiKey(user: string, opts?: { label?: string; days?: number }): CreatedApiKey;
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
    /**
     * `n` characters of `a-z0-9` from a cryptographically secure source (default 10). A
     * fraction rounds up and anything not positive gives `""`; throws only above 65536.
     */
    randomString(n?: number): string;
  };
  /** current authenticated user (Guest when anonymous) */
  user(): string;
  getRoles(user?: string): string[];
  /** true inside `ddcore test` */
  isTest(): boolean;
  /** true when running in a job/migrate rather than a request */
  isJob(): boolean;
  /**
   * Runs `fn` as `user` and returns what it returns. Inside it every read and
   * write honours that user's roles and access scopes — including in a job, a
   * scheduled method or a guest webhook, where nothing else applies them — and
   * `owner`, `modified_by` and audit events record that user. It shares the
   * caller's transaction, restores the caller when `fn` returns or throws, and
   * nests. `ddcore.session` follows the switch; a `ctx` argument received
   * earlier does not. Throws when the user does not exist or is disabled.
   */
  runAs<T>(user: string, fn: () => T): T;
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
   * Hashing (`sha256`), HMAC (`hmacSha256`, to check a webhook signature over the
   * exact bytes the provider sent, `ctx.request.rawBody`), constant-time comparison,
   * secure random values (`randomToken`, `randomInt`) and certificate inspection.
   */
  crypto: {
    /**
     * HMAC-SHA256 of `data` under `key`. Without options: `key` is a UTF-8 string and the
     * answer is lower-case hex. `keyEncoding` says the key is `"base64"` or `"hex"` text
     * (a `whsec_` secret is base64 once its prefix is cut), `output` picks `"hex"`
     * (default), `"base64"` or `"base64url"` (no padding).
     */
    hmacSha256(key: string, data: string, opts?: {
      keyEncoding?: "utf8" | "base64" | "hex";
      output?: "hex" | "base64" | "base64url";
    }): string;
    /** SHA-256 of the UTF-8 `data`: lower-case hex unless `output` says otherwise. Store this, not a token. */
    sha256(data: string, opts?: { output?: "hex" | "base64" | "base64url" }): string;
    /** `bytes` (default 32, 1 to 1024) from crypto/rand as base64url without padding. */
    randomToken(bytes?: number): string;
    /** A uniform integer in `[min, max)` from crypto/rand. Both must be safe integers and `max > min`. */
    randomInt(min: number, max: number): number;
    /** Constant-time string comparison: use it, never `===`, on a signature. */
    timingSafeEqual(a: string, b: string): boolean;
    /**
     * Describes the leaf certificate of a PKCS#12 (`.pfx`) file, base64-encoded:
     * the same input as `clientCert.pfx` of `ddcore.http`. A wrong password or
     * an unreadable file throws a `ValidationError` that does not repeat the
     * material, so it validates a certificate before storing it.
     */
    pfxInfo(pfx: string, password?: string): CertInfo;
    /** The same for a PEM certificate; the certificates after the first are its chain. */
    certInfo(pem: string): CertInfo;
  };
  /**
   * Web Push with the site's VAPID key, read from `DDCORE_SECRET_VAPID_PUBLIC_KEY`,
   * `_PRIVATE_KEY` and `_SUBJECT` (`ddcore push keys` generates a pair). See `push`.
   */
  push: {
    /**
     * Encrypts `payload` for the subscription (RFC 8291) and posts it to its
     * endpoint signed with VAPID (RFC 8292). A string goes as is, any other
     * value as JSON; at most 3993 bytes. Only a failed request, a missing key or
     * a bad argument throws a `ValidationError`.
     */
    send(subscription: PushSubscription, payload: any, opts?: PushOpts): PushResult;
    /** The VAPID public key a page subscribes with, or null when the site has none. */
    publicKey(): string | null;
  };
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
   * Tenancy: several customers in one database, each confined to its own rows
   * (`"tenancy": true` in ddcore.json). Code runs in one space — a tenant, or
   * the platform space where Admin, scheduled methods and migrations work.
   * See `tenancy`.
   */
  tenant: {
    /** The tenant this code is working in; `""` in the platform space, and always without tenancy. */
    current(): string;
    /** Every tenant. Platform space only. */
    list(): { id: string; title: string; enabled: boolean }[];
    /**
     * Runs `fn` inside a tenant, on the current transaction: what it reads and
     * writes there are that tenant's rows. Only the platform space can enter a
     * tenant — a scheduled method fanning out, a migration patch backfilling.
     */
    run<T>(id: string, fn: () => T): T;
  };
  /**
   * An encrypted credential vault, stored in the database but encrypted at
   * rest with `DDCORE_SECRET_KEY` and audited on every read, write and delete.
   *
   * Use this for machine credentials that belong to a row (e.g. one API token
   * per customer/tenant) where the set is dynamic and cannot be known at boot.
   * Values never appear in HTTP responses, MCP, Version diffs or exports.
   *
   * With tenancy, each space has its own secrets. `{ shared: true }` names the
   * platform space's secret instead — where the `Vault` fields of a `shared`
   * DocType are kept: every space reads it, only the platform space sets or
   * deletes it.
   */
  vault: {
    set(name: string, value: string, opts?: { shared?: boolean }): void;
    get(name: string, opts?: { shared?: boolean }): string | null;
    del(name: string, opts?: { shared?: boolean }): void;
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

export interface WhitelistOpts {
  allowGuest?: boolean;
  /** Accepted HTTP verbs; any other answers 405. Omitted, any verb is accepted. */
  methods?: ("GET" | "POST")[];
  /**
   * Answer with the returned string as the whole body, in this content type
   * and without the `{data, messages}` envelope. For a provider that checks a
   * callback URL by reading back a challenge (Meta's `hub.challenge`). The
   * method must return a string; a thrown error is still a JSON error.
   */
  raw?: { contentType: string };
  roles?: string[];
  /**
   * Callable by Website Users (OPS-10). Everything else under /api is closed
   * to them. The method still runs as the caller, in portal mode: its
   * `ddcore.getList`/`getDoc` see only what the portals grant.
   */
  portal?: boolean;
  /**
   * Also answer below the method's own path: `/api/method/<path>/pix/1`
   * reaches this method with `ctx.request.pathTail` = `"pix/1"`
   * (percent-decoded; `""` when the call names the method alone). For a
   * provider that appends to the URL it was registered with. Without it, a
   * sub-path is a 404, as for a method that does not exist.
   */
  pathTail?: boolean;
  /**
   * Callable by pages on the origins listed in `cors.origins` (ddcore.json, or
   * DDCORE_CORS_ORIGINS): the preflight is answered and the response is readable
   * by the calling page. Never with credentials — the caller sends an API key or
   * comes as Guest. Without it a browser keeps the response from any other
   * origin. See `controller-api` → "Calls from another origin".
   */
  cors?: boolean;
}

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

/**
 * Declares a self-service portal: `export default definePortal({…})` in
 * `portal/<name>.portal.ts`. See `docs/agent/portal.md`.
 */
export function definePortal(def: PortalDef): PortalDef {
  __ddcore.register("portal", def);
  return def;
}
