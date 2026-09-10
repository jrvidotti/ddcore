# Controllers e a API do servidor

```ts
import { defineController, whitelisted, _ } from "@ddcore/sdk";
import type { Pedido } from "../../.ddcore/types";

export default defineController<Pedido>("Pedido", {
  beforeInsert(doc) {},                 // antes de nomear
  validate(doc, ctx) {                  // toda gravação (insert e update)
    doc.total = (doc.itens || []).reduce((s, i) => s + ddcore.utils.flt(i.qtd) * ddcore.utils.flt(i.valor), 0);
    if (doc.total < 0) ddcore.throw(_("Total negativo"), { title: _("Pedido") });
  },
  onUpdate(doc) {},                     // depois de gravar (insert e update)
  onSubmit(doc) { doc.dbSet("situacao", "Vigente"); },
  onCancel(doc) {},
  onUpdateAfterSubmit(doc) {},
  onTrash(doc) {}, afterDelete(doc) {},
  beforeRename(doc) {}, afterRename(doc) {},
  methods: {                            // POST /api/resource/Pedido/<name>/<metodo>; no desk: frm.call("resumo", { x: 1 })
    resumo(doc, args, ctx) { return { itens: doc.itens.length }; },
    registrarBaixa(doc, args) { doc.append("baixas", { ... }); doc.save(); return { saldo: doc.saldo }; },
  },
  hasPermission(doc, ptype, user) { /* true|false|undefined */ },
  permissionQuery(user) { return [["owner", "=", user]]; },
});

// função avulsa exposta em POST /api/method/<app>.services.<arquivo>.consultar
export const consultar = whitelisted((args: { cep: string }, ctx) => ({ ... }), { allowGuest: false, roles: ["Gestor"] });
```

Hooks para DocTypes de outras apps: em `ddcore.app.ts`, `docEvents: { "User": { validate(doc) {} }, "*": { onUpdate(doc) {} } }`.

## O documento (`doc`)

Campos como propriedades; tabelas filhas são arrays. Métodos: `insert()`, `save()`, `submit()`, `cancel()`, `delete()`, `reload()`,
`dbSet(campo, valor)` / `dbSet({ ... })` (grava direto, sem validate — permitido após envio), `append(tabela, linha)`, `isNew()`,
`getDocBeforeSave()`, `hasValueChanged(campo)`, `runMethod(nome, args)`, `doc.flags` (livre, por request).

## `ddcore.*` (global no servidor)

- `ddcore.db.getValue(doctype, nome | filtros, campo | [campos])` — valor ou objeto (ou `null`)
- `ddcore.db.getList(doctype, { filters, fields, orderBy, limit, start, groupBy })` — respeita permissões; `getAll` ignora
- `ddcore.db.setValue(doctype, nome, campo, valor)` / `setValue(doctype, nome, { ... })` — sem validate; atualiza `modified`
- `ddcore.db.count(doctype, filtros)`, `ddcore.db.exists(doctype, nome | filtros)` → nome ou `null`
- `ddcore.db.sql("SELECT ... WHERE x = $1", [v])` — somente leitura; tabelas `tab_<snake>`
- `ddcore.getDoc(doctype, nome)`, `ddcore.newDoc(doctype, valores)`, `ddcore.deleteDoc(doctype, nome, { force })`
- `ddcore.throw(msg, { title, type })`, `ddcore.msgprint(msg, { title, indicator, alert })`, `ddcore._(texto, args)` / `_()`
- `ddcore.session` → `{ user, roles, lang, request }`; `ddcore.user()`; `ddcore.getRoles(user)`; `ddcore.hasPermission(doctype, ptype, doc)`
- `ddcore.cache.get/set(key, valor, ttlSegundos)/del`
- `ddcore.http.get(url, { headers, timeout })` / `post(url, body)` → `{ status, body, json() }` (a chamada externa sai do servidor)
- `ddcore.enqueue("app.services.mod.fn", args, { queue, runAfter })` → id do job
- `ddcore.publish(evento, payload, { user })` — SSE para o desk
- `ddcore.log.info/warn/error`
- `ddcore.utils`: `flt(v, precisao)`, `cint`, `cstr`, `getdate`, `nowdate()`, `now()`, `formatDate(d, "dd/mm/yyyy")`, `addDays`, `addMonths`, `addYears`,
  `getFirstDay`, `getLastDay`, `dateDiff(a, b)`, `monthDiff(a, b)`, `formatCurrency(v)`, `roundTo`, `randomString`

Filtros: `{ campo: valor, outro: [">", 10] }` ou `[["campo", "=", v], ["Tabela Filha", "campo", ">", v]]`.
Operadores: `= != > >= < <= like not like in not in between is set not set`.
Campos em `fields` aceitam agregados: `"count(name) as n"`, `"sum(valor) as total"`.

## Testes

```ts
import "@ddcore/sdk/test";
describe("Pedido", () => {
  beforeEach(() => { /* fixtures */ });
  it("soma", () => { const p = ddcore.newDoc("Pedido", {...}).insert(); expect(p.total).toBe(20); });
  it("recusa", () => { expect(() => ddcore.newDoc("Pedido").insert()).toThrow("obrigat"); });
});
```
`expect`: toBe, toEqual, toBeTruthy/Falsy, toBeNull, toBeDefined, toContain, toBeGreaterThan(OrEqual), toBeLessThan(OrEqual),
toBeCloseTo, toHaveLength, toMatch, toThrow(texto|regex), `.not`.
