# Controllers and the server API

```ts
import { defineController, whitelisted, _ } from "@ddcore/sdk";
import type { Order } from "../../.ddcore/types";

export default defineController<Order>("Order", {
  beforeInsert(doc) {},                 // before naming
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
  methods: {                            // POST /api/resource/Order/<name>/<method>; in the desk: frm.call("summary", { x: 1 })
    summary(doc, args, ctx) { return { items: doc.items.length }; },
    settle(doc, args) { doc.append("settlements", { /* … */ }); doc.save(); return { balance: doc.balance }; },
  },
  hasPermission(doc, ptype, user) { /* true|false|undefined */ },
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
`getDocBeforeSave()`, `hasValueChanged(field)`, `runMethod(name, args)`, `doc.flags` (free-form, per request).

## `ddcore.*` (global on the server)

- `ddcore.db.getValue(doctype, name | filters, field | [fields])` — a value or an object (or `null`)
- `ddcore.db.getList(doctype, { filters, fields, orderBy, limit, start, groupBy })` — respects permissions; `getAll` ignores them
- `ddcore.db.setValue(doctype, name, field, value)` / `setValue(doctype, name, { ... })` — no validate; updates `modified`
- `ddcore.db.count(doctype, filters)`, `ddcore.db.exists(doctype, name | filters)` → the name or `null`
- `ddcore.db.sql("SELECT ... WHERE x = $1", [v])` — read-only; tables are `tab_<snake>`
- `ddcore.getDoc(doctype, name)`, `ddcore.newDoc(doctype, values)`, `ddcore.deleteDoc(doctype, name, { force })`
- `ddcore.throw(msg, { title, type })`, `ddcore.msgprint(msg, { title, indicator, alert })`, `ddcore._(text, args)` / `_()`
- `ddcore.session` → `{ user, roles, lang, request }`; `ddcore.user()`; `ddcore.getRoles(user)`; `ddcore.hasPermission(doctype, ptype, doc)`
- `ddcore.cache.get/set(key, value, ttlSeconds)/del`
- `ddcore.http.get(url, { headers, timeout })` / `post(url, body)` → `{ status, body, json() }` (the call leaves from the server)
- `ddcore.enqueue("app.services.mod.fn", args, { queue, runAfter })` → the job id
- `ddcore.publish(event, payload, { user })` — SSE to the desk
- `ddcore.log.info/warn/error`
- `ddcore.utils`: `flt(v, precision)`, `cint`, `cstr`, `getdate`, `nowdate()`, `now()`, `formatDate(d, "dd/mm/yyyy")`, `addDays`, `addMonths`, `addYears`,
  `getFirstDay`, `getLastDay`, `dateDiff(a, b)`, `monthDiff(a, b)`, `formatCurrency(v)`, `roundTo`, `randomString`

`nowdate()` and `now()` are the site's wall clock (`ddcore.json:timezone`), the same day and hour the desk sees.

Filters: `{ field: value, other: [">", 10] }` or `[["field", "=", v], ["Child Table", "field", ">", v]]`.
Operators: `= != > >= < <= like not like in not in between is set not set`.
`fields` accepts aggregates: `"count(name) as n"`, `"sum(amount) as total"`.

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
