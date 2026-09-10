# Controllers e a API do servidor

```ts
import { defineController, whitelisted, _ } from "@cerne/sdk";
import type { Pedido } from "../../.cerne/types";

export default defineController<Pedido>("Pedido", {
  beforeInsert(doc) {},                 // antes de nomear
  validate(doc, ctx) {                  // toda gravação (insert e update)
    doc.total = (doc.itens || []).reduce((s, i) => s + cerne.utils.flt(i.qtd) * cerne.utils.flt(i.valor), 0);
    if (doc.total < 0) cerne.throw(_("Total negativo"), { title: _("Pedido") });
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

Hooks para DocTypes de outras apps: em `cerne.app.ts`, `docEvents: { "User": { validate(doc) {} }, "*": { onUpdate(doc) {} } }`.

## O documento (`doc`)

Campos como propriedades; tabelas filhas são arrays. Métodos: `insert()`, `save()`, `submit()`, `cancel()`, `delete()`, `reload()`,
`dbSet(campo, valor)` / `dbSet({ ... })` (grava direto, sem validate — permitido após envio), `append(tabela, linha)`, `isNew()`,
`getDocBeforeSave()`, `hasValueChanged(campo)`, `runMethod(nome, args)`, `doc.flags` (livre, por request).

## `cerne.*` (global no servidor)

- `cerne.db.getValue(doctype, nome | filtros, campo | [campos])` — valor ou objeto (ou `null`)
- `cerne.db.getList(doctype, { filters, fields, orderBy, limit, start, groupBy })` — respeita permissões; `getAll` ignora
- `cerne.db.setValue(doctype, nome, campo, valor)` / `setValue(doctype, nome, { ... })` — sem validate; atualiza `modified`
- `cerne.db.count(doctype, filtros)`, `cerne.db.exists(doctype, nome | filtros)` → nome ou `null`
- `cerne.db.sql("SELECT ... WHERE x = $1", [v])` — somente leitura; tabelas `tab_<snake>`
- `cerne.getDoc(doctype, nome)`, `cerne.newDoc(doctype, valores)`, `cerne.deleteDoc(doctype, nome, { force })`
- `cerne.throw(msg, { title, type })`, `cerne.msgprint(msg, { title, indicator, alert })`, `cerne._(texto, args)` / `_()`
- `cerne.session` → `{ user, roles, lang, request }`; `cerne.user()`; `cerne.getRoles(user)`; `cerne.hasPermission(doctype, ptype, doc)`
- `cerne.cache.get/set(key, valor, ttlSegundos)/del`
- `cerne.http.get(url, { headers, timeout })` / `post(url, body)` → `{ status, body, json() }` (a chamada externa sai do servidor)
- `cerne.enqueue("app.services.mod.fn", args, { queue, runAfter })` → id do job
- `cerne.publish(evento, payload, { user })` — SSE para o desk
- `cerne.log.info/warn/error`
- `cerne.utils`: `flt(v, precisao)`, `cint`, `cstr`, `getdate`, `nowdate()`, `now()`, `formatDate(d, "dd/mm/yyyy")`, `addDays`, `addMonths`, `addYears`,
  `getFirstDay`, `getLastDay`, `dateDiff(a, b)`, `monthDiff(a, b)`, `formatCurrency(v)`, `roundTo`, `randomString`

Filtros: `{ campo: valor, outro: [">", 10] }` ou `[["campo", "=", v], ["Tabela Filha", "campo", ">", v]]`.
Operadores: `= != > >= < <= like not like in not in between is set not set`.
Campos em `fields` aceitam agregados: `"count(name) as n"`, `"sum(valor) as total"`.

## Testes

```ts
import "@cerne/sdk/test";
describe("Pedido", () => {
  beforeEach(() => { /* fixtures */ });
  it("soma", () => { const p = cerne.newDoc("Pedido", {...}).insert(); expect(p.total).toBe(20); });
  it("recusa", () => { expect(() => cerne.newDoc("Pedido").insert()).toThrow("obrigat"); });
});
```
`expect`: toBe, toEqual, toBeTruthy/Falsy, toBeNull, toBeDefined, toContain, toBeGreaterThan(OrEqual), toBeLessThan(OrEqual),
toBeCloseTo, toHaveLength, toMatch, toThrow(texto|regex), `.not`.
