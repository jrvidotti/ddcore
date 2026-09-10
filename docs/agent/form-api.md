# Scripts de formulário (desk)

Arquivo `doctypes/<snake>/<snake>.form.ts`, compilado pelo servidor e carregado ao abrir o formulário.

```ts
import { defineForm, cerne } from "@cerne/desk-sdk";
import type { Lancamento } from "../../.cerne/types";

defineForm<Lancamento>("Lancamento", {
  setup(frm) { frm.setQuery("categoria", () => ({ filters: { natureza: frm.doc.natureza } })); },
  refresh(frm) {
    frm.setDfProperty("baixas", "cannotAddRows", true);
    if (frm.isNew) return;
    frm.addIndicator(__("Saldo: {0}", [cerne.format.currency(frm.doc.saldo_devedor)]), "red");
    frm.addButton(__("Registrar pagamento"), () => baixar(frm), __("Ações"));
    frm.setInnerGroupAsPrimary(__("Ações"));
  },
  onChange: { natureza(frm) { frm.setValue("categoria", null); } },
  validate(frm) { /* return false para impedir o save */ },
  afterSave(frm) {},
});
```

## `frm`

`doc`, `doctype`, `meta`, `isNew`, `isDirty`, `docstatus`, `perm`, `getValue`, `setValue(campo | {..}, valor)`, `field(campo)`,
`setDfProperty(campo, prop, valor)` (`hidden`, `readOnly`, `reqd`, `label`, `options`, `cannotAddRows`, `cannotDeleteRows`),
`setQuery(campo, () => ({ filters }))`, `toggleDisplay/toggleReqd/toggleEnable`, `addButton(label, fn, grupo)`, `removeButton`,
`setPrimaryAction(label, fn)`, `setInnerGroupAsPrimary(grupo)`, `addIndicator(label, cor)`, `addChild(tabela, valores)`, `removeChild(tabela, idx)`,
`trigger(campo)`, `save()`, `submit()`, `cancel()`, `reload()`,
`call(metodo, args, { reload })` → chama `methods.<metodo>` do controller e recarrega o doc.

## `cerne` no desk

- `cerne.call("app.services.arquivo.fn", args)` — função whitelisted
- `cerne.db.getValue/getList/count/getDoc/setValue/insert` (assíncronos: use `await`)
- `cerne.ui.Dialog({ title, fields, values, primaryLabel, primaryAction(values, dlg), dangerLabel, dangerAction(values, dlg), onChange(campo, values, dlg), size })` → `dlg.show()/hide()/setValue/getValue/setHtml(campoHTML, html)`
- `cerne.ui.msgprint(msg, { title, indicator })`, `cerne.ui.toast`, `cerne.ui.confirm(msg)`, `cerne.ui.prompt(title, fields)`, `cerne.ui.showError(e)`
- `cerne.format.currency/date/number/value`, `cerne.datetime.today/addMonths/addDays/monthStart/monthEnd`
- `__("texto", [args])` — tradução

### Datas e horas

`cerne.datetime` trabalha com **datas civis** (`"YYYY-MM-DD"`) e tem exatamente a semântica de
`cerne.utils` no servidor: `today()` é o dia do relógio **local** do navegador (não o dia UTC) e
`addMonths` limita o dia ao fim do mês de destino — `addMonths("2026-01-31", 1) === "2026-02-28"`.

Campos `Datetime`, ao contrário, são **instantes**: viajam em ISO UTC e o control os exibe/recebe
no fuso do navegador. Não fatie a string (`v.slice(0, 16)`) para preencher um `datetime-local`, isso
mostra UTC como se fosse hora local; use `$lib/datetime` (`toDatetimeLocal`/`fromDatetimeLocal`).

## `defineListView`

Ajusta a listagem de um DocType a partir de um script global (`client/*.ts`):

```ts
import { defineListView, cerne } from "@cerne/desk-sdk";

defineListView("Lancamento", {
  columns: ["contrato", "competencia", "valor", "saldo_devedor"], // no lugar do inListView da meta
  filters: { situacao: "Em Aberto" },   // filtros iniciais (a query string da URL ainda vence)
  orderBy: "vencimento asc",
  pageSize: 50,
  formatters: { valor: (v, row) => cerne.format.currency(v) }, // texto da célula
  indicator: (row) => (row.saldo_devedor > 0 ? { label: "Em aberto", color: "red" } : { label: "Quitado", color: "green" }),
});
```

Todas as chaves são opcionais. `formatters` devolve **texto** (não HTML); `indicator` substitui a
coluna de status padrão e pode devolver `null` para não mostrar nada na linha.

Scripts globais (`client/*.ts`, listados em `desk.include` no `cerne.app.ts`) rodam em todo o desk: máscaras, atalhos, `defineForm` para vários DocTypes.
Inputs de campos Data têm `data-fieldname` e `data-fieldtype` para máscaras por delegação de eventos.
