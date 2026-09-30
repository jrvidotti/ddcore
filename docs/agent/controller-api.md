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
  // …
}, { allowGuest: true, methods: ["POST"] });
```

- `raw: { contentType }` — the returned **string** is the whole body, with that content type. Anything else
  returned is a 500. A thrown error is still a JSON error with its status.
- `ctx.request` (and `ddcore.session.request`) carries `rawBody`, the body exactly as received (UTF-8), and
  `headers`, names lower-cased. `cookie`, `authorization` and `x-ddcore-csrf` are left out on purpose. `args` is
  still parsed from a JSON body; a body that is not JSON (and is not sent as `application/json`) leaves `args`
  empty instead of failing, so read `rawBody`.
- `ddcore.crypto.hmacSha256(key, data)` → lower-case hex; `ddcore.crypto.timingSafeEqual(a, b)` compares in
  constant time. Never compare a signature with `===`. For a timestamped scheme, also check the timestamp header
  against `Date.now()` and refuse an event older than the window you accept.

Hooks for another app's DocTypes: in `ddcore.app.ts`, `docEvents: { "User": { validate(doc) {} }, "*": { onUpdate(doc) {} } }`.
A DocType has **one** controller, its owner's: `defineController` from a second app is refused. To add rules to
someone else's DocType use `docEvents`, and `extendDoctype` for fields, properties and permissions — its
`hasPermission` and `permissionQuery` chain with the owner's (any denial wins, filters are AND-ed). See `extending`.

Every message a person reads goes through `_()`, and the key is its English text. See `i18n`.

## The document (`doc`)

Fields are properties; child tables are arrays. Methods: `insert()`, `save()`, `submit()`, `cancel()`, `delete()`, `reload()`,
`dbSet(field, value)` / `dbSet({ ... })` (writes straight through, no validate — allowed after submission), `append(table, row)`, `isNew()`,
`getDocBeforeSave()`, `hasValueChanged(field)`, `runMethod(name, args)`, `applyWorkflow(action)` (applies a workflow transition and
reloads the document with the new state and docstatus; see `workflows`), `doc.flags` (free-form, per request).

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
- `ddcore.externalDb("sql_server").sql("SELECT ... WHERE x = @p1", [v], { timeout })` — read-only query on another database (SQL Server), configured from `DDCORE_SECRET_SQL_SERVER_*`. See `external-db`
- `ddcore.getDoc(doctype, id)`, `ddcore.newDoc(doctype, values)`, `ddcore.deleteDoc(doctype, id, { force })`, `ddcore.rename(doctype, oldID, newID)`
- `ddcore.throw(msg, { title, type })`, `ddcore.msgprint(msg, { title, indicator, alert })`, `ddcore._(text, args)` / `_()`
- `ddcore.session` → `{ user, roles, lang, request }` (`request`: `{ method, path, ip, rawBody, headers }` on a whitelisted call — see *Inbound webhooks*); `ddcore.user()`; `ddcore.getRoles(user)`; `ddcore.hasPermission(doctype, ptype, doc)` (`doc` may be just `{ id, owner }`)
- `ddcore.share.add(doctype, id, user, { write, share, overrideScope })` / `remove(doctype, id, user)` / `list(doctype, id)` — per-user document shares, checked with the current user as sharer. See `sharing`
- `ddcore.users.invite({ email, fullName, roles?, userType? })` / `resendInvite(user)` — create an account and mail its invitation; returns `{ user, expires, link? }`. Without System Manager, only a Website User with no privileged role. See `portal`
- `ddcore.redact(doctype, doc)` → a copy of `doc` as an API read would show it to the current user: Password/Vault blanked and fields above their permission level removed. Server code sees whole documents; redact before a method or report hands one to a client. See `field-permissions`
- `ddcore.cache.get/set(key, value, ttlSeconds)/del`
- `ddcore.http.get(url, opts?)` / `del(url, opts?)` send GET / DELETE requests.
- `ddcore.http.post(url, body?, opts?)` / `put(url, body?, opts?)` / `patch(url, body?, opts?)` send POST / PUT / PATCH requests. Object bodies are JSON-encoded; string bodies are sent unchanged.
- All HTTP calls are synchronous and leave from the server. `HttpOpts` accepts `headers` (a string map) and `timeout` (seconds, default 15). The named method determines the verb; `opts.method` cannot override it. Replace older `post(url, body, { method: "PUT" })` or `get(url, { method: "DELETE" })` workarounds with `put` or `del`.
- `HttpResponse` exposes `{ status, body, headers, json() }`. `body` is text and `json()` parses it. Response header names use Go's canonical HTTP casing (for example, `response.headers["Ratelimit-Remaining"]`); repeated values are joined with `", "`. HTTP error statuses are returned as responses; transport errors throw.
- `ddcore.enqueue("app.services.mod.fn", args, { queue, runAfter, timeout, maxAttempts, backoff, onStart, onFailure })` → the job id.
  Written on the current transaction, so the job exists only if the request commits. `maxAttempts`
  defaults to 3; use `1` for work whose effects outside the database must not be repeated.
  `onStart` / `onFailure` are method paths called as `fn(args, job)`, each in a transaction of its
  own, so a document can show that its job is running or that it failed. See `ops` → "Lifecycle
  callbacks".
- `ddcore.publish(event, payload, { user })` — SSE to the desk
- `ddcore.log.info/warn/error`
- `ddcore.utils`: `flt(v, precision)`, `cint`, `cstr`, `getdate`, `nowdate()`, `now()`, `formatDate(d, "dd/mm/yyyy")`, `addDays`, `addMonths`, `addYears`,
  `getFirstDay`, `getLastDay`, `dateDiff(a, b)`, `monthDiff(a, b)`, `formatCurrency(v)`, `roundTo`, `randomString`
- `ddcore.utils` for money: `currencyPrecision()`, `roundCurrency(v)`, `splitAmount(total, n)`

`nowdate()` and `now()` are the site's wall clock (`ddcore.json:timezone`), the same day and hour the desk sees.
A `Datetime` written without an offset — which is what `now()` returns — is read on that same clock.

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
