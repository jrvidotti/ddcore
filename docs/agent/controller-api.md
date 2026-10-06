# Controllers and the server API

```ts
import { defineController, whitelisted, _ } from "@ddcore/sdk";
import type { Order } from "../../.ddcore/types";

export default defineController<Order>("Order", {
  beforeInsert(doc) {},                 // before the id is generated
  validate(doc, ctx) {                  // every write (insert and update)
    doc.total = (doc.items || []).reduce((s, i) => s + ddcore.utils.flt(i.qty) * ddcore.utils.flt(i.amount), 0);
    if (doc.total < 0) ddcore.throw(_("Negative total"), { title: _("Order") });
  },
  onUpdate(doc) {},                     // after writing (insert and update)
  onSubmit(doc) { doc.dbSet("status", "Active"); },
  onCancel(doc) {},
  onUpdateAfterSubmit(doc) {},
  onTrash(doc) {}, afterDelete(doc) {},
  beforeRename(doc) {}, afterRename(doc) {},
  onLoad(doc) { doc.balance = /* … */ 0; },  // fills `computed` fields when a form loads the doc; nothing is stored
  methods: {                            // POST /api/resource/Order/<id>/<method>; in the desk: frm.call("summary", { x: 1 })
    summary(doc, args, ctx) { return { items: doc.items.length }; },
    settle(doc, args) { doc.append("settlements", { /* … */ }); doc.save(); return { balance: doc.balance }; },
  },
  hasPermission(doc, ptype, user) { /* true|false|undefined; ptype includes "share" — see `sharing` */ },
  permissionQuery(user) { return [["owner", "=", user]]; },
});

// a loose function, exposed at POST /api/method/<app>.services.<file>.lookup
export const lookup = whitelisted((args: { postcode: string }, ctx) => ({ /* … */ }), { allowGuest: false, roles: ["Manager"] });
// callable by Website Users too (portals); their reads inside it see only what the portal grants
export const myPayslips = whitelisted(() => ddcore.db.getList("Payslip", { fields: ["id", "period"] }), { portal: true });
// `methods` is enforced: any other verb is a 405 with an `Allow` header
export const ping = whitelisted(() => "pong", { allowGuest: true, methods: ["GET"] });
```

### Inbound webhooks

A provider that calls *you* needs two things a JSON method does not give: an answer that is not wrapped in
`{data, messages}`, and the exact bytes it signed. Both are opt-ins on a whitelisted method:

```ts
// the verification handshake: the challenge is echoed back as the whole body
export const verify = whitelisted(
  (args) => args["hub.verify_token"] === ddcore.secret("META_VERIFY_TOKEN") ? args["hub.challenge"] : ddcore.throw("Forbidden"),
  { allowGuest: true, methods: ["GET"], raw: { contentType: "text/plain" } },
);

// the event itself: check the signature over the raw body before trusting it
export const receive = whitelisted((args, ctx) => {
  const { rawBody, headers } = ctx.request!;
  const want = "sha256=" + ddcore.crypto.hmacSha256(ddcore.secret("META_APP_SECRET")!, rawBody!);
  if (!ddcore.crypto.timingSafeEqual(want, headers!["x-hub-signature-256"] ?? "")) ddcore.throw("Forbidden");
  // the call arrived as Guest: do the work as the user this integration acts for
  ddcore.runAs(integrationUser, () => { /* reads and writes honour that user's scopes */ });
}, { allowGuest: true, methods: ["POST"] });
```

- `raw: { contentType }` — the returned **string** is the whole body, with that content type. Anything else
  returned is a 500. A thrown error is still a JSON error with its status.
- `ctx.request` (and `ddcore.session.request`) carries `rawBody`, the body exactly as received (UTF-8), and
  `headers`, names lower-cased. `cookie`, `authorization` and `x-ddcore-csrf` are left out on purpose. `args` is
  still parsed from a JSON body; a body that is not JSON (and is not sent as `application/json`) leaves `args`
  empty instead of failing, so read `rawBody`.
- `pathTail: true` — the method also answers below its own path, and reads what came after it, percent-decoded,
  as `ctx.request.pathTail` (`""` when the call named the method alone). This is for a provider that appends to
  the URL it was registered with: a bank that posts PIX events to `<url>/pix`, say. Register
  `/api/method/<app>.services.<file>.<fn>` with the bank and dispatch on the tail:

  ```ts
  export const bank = whitelisted((args, ctx) => {
    if (ctx.request!.pathTail === "pix") { /* the PIX event, in rawBody */ }
  }, { allowGuest: true, methods: ["POST"], pathTail: true });
  ```

  A method without it does not exist at a sub-path: `/api/method/<path>/x` answers the same 404 as an unknown
  method, and `ctx.request` carries no `pathTail`.
  The access log and the Error Log stop at the method's name (`/api/method/<path>/…`), so a sender that can
  only authenticate through its URL may carry a shared token in the tail; compare it with `timingSafeEqual`.
- `ddcore.crypto.hmacSha256(key, data)` → lower-case hex; `ddcore.crypto.timingSafeEqual(a, b)` compares in
  constant time. Never compare a signature with `===`. For a timestamped scheme, also check the timestamp header
  against `Date.now()` and refuse an event older than the window you accept.
  `hmacSha256(key, data, { keyEncoding?, output? })` takes the key as `"utf8"` (default), `"base64"` or `"hex"`
  and answers `"hex"` (default), `"base64"` or `"base64url"`.
- A sender that follows [Standard Webhooks](https://www.standardwebhooks.com/) (the scheme ddcore itself signs
  with, see `webhooks`) is checked in one call. `ddcore.webhooks.verify(secret, headers, rawBody, { toleranceSeconds = 300 })`
  returns a boolean: it decodes a `whsec_<base64>` secret, accepts several space-separated `v1,…` signatures
  (repeated `webhook-signature` headers too), compares in constant time, enforces the timestamp window
  (`toleranceSeconds: Infinity` switches it off), reads header names in any case, and returns `false` (never
  throws) when a header, `headers` or `rawBody` is missing. A missing secret (`ddcore.secret` returns `null` when the variable is
  unset) or an empty one verifies nothing and gives `false`, so a site that forgot to configure it refuses every call:

  ```ts
  export const gateway = whitelisted((args, ctx) => {
    const { headers, rawBody } = ctx.request!;
    if (!ddcore.webhooks.verify(ddcore.secret("GATEWAY_SECRET"), headers, rawBody)) ddcore.throw("Forbidden");
  }, { allowGuest: true, methods: ["POST"] });
  ```

### Calls from another origin

A page served from another origin — a merchant's shop calling your payment methods — can call a method only
when the method opts in with `cors: true` **and** the site lists that origin:

```jsonc
// ddcore.json (or DDCORE_CORS_ORIGINS="https://shop.example.com,https://*.partner.example")
"cors": { "origins": ["https://shop.example.com", "https://*.partner.example"] }
```

```ts
export const status = whitelisted((args) => ({ status: lookUp(args.code) }), { allowGuest: true, cors: true, methods: ["GET"] });
```

- An origin is `scheme://host[:port]`, compared exactly and without regard to case; `https://*.partner.example`
  matches every subdomain over https but not `partner.example` itself; `*` matches any origin. An entry no
  browser would send as an `Origin` (a path, no scheme, a wildcard elsewhere) stops the server at startup.
- The browser's preflight (`OPTIONS /api/method/<path>`) gets `204` with the origin echoed in
  `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods` from the method's `methods` (`GET, POST`
  without them), `Access-Control-Allow-Headers: Authorization, Content-Type, X-Tenant` and a ten-minute
  `Access-Control-Max-Age`. A method without `cors`, or an origin outside the list, gets `403`.
- The call itself gets the origin echoed and `Access-Control-Expose-Headers: X-Request-Id`, on an error as on a
  result, so the page can read either.
- **Never credentials.** No `Access-Control-Allow-Credentials` is ever sent, so the browser does not hand the
  page a response to a call made with the visitor's session cookie. The caller sends
  `Authorization: token <key>:<secret>` — only from a server or an app that can keep a key secret, never from
  a public page — or comes as Guest (`allowGuest: true`).
- Everything else is unchanged: `/api/resource`, every other route and every method without `cors` answer
  without CORS headers, and a request from the site's own pages (or with no `Origin`) is not CORS at all. A
  page the app serves itself (`www`) is on the site's origin and needs none of this.

Hooks for another app's DocTypes: in `ddcore.app.ts`, `docEvents: { "User": { validate(doc) {} }, "*": { onUpdate(doc) {} } }`.
A DocType has **one** controller, its owner's: `defineController` from a second app is refused. To add rules to
someone else's DocType use `docEvents`, and `extendDoctype` for fields, properties and permissions — its
`hasPermission` and `permissionQuery` chain with the owner's (any denial wins, filters are AND-ed). See `extending`.

Every message a person reads goes through `_()`, and the key is its English text. See `i18n`.

## The document (`doc`)

Fields are properties; child tables are arrays. Methods: `insert()`, `save()`, `submit()`, `cancel()`, `delete()`, `reload()`,
`dbSet(field, value)` / `dbSet({ ... })` (writes straight through, no validate — allowed after submission), `append(table, row)`, `isNew()`,
`getDocBeforeSave()`, `hasValueChanged(field)`, `runMethod(name, args)`, `applyWorkflow(action)` (applies a workflow transition and
reloads the document with the new state and docstatus; see `workflows`), `doc.flags` (see below).

### `doc.flags`: context for one write

`doc.flags` is a free-form object that travels with **one write** of that document. What is set on it before
`insert()`, `save()`, `submit()`, `cancel()` or `delete()` is what every hook of that write sees; what a hook sets is
seen by the later hooks of the same write (`validate` can leave a note for `onUpdate`) and is back on the caller's
`doc.flags` when the call returns. It is how server code tells a rule in `validate` that the write is the system's:

```ts
// thing.controller.ts
validate(doc) {
  if (doc.hasValueChanged("category") && !doc.flags.fromMeta) ddcore.throw(_("The category cannot be changed"));
}

// the integration
const doc = ddcore.getDoc("Thing", id);
doc.category = remote.category;
doc.flags.fromMeta = true;
doc.save();
```

The flags are never stored and never leave the server, so a client cannot set them: a write from the Desk, the REST
API or MCP starts with `{}`. They belong to that document only — another document saved inside a hook has its own,
and `getDoc` of the same document again starts empty. Values must be JSON: the flags cross to the engine and back
with the document.

### What a delete leaves behind

A delete — from the desk, the REST API, `doc.delete()`, `ddcore.deleteDoc` or the MCP
`delete_doc` — runs `onTrash`, removes the row with its child rows, attachments, comments,
assignments and shares, then `afterDelete`. In the same transaction, so a refused or rolled-back
delete leaves none of it, it records:

- **A deletion Version**, for every DocType, whether `trackChanges` or not. It has `deleted`
  checked, and its `data` is `{"deleted": { …the document as it was, child rows included… }}`
  rather than a `{"changed": …}` diff. Password and Vault fields, and secret-named columns, are
  left out, as they are from every Version.
- **The Versions the document already had.** They stay behind, so the history of a
  `trackChanges` document ends with its deletion instead of disappearing with it.
- **A `doc.delete` Audit Event** naming who deleted what, with `{"version": "<deletion Version id>"}`
  (see `audit`).

Version, Error Log, Email Delivery and Webhook Delivery write no deletion Version: they are
records already. Their deletion is still audited.

Only a System Manager reads the Versions of a document that no longer exists. To find what was
deleted, list Version with **Deleted** checked, or filter it on the DocType and id. A document
later created under the same id is a different document: other readers of it see its own
Versions only, while a System Manager sees the whole timeline, deletion included.

There is no undelete. The snapshot is there to be read, and a document can be re-created from it
by hand.

## `ddcore.*` (global on the server)

- `ddcore.db.getValue(doctype, id | filters, field | [fields])` — a value or an object (or `null`)
- `ddcore.db.getSingleValue(doctype, field)` — one field of a Single (see "Single DocTypes"); before the first save, the field's default
- `ddcore.db.getList(doctype, { filters, fields, orderBy, limit, start, groupBy })` — respects permissions; `getAll` skips role permissions but still applies user access scopes (see `scopes`)
- `ddcore.db.setValue(doctype, id, field, value)` / `setValue(doctype, id, { ... })` — no validate; updates `modified`; skips role permissions but applies user access scopes, the closed-DocType, workflow and Audit Event refusals (see `scopes`)
- `ddcore.db.count(doctype, filters)`, `ddcore.db.exists(doctype, id | filters)` → the id or `null`; applies user access scopes (see `scopes`)
- `ddcore.db.sql("SELECT ... WHERE x = $1", [v])` — read-only; tables are `tab_<snake>`
- `ddcore.db.lock(key)` — an advisory lock held until the transaction ends: another request locking the same key waits. Use it to make an operation idempotent under concurrency (`ddcore.db.lock("billing:" + contract)`). Inside a tenant the key is the tenant's own (see `tenancy`)
- `ddcore.db.savepoint(fn)` → what `fn` returns. When `fn` throws, its writes are rolled back and the error rethrown, and the transaction goes on. See "Savepoints" below
- `ddcore.externalDb("sql_server").sql("SELECT ... WHERE x = @p1", [v], { timeout })` — read-only query on another database (SQL Server), configured from `DDCORE_SECRET_SQL_SERVER_*`. See `external-db`
- `ddcore.getDoc(doctype, id, { ignorePermissions })`, `ddcore.newDoc(doctype, values)`, `ddcore.deleteDoc(doctype, id, { force })`, `ddcore.rename(doctype, oldID, newID)`
- `ddcore.getDoc(doctype, id, { ignorePermissions: true })` loads a document the user has no role permission to read, so a service can load, change and `save({ ignorePermissions: true })` it under the caller's identity. The user's access scopes and the tenancy wall still apply (see `scopes`), and `doc.reload({ ignorePermissions: true })` reads it again the same way. The document comes back whole, with no field-level redaction: `ddcore.redact` it before a method returns it to a client (see `field-permissions`)
- `ddcore.throw(msg, { title, type })`, `ddcore.msgprint(msg, { title, indicator, alert })`, `ddcore._(text, args)` / `_()`
- `ddcore.session` → `{ user, roles, lang, request }` (`request`: `{ method, path, ip, rawBody, headers }` on a whitelisted call, plus `pathTail` when the method opts in — see *Inbound webhooks*); `ddcore.user()`; `ddcore.getRoles(user)`; `ddcore.hasPermission(doctype, ptype, doc, user?)` (`doc` may be just `{ id, owner }`, or a document id; `user` checks another user's roles, scopes and shares instead of the current one's)
- `ddcore.share.add(doctype, id, user, { write, share, overrideScope })` / `remove(doctype, id, user)` / `list(doctype, id)` — per-user document shares, checked with the current user as sharer. See `sharing`
- `ddcore.users.invite({ email, fullName, roles?, userType? })` / `resendInvite(user)` — create an account and mail its invitation; returns `{ user, expires, link? }`. Without System Manager, only a Website User with no privileged role. See `portal`
- `ddcore.users.createApiKey(user, { label?, days? })` → `{ key, secret, expires }` — issues an API key for another user, what `ddcore apikey` does, so provisioning a tenant (its users and its integration's key) can be one method or an `onTenantCreate`. The caller must be a System Manager outside portal mode, or Admin; anyone else gets a `PermissionError`. Inside a tenant only that tenant's users can get one (another tenant's user is reported as not existing); from the platform space the key is made in the user's own tenant. `days` defaults to the site's `apiKeyDays` (see `auth`), and `expires` is `null` for a key that never expires. The secret is returned this once, since only its hash is stored; an `apikey.create` audit event on the User records who issued which key, never the secret. The key signs in as `Authorization: token key:secret`
- `ddcore.redact(doctype, doc)` → a copy of `doc` as an API read would show it to the current user: Password/Vault blanked and fields above their permission level removed. Server code sees whole documents; redact before a method or report hands one to a client. See `field-permissions`
- `ddcore.cache.get/set(key, value, ttlSeconds)/del` — an in-memory cache per process. `set` stays in the process that made it; `del` drops the key there at once and, when the transaction commits, in every other process on the database that listens (server, `jobs work`, `mcp`)
- `ddcore.http.get(url, opts?)` / `del(url, opts?)` send GET / DELETE requests.
- `ddcore.http.post(url, body?, opts?)` / `put(url, body?, opts?)` / `patch(url, body?, opts?)` send POST / PUT / PATCH requests. Object bodies are JSON-encoded; string bodies are sent unchanged. `bodyEncoding: "base64"` sends a base64 string `body` as raw bytes (`Content-Type` defaults to `application/octet-stream`); `bodyEncoding: "multipart"` sends an array of parts, `{ name, value }` for a text field or `{ name, base64, filename?, contentType? }` for a file, as `multipart/form-data` (the framework sets `Content-Type` with the boundary; a file part's `filename` defaults to its `name`, its `contentType` to `application/octet-stream`). An unknown `bodyEncoding`, invalid base64, a part without `name`, or one with both `value` and `base64` throws.
- All HTTP calls are synchronous and leave from the server. `HttpOpts` accepts `headers` (a string map), `timeout` (seconds, default 15), `responseType` (`"text"`, the default, or `"base64"` for a binary body such as an image or audio) and `maxBytes` (the largest body accepted, default 10 MiB; a larger response throws instead of being cut short). The named method determines the verb; `opts.method` cannot override it. Replace older `post(url, body, { method: "PUT" })` or `get(url, { method: "DELETE" })` workarounds with `put` or `del`.
- `opts.clientCert` presents a client certificate to an API that authenticates by mutual TLS, as most banking APIs do. It is either `{ pfx, password? }` — a PKCS#12 (`.pfx`) file, base64-encoded — or `{ cert, key }` in PEM. The chain in the file is sent with the certificate; the server is still verified against the system roots. Calls with the same certificate share connections, so a token request and the operation after it pay for one handshake. Keep the file in `ddcore.vault` or `ddcore.secret`: `ddcore.http.post(url, body, { clientCert: { pfx: ddcore.vault.get("bank:pfx")!, password: ddcore.secret("bank_pfx_password") } })`. A wrong password or an unreadable file throws a `ValidationError` that does not repeat the material.
- `ddcore.crypto` also makes secrets from `crypto/rand`: `randomToken(bytes = 32)` (base64url, no padding), `randomInt(min, max)` (uniform in `[min, max)`, safe integers, `max > min`) and `sha256(data, { output? })` (hex by default), so a session token is `randomToken()`, a six-digit OTP is `String(ddcore.crypto.randomInt(0, 1e6)).padStart(6, "0")`, and what you store is `sha256(token)`, not the token. `ddcore.utils.randomString(n)` is cryptographically secure too (`a-z0-9`; a fraction rounds up, anything not positive gives `""`, and it throws only above 65536), but `randomToken` carries 6 bits per character against about 5.17 for `randomString`.
- `ddcore.crypto.pfxInfo(pfx, password?)` describes the certificate in a PKCS#12 file, the same base64 input as `clientCert.pfx`, without making a call with it: `{ notBefore, notAfter, subject, issuer, serial, chain }`. `notBefore` and `notAfter` are RFC 3339 instants in UTC (`"2027-01-31T12:00:00Z"`), `subject` and `issuer` distinguished names (`"CN=…"`), `serial` lower-case hex, and `chain` how many certificates came along with the leaf. It never returns key material. A wrong password or an unreadable file throws a `ValidationError` that does not repeat the material, so a method that stores a certificate calls it first: the password is checked on save rather than at the first call to the bank, and the expiry date comes from the file instead of being typed. `ddcore.crypto.certInfo(pem)` does the same for a PEM certificate, the `cert` of the `{ cert, key }` form; the certificates after the first are its chain.
- `HttpResponse` exposes `{ status, body, headers, json() }`. `body` is text (base64 with `responseType: "base64"`) and `json()` parses it. Response header names use Go's canonical HTTP casing (for example, `response.headers["Ratelimit-Remaining"]`); repeated values are joined with `", "`. HTTP error statuses are returned as responses; transport errors throw.
- Redirects are followed, up to 10 hops; `opts.maxRedirects` changes the limit, and `maxRedirects: 0` returns the 3xx itself, with its target in `headers.Location`, so the app chooses whether to follow and with which headers. A hop to another host than the one called (a different port counts), or from `https` to anything else, drops every header the call passed except `Content-Type`: a token sent as `api_access_token` or `X-Api-Key` stays with the host it was meant for, not the object storage or CDN a download redirects to. A hop on the same host keeps them. As in every HTTP client, a 301, 302 or 303 turns the request into a GET without a body, and a 307 or 308 resends both. `ddcore.files.save({ fromUrl })` follows redirects under the same rule.
- `ddcore.push.send(subscription, payload, { ttl, urgency, topic })` → `{ status, body, headers }` — a Web Push message to one browser subscription, encrypted for it and signed with the site's VAPID key (`DDCORE_SECRET_VAPID_*`); any status is returned, so the app deletes a subscription that answered 404 or 410. `ddcore.push.publicKey()` → the key a page subscribes with, or `null`. See `push`
- `ddcore.files.save({ doctype?, id?, fieldname?, filename, isPrivate?, contentType?, content | contentBase64 | fromUrl, headers?, maxBytes?, timeout?, ignorePermissions? })` → the `File` document. Stores bytes the server holds or downloads, with the rules of an upload; see [storage](storage.md#from-server-code).
- `ddcore.files.presign(fileUrl, { ttl?, ignorePermissions? })` → a URL anyone can GET the file at for `ttl` seconds. S3 backend only; see [storage](storage.md#from-server-code).
- `ddcore.enqueue("app.services.mod.fn", args, { queue, runAfter, timeout, maxAttempts, backoff, onStart, onFailure, uniqueKey, runAs })` → the job id. With `uniqueKey`, a job still `queued` under the same key makes the call a no-op that returns that job's id (see [ops](ops.md)).
  Written on the current transaction, so the job exists only if the request commits. `maxAttempts`
  defaults to 3; use `1` for work whose effects outside the database must not be repeated.
  `onStart` / `onFailure` are method paths called as `fn(args, job)`, each in a transaction of its
  own, so a document can show that its job is running or that it failed. See `ops` → "Lifecycle
  callbacks". `runAs` is the user the job acts as: without it a job runs with permissions ignored,
  with it the body and both callbacks run under that user's roles and access scopes.
- `ddcore.runAs(user, fn)` → what `fn` returns. Runs `fn` under `user`'s roles and access scopes, in
  the current transaction, and restores the caller afterwards, even when `fn` throws. It is how a
  job, a scheduled method or a guest webhook gets the isolation a request has; the user must exist
  and be enabled. See `scopes` → "Running as a user: `runAs`"
- `ddcore.callMethod("app.services.mod.fn", args)` → what the function returns. Calls an exported function of any loaded module as `fn(args, ctx)`, now, in the current transaction; no role or `whitelisted` check. See `conventions` → "Calling another app's server code"
- `ddcore.isTest()` → `true` inside `ddcore test`; `ddcore.isJob()` → `true` inside a background job
- `ddcore.publish(event, payload, { user, doctype, id })` — SSE to the desk when the transaction commits. With `user`, only that user's sessions; with `doctype` and `id`, only sessions that may read that document (with `doctype` alone, the DocType) — the audience `doc_update` has. The name is letters, digits and `_ . : -`; prefix it with the app's name. The desk listens with `frm.onRealtime` / `ddcore.realtime.on` (see `form-api`)
- `ddcore.log.info/warn/error`
- `ddcore.utils`: `flt(v, precision)`, `cint`, `cstr`, `getdate`, `nowdate()`, `now()`, `formatDate(d, "dd/mm/yyyy")`, `addDays`, `addMonths`, `addYears`,
  `getFirstDay`, `getLastDay`, `dateDiff(a, b)`, `monthDiff(a, b)`, `formatCurrency(v)`, `roundTo`, `randomString` (cryptographically secure)
- `ddcore.utils` for money: `currencyPrecision()`, `roundCurrency(v)`, `splitAmount(total, n)`

`nowdate()` and `now()` are the site's wall clock (`ddcore.json:timezone`), the same day and hour the desk sees.
A `Datetime` written without an offset — which is what `now()` returns — is read on that same clock.

### Savepoints

A failed statement aborts the whole transaction in Postgres: a `try/catch` catches the error,
and every later query fails with `current transaction is aborted`. `ddcore.db.savepoint(fn)`
runs `fn` inside a savepoint. When `fn` throws, for a SQL error or for any other reason, only
`fn`'s work is undone, the error is rethrown, and the code that catches it carries on in the
same transaction. Undone with the writes are the `msgprint`s, `publish`es and other
after-commit effects `fn` made. When `fn` returns, its work stays and its value is returned.
Savepoints nest.

A value a unique index refuses, from `insert`, `save` or `ddcore.db.setValue`, throws a
`DuplicateEntryError`. Catching it turns an idempotent create into an ordinary outcome, with
no lock around it:

```ts
export function createCharge(args: { reference: string; amount: number }) {
  const doc = ddcore.newDoc("Charge", { external_reference: args.reference, amount: args.amount });
  try {
    ddcore.db.savepoint(() => doc.insert());
  } catch (e: any) {
    if (e.name !== "DuplicateEntryError") throw e;
    // a retry: return the charge the first call made
    return ddcore.getDoc("Charge", ddcore.db.exists("Charge", { external_reference: args.reference })!);
  }
  return doc;
}
```

`fn` is synchronous, as all server code is. A savepoint needs the request's transaction, which
every controller, method and job has.

### Money

The framework rounds a `Currency` field on its way into the database, at the
site's precision and under the site's rounding rule (see `fieldtypes`). By the
time `validate` runs, a Currency field on `doc` is **already rounded**, and
anything the hook assigns is rounded again before the insert. So app code needs
these only for arithmetic *between* fields:

```ts
u.roundCurrency(subtotal * 1.07)        // the way the server is about to store it
u.currencyPrecision()                    // 2 on a USD site, 0 on a JPY one
u.splitAmount(100, 3)                    // [33.34, 33.33, 33.33] — sums to exactly 100
```

`roundTo(v, 2)` hardcodes an answer the site may not share; `roundCurrency` asks.
And `splitAmount` exists because three rounded thirds of 100.00 are 33.33 each
and leave the schedule a cent short of its total — the residue lands on the
earliest parts, always the same way, so a schedule recomputed is a schedule
unchanged.

Two paths deliberately bypass all of this, because they bypass the document
layer entirely: `ddcore.db.sql` and a patch's SQL. What they write is what the
column gets.

Filters: `{ field: value, other: [">", 10] }` or `[["field", "=", v], ["Child Table", "field", ">", v]]`.
Operators: `= != > >= < <= like not like in not in between is set not set`, plus the tree
operators `descendants of`, `descendants of (inclusive)`, `not descendants of`, `ancestors of`
and `not ancestors of` on a tree DocType's `id` or a Link to one (see `trees`).
In the list shape, an item `{ any: [group, ...] }` ORs groups of filters, each written in either
shape above. A row matches when every filter of at least one group does, and `{ any: [] }`
matches nothing. `[["amount", ">", 0], { any: [[["status", "=", "Open"]], [["status", "=", "Overdue"], ["owner", "=", user]]] }]`
is `amount > 0 AND (status = Open OR (status = Overdue AND owner = user))`. The REST list's
`or_filters` holds a single OR group, and the Desk's search already uses it; an `any` item is how
a list adds an OR of its own.
`fields` accepts aggregates: `"count(id) as n"`, `"sum(amount) as total"`.

## Tests

```ts
import "@ddcore/sdk/test";
describe("Order", () => {
  beforeEach(() => { /* fixtures */ });
  it("adds up", () => { const o = ddcore.newDoc("Order", {/* … */}).insert(); expect(o.total).toBe(20); });
  it("refuses", () => { expect(() => ddcore.newDoc("Order").insert()).toThrow("required fields"); });
});
```
`expect`: toBe, toEqual, toBeTruthy/Falsy, toBeNull, toBeDefined, toContain, toBeGreaterThan(OrEqual), toBeLessThan(OrEqual),
toBeCloseTo, toHaveLength, toMatch, toThrow(text|regex), `.not`.

`beforeEach`, `afterEach` and `beforeAll` belong to the `describe` they are written in. Written
outside any `describe`, they belong to their file: they wrap that file's tests and no other file's,
whatever app it is in.

Tests run as `Admin`, which no role or User Permission restricts. To see a permission or a scope
at work, run part of a test as another user with `ddcore.test.asUser(user, fn)`:

```ts
it("a parish manager sees only their parish", () => {
  ddcore.newDoc("User", { email: "manager@x.test", full_name: "Manager", roles: [{ role: "Parish Manager" }] }).insert();
  ddcore.newDoc("User Permission", { user: "manager@x.test", allow: "Parish", for_value: "PAR-0001" }).insert();
  ddcore.test.asUser("manager@x.test", () => {
    expect(ddcore.db.getList("Tither", { fields: ["parish"] }).every((t) => t.parish === "PAR-0001")).toBe(true);
    expect(() => ddcore.getDoc("Tither", otherParishTither)).toThrow();
  });
});
```
`fn` runs with that user's roles, User Permission scopes, shares and user type, inside the test's
transaction. Users and User Permissions the test inserted count, and roll back with it.
`ddcore.session.user` names the user, `asUser` returns what `fn` returns, and the previous user is
back afterwards, even when `fn` throws. `ddcore.test` exists only inside `ddcore test`.

## Single DocTypes (settings)

Declare `isSingle: true` for one configuration per DocType and instance. Singles use
normal typed tables, standard metadata, validation, hooks and child tables, with
`id: "singleton"` and `docstatus: 0` enforced by PostgreSQL.

```ts
const settings = ddcore.getDoc("Project Settings");
settings.planning_enabled = false;
settings.save();
settings.reload();
```

To read one field without loading the document (and its child tables), use
`ddcore.db.getSingleValue("Project Settings", "planning_enabled")`; like `db.getValue`, it
skips role permissions.

Omitting the id is supported only for Singles. Before the first save, `getDoc`
returns an unsaved document with the usual defaults and empty child tables; reading
never inserts a record. Defaults do not overwrite persisted values. Saving requires
`write`, including the first save; `create` alone does not grant it. Reads require
`read`, including reads of defaults. Explicit privileged contexts keep their usual
meaning. Owner-only permissions have no owner to match before the first save.

The first save runs insert hooks and `onUpdate`; subsequent saves use the update
lifecycle. Rollback also rolls back settings and children. Two competing first
saves produce one success and one duplicate conflict. Updates use the ordinary
`modified` timestamp conflict check; reload before retrying a rejected save.

REST uses `GET` / `PUT /api/resource/{doctype}/singleton`; PUT also performs the
first save. POST to `/api/resource/{doctype}` inserts only once. Other identities,
deleting, renaming a record, submitting, cancelling and amending are unsupported.
Singles cannot declare `idGeneration`, `allowRename`, `submittable`, or `isChild`.
List/count/database queries see only persisted rows. Export of Singles is not yet
supported. Password redaction and attachment/history permissions still apply.

Desk opens `/app/{doctype}` directly as a settings form. Browser scripts may use
`await ddcore.db.getSingle("Project Settings")` and
`await ddcore.db.setValue("Project Settings", "singleton", values)`.
