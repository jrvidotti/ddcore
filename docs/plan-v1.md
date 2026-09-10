# ddcore — port do Frappe Framework para Go + TypeScript + PostgreSQL

## Contexto

O POC `../rent-frappe` (controle de aluguéis, Frappe v16) mostrou o valor do modelo
DocType/Document/desk, mas também as dores: stack pesada (Python + Node + MariaDB +
Redis + bench, tudo em Docker), meta duplicada (JSON + banco), regras enforçadas só no
cliente (`mandatory_depends_on`), sidebar inferida, tradução espalhada, e um formato de
DocType ruidoso para agentes de código editarem.

**Objetivo:** um framework moderno e rápido, feito para ser desenvolvido por
ferramentas agênticas: binário único em Go, apps em TypeScript tipado, Postgres como
único serviço externo, MCP para desenvolver DocTypes/regras/dados.

**Critério de pronto da v1:** um app externo roda ponta a ponta no novo framework
(DocTypes, regras, relatórios, workspace, scheduler e testes). O app `alugueis`, usado
na validação original, vive agora em repositório independente.

## Decisões fechadas

| Tema | Decisão |
|---|---|
| Nome | `ddcore` — bin `ddcore`, Go `github.com/jrvidotti/ddcore`, npm `@ddcore/*` |
| Linguagens | Core em Go; lógica de app em TS executada no **goja** embutido; desk em TS |
| Meta | `defineDoctype` em TS tipado; arquivo → memória (sem meta no banco) |
| Build | **esbuild embutido** no binário transpila TS de apps (server e form scripts); desk pré-compilado e embutido via `embed.FS` |
| Banco | PostgreSQL (pgx); uma tabela por DocType; sem Redis (fila e cache no Postgres/memória) |
| Tenancy | Um tenant por instância |
| Desk | Svelte 5 + SvelteKit (adapter-static, SPA) |
| Auth v1 | usuário/senha (argon2id) + cookie de sessão; `Authorization: token key:secret` |
| Repos | Framework em `/Users/junior/dev/ddcore`; apps de produto em repositórios independentes |

## 1. Layout do monorepo

```
ddcore/
  go.mod                        module github.com/jrvidotti/ddcore
  cmd/ddcore/                    main + subcomandos
  internal/meta/                modelo de DocType, loader (esbuild→goja→JSON), validação de meta
  internal/db/                  pgx pool, tx por request, filtros→SQL, schema diff/migrate
  internal/doc/                 Document: lifecycle, naming, links, fetch_from, child tables, versions
  internal/perm/                roles × doctype × ação, ifOwner, hooks de permissão
  internal/js/                  pool goja, bridge `ddcore.*`, module loader, eval
  internal/api/                 router (chi), REST/RPC, auth, SSE, assets, /mcp
  internal/jobs/                fila `ddcore_job` + scheduler cron
  internal/report/              executor de reports
  internal/i18n/                catálogos + `_()`
  internal/mcp/                 servidor MCP (stdio e HTTP)
  core/                         app "core" em TS embutido (User, Role, Has Role, File, Comment, Version, Error Log)
  desk/                         SvelteKit → build embutido
  packages/sdk/                 @ddcore/sdk: defineDoctype/defineController/defineReport/defineWorkspace/defineApp + tipos do bridge + test API
  packages/desk-sdk/            @ddcore/desk-sdk: defineForm, frm.*, Dialog, format
  apps/                         reservado para um futuro app de exemplo
  docs/                         referência do framework (escrita para agentes; cada API com exemplo)
  docs/superpowers/specs/       este design, copiado como 2026-09-09-ddcore-design.md
```

Layout de um app:

```
alugueis/                       # repositório standalone; apps: ["."]
  ddcore.app.ts                  defineApp({ name, title, requires, docEvents, scheduler, afterInstall, desk: { include } })
  doctypes/contrato/
    contrato.doctype.ts         meta
    contrato.controller.ts      hooks + métodos whitelisted (goja)
    contrato.form.ts            script do desk (browser)
    contrato.test.ts
  reports/*.report.ts
  workspaces/*.workspace.ts
  services/*.ts                 cobranca.ts, faturamento.ts, reajuste.ts, api.ts
  client/*.ts                   mascaras.ts, endereco.ts (bundle do desk)
  patches/0001_*.ts
  translations/pt-BR.csv
  .ddcore/types.d.ts             gerado por `ddcore types`
  CLAUDE.md                     gerado por `ddcore new-app`
```

## 2. Meta e mapeamento no Postgres

`defineDoctype({ name, module, naming, submittable, trackChanges, allowRename, isChild, fields, permissions, listSettings })`.

- **Fieldtypes v1**: Data, Email, Small Text, Text, Text Editor, Int, Float, Currency, Percent, Check,
  Date, Datetime, Time, Select, Link, Dynamic Link, Table, Attach, JSON, Section Break,
  Column Break, Tab Break, HTML.
- **Propriedades**: `label, reqd, unique, default, readOnly, hidden, options, fetchFrom,
  dependsOn, readOnlyDependsOn, mandatoryDependsOn, allowOnSubmit, inListView,
  inStandardFilter, searchIndex, length, precision, description, columns (largura no grid)`.
- `dependsOn`/`readOnlyDependsOn`/`mandatoryDependsOn` são expressões JS sobre `doc`,
  avaliadas no desk **e no servidor** (goja) — `mandatoryDependsOn` é enforçado no save.
- **Tabelas**: `tab_<snake_case(name)>`. Colunas padrão: `name text PK, owner text,
  creation timestamptz, modified timestamptz, modified_by text, docstatus smallint`.
  Filhas: `+ parent, parenttype, parentfield, idx` com índice `(parent, parentfield, idx)`.
- **Tipos**: Data/Select/Link/Dynamic Link/Attach → `text`; Small/Text/Text Editor → `text`;
  Int → `bigint`; Float → `double precision`; Currency/Percent → `numeric(21,9)`;
  Check → `boolean`; Date → `date`; Datetime → `timestamptz`; Time → `time`; JSON → `jsonb`.
- **Índices**: `unique`, `searchIndex`, todo Link; `modified` sempre.
- **Integridade de Link/Dynamic Link** é do core (valida no save, bloqueia delete com
  `LinkExistsError`, cascade no rename) — sem FK, para tratar Link e Dynamic Link igual.
- **Naming**: `{ series: "CTR-.YYYY.-.####" }` (contador em `ddcore_series` com `FOR UPDATE`),
  `{ field: "codigo" }`, `{ hash: true }`, `{ prompt: true }`, `{ format: "..." }`.
- **`ddcore migrate`**: meta → diff com `information_schema` → DDL (create table, add column,
  alter type `USING`, índices). Nunca dropa sem `--prune`; `--dry-run` imprime SQL;
  registra em `ddcore_migration`; roda patches pendentes depois do DDL.
- **Tabelas internas** (prefixo `ddcore_`, fora do modelo DocType): `session`, `series`,
  `job`, `patch`, `migration`, `api_key`.

## 3. Document, controllers e bridge Go↔TS

**Lifecycle** (Go, ordem igual ao Frappe): `beforeValidate → validate → beforeSave →
insert/update → afterInsert/onUpdate`; `beforeSubmit → onSubmit`; `beforeCancel →
onCancel`; `onTrash → afterDelete`; `beforeRename → afterRename`. Regras do core:
reqd, unique, opções de Select, links existentes, `fetchFrom`, child tables (idx,
parent), campos sem `allowOnSubmit` imutáveis com `docstatus=1`, transições de
docstatus, concorrência otimista por `modified`, Version com diff quando `trackChanges`.

**Controller** (TS, síncrono — o bridge chama Go direto, sem `await`):

```ts
export default defineController<Contrato>({
  validate(doc, ctx) { ... },
  onSubmit(doc, ctx) { ... },
  methods: { registrarBaixa: whitelisted((doc, args) => { ... }) },
  hasPermission(doc, ptype, user) { ... },          // opcional
  permissionQuery(user) { return [["owner","=",user]] } // opcional
});
```

**Bridge `ddcore.*`** (global no goja, implementado em Go):
`db.getValue/getList/setValue/count/exists/sql(readonly)`, `getDoc/newDoc/deleteDoc`,
`doc.insert/save/submit/cancel/dbSet/append/reload/getDoc(child)`, `throw(msg,{title})`,
`_()`, `session.user/roles`, `hasPermission`, `cache.get/set/del`, `http.get/post`,
`enqueue`, `log.*`, `utils.flt/cint/getdate/addMonths/addDays/getFirstDay/getLastDay/
nowdate/now/formatCurrency/formatDate`, `msgprint`.

**Runtime**: pool de `goja.Runtime` (não são goroutine-safe); cada um carrega o bundle
das apps uma vez; por request: acquire → `ctx` (user, tx, locale) → hooks → release.
Hot-reload no `ddcore dev` recria o pool. Erros: `ValidationError` (417), `PermissionError`
(403), `DoesNotExist` (404), `LinkExists` (417), `TimestampMismatch` (409) — payload
`{ error: { type, title, message } }`.

**Transação**: uma por request/job, aberta pelo core; erro → rollback. Não existe
`commit()` explícito para o app.

**Permissões v1**: `permissions: [{ role, read, write, create, delete, submit, cancel,
amend, report, export, ifOwner }]`; `Administrator` passa tudo; `Guest` só o que for
explícito. Perm level por campo e User Permissions → v2.

## 4. API HTTP

- `GET/POST /api/resource/:doctype` — `filters` (JSON: `[[f,op,v]]` ou `{f:v}`), `fields`,
  `order_by`, `limit`, `start`; ops: `= != > >= < <= like not like in not in between is`
  e `["Child DocType","campo",op,v]`.
- `GET/PUT/DELETE /api/resource/:doctype/:name`; `POST .../:name/:method` (submit, cancel,
  amend, rename, whitelisted).
- `POST /api/method/:app.:module.:fn` — funções exportadas com `whitelisted(fn, {allowGuest})`.
- `GET /api/meta/:doctype` (ETag), `GET /api/boot`, `GET /api/search/link`,
  `GET /api/report/:name?filters=`, `GET /api/translations?lang=`, `POST /api/upload`,
  `GET /api/events` (SSE: `doc_update`, `list_update`, `progress`, `job_done`).
- Auth: `POST /api/login|logout`; cookie `sid` + CSRF header; `Authorization: token key:secret`.

## 5. Desk (Svelte 5 + SvelteKit SPA)

- Rotas: `/login`, `/app`, `/app/:doctype`, `/app/:doctype/new`, `/app/:doctype/:name`,
  `/app/workspace/:name`, `/app/report/:name`.
- List view da meta: colunas `inListView`, filtros `inStandardFilter` + filtro avançado,
  ordenação, paginação, seleção e ações em massa, indicador de docstatus/status.
- Form view da meta: seções/colunas/tabs; um control por fieldtype (Link com busca e
  link para abrir; Date/Datetime pt-BR; Currency com máscara BRL; Table como grid
  editável + dialog de linha, `cannotAddRows` via `frm.setDfProperty`); Salvar/Enviar/
  Cancelar/Emendar; sidebar com comentários, versões, criado/modificado.
- **Form scripts** (`*.form.ts`, `@ddcore/desk-sdk`): compilados pelo esbuild embutido,
  servidos como ESM em `/assets/apps/:app/forms/:doctype.js`, importados dinamicamente.
  API: `defineForm("Lancamento", { setup, refresh, onChange: { campo(frm) } })`;
  `frm.doc/setValue/getValue/setQuery/setDfProperty/addButton/setPrimaryAction/call/
  reload/isNew/addIndicator`; `ddcore.ui.Dialog({ title, fields, primaryAction })`,
  `ddcore.ui.msgprint/alert/confirm`; `ddcore.format.currency/date`; `ddcore.call`.
- **Bundle de app**: `desk.include` em `ddcore.app.ts` → `/assets/apps/:app/desk.js`
  carregado em todo o desk (máscaras, ViaCEP via `/api/method/...`).
- Reports: `defineReport({ name, filters, execute(filters, ctx) → { columns, rows } })`
  (goja); desk renderiza filtros + tabela com totais + export CSV.
- Workspace: `defineWorkspace({ name, label, icon, shortcuts, numberCards, charts,
  reports })`; sidebar sempre explícita. Cards: `{ label, doctype, filters }` ou
  `{ label, method }`; charts: `{ label, method, type: "bar"|"line" }`.
- i18n: `/api/translations` mescla catálogo do core + CSV das apps (app vence).

## 6. Jobs, scheduler, patches, testes

- `ddcore.enqueue("minha_app.services.tarefas.executar", args)` → `ddcore_job`;
  workers em goroutines (`--workers N`) com `FOR UPDATE SKIP LOCKED`; retry, timeout,
  log de erro em Error Log.
- Scheduler em `ddcore.app.ts`: `scheduler: { cron: { "0 3 1 * *": [fn] }, daily: [fn],
  hourly, weekly, monthly }`; `scheduler.enabled` na config; `ddcore jobs run <fn>`.
- Patches: `patches/NNNN_nome.ts` exporta `execute(ctx)`; ordem lexicográfica; `ddcore_patch`.
- `afterInstall(ctx)` em `ddcore.app.ts` para roles/dados iniciais.
- **Testes de app**: `ddcore test [--filter]` executa `*.test.ts` no goja com `describe/it/
  expect/beforeEach` e helper `makeDoc`; cada `it` em transação revertida; sem servidor HTTP.
- **Testes do core**: Go, contra Postgres de teste (`DDCORE_TEST_DSN`), incluindo round-trip
  meta → DDL → introspecção e a semântica de docstatus/allowOnSubmit.

## 7. CLI e MCP

**CLI**: `init`, `new-app`, `dev` (servidor + watcher + hot-reload; `--auto-migrate`),
`migrate [--dry-run|--prune]`, `types`, `test`, `exec <fn> --args`, `eval '<ts>'`,
`jobs run <fn>`, `user add`, `demo`, `mcp`, `docs`.

**MCP** (`ddcore mcp` stdio; `/mcp` HTTP no `dev` com API key):

| Grupo | Tools |
|---|---|
| Meta | `list_doctypes`, `get_doctype`, `scaffold_doctype`, `validate_meta`, `migrate(dry_run)`, `generate_types` |
| Dados | `get_doc`, `list_docs`, `insert_doc`, `update_doc`, `delete_doc`, `submit_doc`, `cancel_doc`, `call_method`, `sql_query` (readonly) |
| Dev | `eval` (tx revertida por padrão), `run_tests`, `get_logs`, `reload` |
| Resources | `ddcore://docs/{fieldtypes,controller-api,form-api,report-api,conventions}`, `ddcore://meta/{doctype}` |

Erros das tools dizem o que fazer (ex.: "coluna pendente; rode migrate").
`ddcore new-app` gera `CLAUDE.md` do app apontando para `docs/`.

## 8. Histórico de implementação da v1

Historicamente, cada fase terminou com testes passando e parte do `alugueis` rodando. Ao aprovar este
plano: (a) copiar este documento para `docs/superpowers/specs/2026-09-09-ddcore-design.md`,
`git init` + commit; (b) invocar `superpowers:writing-plans` para o plano detalhado da
fase 0+1.

**Fase 0 — Spike goja/esbuild (throwaway).** Verificar: async/await e classes no goja;
transpilar TS em memória com `github.com/evanw/esbuild/pkg/api`; pool de runtimes;
custo de 10k chamadas ao bridge. Fallback: `target: es2016` no esbuild.

**Fase 1 — Kernel.** `internal/meta` (modelo + loader + validação), `internal/db`
(pool, tx, filtros→SQL, migrate), `internal/doc` (CRUD, naming, links, child tables,
fetchFrom, versions), `internal/perm`, `internal/api` (REST, auth, boot, meta), app
`core` (User, Role, Has Role). Valida com `Pessoa`, `Imovel`, `Caracteristica`,
`Caracteristica Imovel` via REST.

**Fase 2 — Runtime TS.** `internal/js` (pool, bridge, module loader), controllers,
whitelisted, submit/cancel/amend, `@ddcore/sdk`, `ddcore types`, `ddcore test`, `ddcore eval`.
Porta `Contrato`, `Lancamento`, `Baixa Lancamento`, `Indice`, `services/{cobranca,
faturamento,reajuste}` e **traduz os testes do POC** (`tests/test_*.py` → `*.test.ts`).

**Fase 3 — Desk.** Login, boot, list view, form view com todos os controls e grid,
Link search, form scripts + Dialog, bundle de app (máscaras, ViaCEP), SSE.

**Fase 4 — Reports, workspace, cards/charts, jobs/scheduler, patches, i18n.**

**Fase 5 — CLI completo, MCP, docs, `ddcore demo`, port ponta a ponta do alugueis.**

Depois da validação da v1, o app foi extraído para `/Users/junior/dev/alugueis`. O
repositório do framework usa apps temporários nos testes de aceitação; um app de exemplo
rastreado ficou explicitamente adiado.

## Verificação histórica da v1

Os itens abaixo foram executados quando `alugueis` ainda integrava o monorepo. Depois da
extração, a suíte do DDCore valida os mesmos limites do framework com apps temporários,
enquanto a suíte de domínio roda no repositório standalone.

1. `ddcore dev` num checkout limpo + Postgres vazio: `ddcore init && ddcore migrate` cria
   todas as tabelas do core e do alugueis; `ddcore migrate --dry-run` em seguida é vazio.
2. `ddcore test` passa com os testes portados do POC (contrato, lancamento, cobranca,
   faturamento, reajuste, documentos, api, traducao).
3. Desk: login → `/app` cai no workspace Alugueis (sidebar explícita) → criar Pessoa com
   máscara de CPF e ViaCEP → criar Imóvel → Contrato (submit muda Imóvel para Alugado)
   → *Gerar competência* → Lançamento → *Registrar pagamento* pelo dialog → relatórios
   e cards refletem.
4. MCP: com `ddcore mcp`, um agente consegue `scaffold_doctype` → `migrate` → `insert_doc`
   → `run_tests` sem tocar no terminal.
5. Referência de mordidas do Frappe documentadas no CLAUDE.md do POC que **não podem
   se repetir**: `mandatoryDependsOn` no servidor; sidebar nunca inferida; idioma em um
   só lugar; `set_df_property` redesenha o grid; scheduler desabilitado é visível no boot.

## Riscos

- goja: cobertura ES/perf → fase 0. Sem npm nativo: libs puras só (aceitável para regras).
- Desk foi a maior fatia da v1 → gerar tudo da meta e limitar inicialmente os fieldtypes ao app de validação.
- Semântica de docstatus/allowOnSubmit/dbSet → coberta pelos testes portados na fase 2.
