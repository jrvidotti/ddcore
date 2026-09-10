# cerne — referência para agentes

cerne é um framework de aplicações no espírito do Frappe: **DocTypes** (modelos declarados
em TypeScript) viram tabelas Postgres, formulários, listas e API REST automaticamente.
O core é um binário Go (`cerne`) que embute esbuild + goja: a lógica dos apps é TypeScript
executado **no servidor, de forma síncrona** (sem `await`), e os scripts de formulário rodam no desk.

Documentos disponíveis (também como resources MCP `cerne://docs/<nome>`):

- `conventions` — layout de um app, nomes, o que nunca fazer
- `fieldtypes` — todos os fieldtypes e propriedades de campo
- `controller-api` — `defineController`, hooks, métodos, a API `cerne.*` do servidor
- `form-api` — `defineForm`, `frm.*`, dialogs (desk)
- `report-api` — `defineReport`, `defineWorkspace`, cards e charts
- `cli` — comandos `cerne` e o fluxo de desenvolvimento

## Fluxo típico

1. Em um monorepo, `cerne new-app <nome>` cria `apps/<nome>`; em um repositório do
   próprio app, use `cerne new-app <nome> --dir .` e `apps: ["."]`.
2. Escreva `doctypes/<snake>/<snake>.doctype.ts` com `defineDoctype`.
3. `cerne migrate` (ou a tool MCP `migrate`) cria/altera as tabelas e gera em `.cerne/`
   os tipos dos DocTypes e as declarações dos SDKs embutidos.
4. Regras em `<snake>.controller.ts`; scripts de tela em `<snake>.form.ts`; testes em `<snake>.test.ts`.
5. `cerne dev` sobe o servidor em hot-reload; `cerne test` roda os testes em transações revertidas.

## Modelo mental

- Um DocType = uma tabela `tab_<snake_case>`; tabelas filhas (`isChild`) têm `parent`, `parenttype`, `parentfield`, `idx`.
- Colunas padrão: `name` (PK, texto), `owner`, `creation`, `modified`, `modified_by`, `docstatus` (0 rascunho, 1 enviado, 2 cancelado).
- Ciclo de vida: `beforeValidate → validate → beforeSave → (insert|update) → afterInsert/onUpdate`; `beforeSubmit → onSubmit`; `beforeCancel → onCancel`; `onTrash → afterDelete`.
- O core valida `reqd`, `unique`, `options` de Select, links existentes, `fetchFrom`, `mandatoryDependsOn` (**no servidor**) e bloqueia alterar campos sem `allowOnSubmit` depois de enviado.
- Uma transação por request/job. Erro → rollback. Não existe `commit()` para o app.
