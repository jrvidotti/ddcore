# Relatórios, workspaces, cards e charts

```ts
import { defineReport, _ } from "@cerne/sdk";
export default defineReport({
  name: "Contratos a Vencer", refDoctype: "Contrato", roles: ["Gestor"],
  filters: [{ fieldname: "dias", label: "Dias", fieldtype: "Int", default: 90, reqd: true }, { fieldname: "imovel", fieldtype: "Link", options: "Imovel", label: "Imóvel" }],
  execute(filters, ctx) {
    const rows = cerne.db.getList("Contrato", { filters: {...}, fields: [...], limit: 10000 });
    return {
      columns: [{ fieldname: "name", label: _("Contrato"), fieldtype: "Link", options: "Contrato", width: 140 }, ...],
      rows,
      summary: [{ label: _("Total"), value: 10, datatype: "Currency", indicator: "red" }],
      chart: { type: "bar", labels: [...], datasets: [{ name: "Receitas", values: [...] }] },
    };
  },
});
```
Defaults de filtro Date: `"Today"`, `"month_start"`, `"month_end"`, `"-11m"` (início do mês, N meses atrás).
Rota: `/app/report/<nome>`; API: `GET /api/report/<nome>?filters={...}`.

```ts
import { defineWorkspace } from "@cerne/sdk";
export default defineWorkspace({
  name: "Comercial", label: "Comercial", icon: "briefcase", roles: ["Gestor"],
  sidebar: [{ label: "Visão Geral", route: "/app/workspace/Comercial", icon: "layout-dashboard" }, { label: "Contratos", doctype: "Contrato", icon: "notepad-text" }, { label: "Cadastros" /* sem link = cabeçalho */ }, { label: "Relatório X", report: "Relatório X" }],
  shortcuts: [{ label: "Contratos", doctype: "Contrato", icon: "notepad-text" }],
  numberCards: [
    { name: "vigentes", label: "Contratos Vigentes", doctype: "Contrato", filters: { situacao: "Vigente" }, color: "green", route: "/app/Contrato?situacao=Vigente" },
    { name: "atraso", label: "Total em Atraso", doctype: "Lancamento", filters: { status: "Atrasado" }, aggregate: "sum:saldo_devedor", color: "red" },
    { name: "mes", label: "Recebido no Mês", method() { return { value: 10, formatted: "R$ 10,00" }; } },
  ],
  charts: [{ name: "receita", label: "Receita Mensal", type: "bar", method() { return { type: "bar", labels: [...], datasets: [{ name: "Receita", values: [...] }] }; } }],
  links: [{ label: "Cadastros", items: [{ label: "Pessoas", doctype: "Pessoa" }] }],
});
```
Ícones: subconjunto lucide (`building-2, users, user, notepad-text, receipt, list, bar-chart-3, layout-dashboard, shield, paperclip, message-square, house, tag, history, settings`).
`desk.home` no `cerne.app.ts` define o workspace inicial.
