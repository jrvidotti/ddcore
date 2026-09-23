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
  methods: {                            // POST /api/resource/Order/<id>/<method>; in the desk: frm.call("summary", { x: 1 })
    summary(doc, args, ctx) { return { items: doc.items.length }; },
    settle(doc, args) { doc.append("settlements", { /* … */ }); doc.save(); return { balance: doc.balance }; },
  },
  hasPermission(doc, ptype, user) { /* true|false|undefined; ptype includes "share" — see `sharing` */ },
  permissionQuery(user) { return [["owner", "=", user]]; },
});

// a loose function, exposed at POST /api/method/<app>.services.<file>.lookup
export const lookup = whitelisted((args: { postcode: string }, ctx) => ({ /* … */ }), { allowGuest: false, roles: ["Manager"] });
```

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

## `ddcore.*` (global on the server)

- `ddcore.db.getValue(doctype, id | filters, field | [fields])` — a value or an object (or `null`)
- `ddcore.db.getList(doctype, { filters, fields, orderBy, limit, start, groupBy })` — respects permissions; `getAll` skips role permissions but still applies user access scopes (see `scopes`)
- `ddcore.db.setValue(doctype, id, field, value)` / `setValue(doctype, id, { ... })` — no validate; updates `modified`; skips role permissions but applies user access scopes, the closed-DocType, workflow and Audit Event refusals (see `scopes`)
- `ddcore.db.count(doctype, filters)`, `ddcore.db.exists(doctype, id | filters)` → the id or `null`; applies user access scopes (see `scopes`)
- `ddcore.db.sql("SELECT ... WHERE x = $1", [v])` — read-only; tables are `tab_<snake>`
- `ddcore.externalDb("sql_server").sql("SELECT ... WHERE x = @p1", [v], { timeout })` — read-only query on another database (SQL Server), configured from `DDCORE_SECRET_SQL_SERVER_*`. See `external-db`
- `ddcore.getDoc(doctype, id)`, `ddcore.newDoc(doctype, values)`, `ddcore.deleteDoc(doctype, id, { force })`, `ddcore.rename(doctype, oldID, newID)`
- `ddcore.throw(msg, { title, type })`, `ddcore.msgprint(msg, { title, indicator, alert })`, `ddcore._(text, args)` / `_()`
- `ddcore.session` → `{ user, roles, lang, request }`; `ddcore.user()`; `ddcore.getRoles(user)`; `ddcore.hasPermission(doctype, ptype, doc)` (`doc` may be just `{ id, owner }`)
- `ddcore.share.add(doctype, id, user, { write, share, overrideScope })` / `remove(doctype, id, user)` / `list(doctype, id)` — per-user document shares, checked with the current user as sharer. See `sharing`
- `ddcore.redact(doctype, doc)` → a copy of `doc` as an API read would show it to the current user: Password/Vault blanked and fields above their permission level removed. Server code sees whole documents; redact before a method or report hands one to a client. See `field-permissions`
- `ddcore.cache.get/set(key, value, ttlSeconds)/del`
- `ddcore.http.get(url, opts?)` / `del(url, opts?)` send GET / DELETE requests.
- `ddcore.http.post(url, body?, opts?)` / `put(url, body?, opts?)` / `patch(url, body?, opts?)` send POST / PUT / PATCH requests. Object bodies are JSON-encoded; string bodies are sent unchanged.
- All HTTP calls are synchronous and leave from the server. `HttpOpts` accepts `headers` (a string map) and `timeout` (seconds, default 15). The named method determines the verb; `opts.method` cannot override it. Replace older `post(url, body, { method: "PUT" })` or `get(url, { method: "DELETE" })` workarounds with `put` or `del`.
- `HttpResponse` exposes `{ status, body, headers, json() }`. `body` is text and `json()` parses it. Response header names use Go's canonical HTTP casing (for example, `response.headers["Ratelimit-Remaining"]`); repeated values are joined with `", "`. HTTP error statuses are returned as responses; transport errors throw.
- `ddcore.enqueue("app.services.mod.fn", args, { queue, runAfter, timeout, maxAttempts })` → the job id.
  Written on the current transaction, so the job exists only if the request commits. `maxAttempts`
  defaults to 3; use `1` for work whose effects outside the database must not be repeated.
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
