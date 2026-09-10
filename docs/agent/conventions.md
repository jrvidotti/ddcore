# Convenções

## Layout de um app

```
<app>/                         # raiz do repositório, ou apps/<app>/ em monorepo
  cerne.app.ts                  defineApp: título, roles, scheduler, docEvents, desk.include, afterInstall, fixtures
  doctypes/<snake>/
    <snake>.doctype.ts          defineDoctype (meta)
    <snake>.controller.ts       defineController (regras, métodos)
    <snake>.form.ts             defineForm (desk) — importa "@cerne/desk-sdk"
    <snake>.test.ts             testes (importa "@cerne/sdk/test")
  services/*.ts                 funções de negócio; whitelisted() expõe em /api/method/<app>.services.<arquivo>.<fn>
  reports/*.report.ts           defineReport
  workspaces/*.workspace.ts     defineWorkspace
  patches/NNNN_nome.ts          export function execute(ctx) — roda uma vez no migrate
  client/*.ts                   scripts carregados em todo o desk (declare em desk.include)
  translations/pt-BR.csv        "texto original,tradução" (sem cabeçalho)
  .cerne/types.d.ts             gerado por `cerne types` — importe `import type { Contrato } from "../../.cerne/types"`
```

Caminho pontilhado de um módulo: `<app>.<pasta>.<arquivo>` (sem `.ts`). Ex.: `minha_app.services.tarefas.executar`.
É o formato usado por `whitelisted`, `scheduler`, `cerne.enqueue`, `cerne exec` e a tool `call_method`.

## Nomes

- Nome de DocType: ASCII, com espaços, capitalizado (`"Baixa Lancamento"`). Vira tabela `tab_baixa_lancamento` e interface `BaixaLancamento`.
- `fieldname`: snake_case ASCII (`data_vencimento`). Labels podem ter acento.
- Reservados: `name, owner, creation, modified, modified_by, docstatus, doctype, parent, parenttype, parentfield, idx`.

## Regras que não mudam

- Código de servidor é síncrono. `cerne.db.getValue(...)` devolve o valor direto; nunca use `await` em controllers/serviços.
- Erros de negócio: `cerne.throw(_("mensagem"), { title: _("Título") })` — vira HTTP 417 `ValidationError` e um toast no desk.
- Depois de enviado (`docstatus 1`), só campos com `allowOnSubmit` mudam via `save()`; o resto vai por `doc.dbSet(...)`.
- Status derivado (ex.: pago/atrasado) é calculado no `validate`, nunca digitado.
- Sidebar e workspace são explícitos (`defineWorkspace`), nunca inferidos.
- Nunca edite `.cerne/types.d.ts`; rode `cerne types`.
- Testes: cada `it` roda numa transação revertida; crie os dados que precisa dentro do teste.
