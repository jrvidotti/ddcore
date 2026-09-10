# Desenvolvimento: DDCore + app `apps/exemplo`

Este documento explica **como se desenvolve** neste repositório: o que é o DDCore, o que é o
app de exemplo, onde fica a fronteira entre o núcleo e o app, quais arquivos existem para
quê e qual é o ciclo de trabalho do dia a dia.

Aqui há uma particularidade que não existe em um app de produto: este checkout é o
**monorepo do framework**. O binário não vem instalado — é compilado daqui (`bin/ddcore`) —
e `apps/exemplo` é carregado por este mesmo `ddcore.json`. Ou seja, quase sempre há **dois
loops de trabalho** acontecendo: o do núcleo (Go + desk) e o do app (TypeScript).

Complementos:

- `README.md` — visão geral, setup e layout do monorepo.
- `CLAUDE.md` / `AGENTS.md` — regras curtas e obrigatórias (síncrono, portas, `.ddcore/`).
- `docs/plan-v1.md` — o design do framework.
- `docs/agent/` (`ddcore docs`, ou os resources MCP `ddcore://docs/*`) — **a referência
  canônica da API**: `conventions`, `fieldtypes`, `controller-api`, `form-api`,
  `report-api`, `cli`. Este documento descreve o modelo mental, não substitui a API.
- `docs/superpowers/specs/2026-09-10-app-exemplo-design.md` — a especificação do app
  exemplo, que é o alvo do que está sendo construído em `apps/exemplo`.

---

## 1. O que é o DDCore

O DDCore (*Data Driven Core*) é um **framework de aplicações de dados** no espírito do Frappe, implementado em
**Go + TypeScript + PostgreSQL** e distribuído como um único binário (`ddcore`). Ele não é
uma biblioteca que o app importa e compila junto: é um **host**. O binário embute o
esbuild e o goja, o servidor HTTP, a camada de banco, o desk (SPA Svelte 5) e as tipagens
dos SDKs.

O app é um diretório de arquivos TypeScript que o binário carrega e executa. Daí a
consequência prática mais importante:

> **O app não tem build próprio, não tem servidor próprio e não depende de Node em
> produção.** O único "motor" é o binário `ddcore`. A única dependência externa é um
> Postgres.

O código de servidor do app roda dentro do intérprete **goja**, que é **síncrono**: não
existe event loop, não existe `Promise` útil, não existe `await`. I/O (banco, HTTP, cache)
é exposto como chamada síncrona que bloqueia e devolve o valor. É uma escolha de design do
framework — e a razão da regra "nunca use `await` no servidor".

### Os três SDKs

| Módulo | Onde roda | O que é |
|---|---|---|
| `@ddcore/sdk` | servidor (goja), **síncrono** | `defineDoctype`, `defineController`, `defineReport`, `defineWorkspace`, `defineApp`, `whitelisted`, e o global `ddcore` (db, utils, http, cache, log…) |
| `@ddcore/sdk/test` | `ddcore test` | `describe`/`it`/`expect` |
| `@ddcore/desk-sdk` | navegador, **assíncrono** | `defineForm`, `defineListView`, `ddcore.ui.Dialog`, `ddcore.db.*` (por HTTP), `ddcore.format`, `ddcore.datetime` |

**Atenção a uma diferença deste repositório.** Num app externo, os três são *materializados*
por `ddcore types` dentro de `.ddcore/` e o `tsconfig.json` aponta para lá. Aqui, como o
código-fonte dos SDKs está no próprio checkout, `apps/exemplo/tsconfig.json` resolve
`@ddcore/sdk`, `@ddcore/sdk/test` e `@ddcore/desk-sdk` direto em `packages/` — o typecheck do app valida contra a
**fonte** dos SDKs, não contra uma cópia gerada. Mudar `packages/sdk` quebra (ou conserta)
o typecheck do app na mesma hora, o que é exatamente o efeito desejado.

O que `./bin/ddcore types` ainda gera em `apps/exemplo/.ddcore/` são os **tipos dos DocTypes**
(`types.d.ts`), a entrada do desk e as declarações embutidas. `.ddcore/` é ignorado pelo git
e **nunca** deve ser editado à mão.

---

## 2. Divisão de responsabilidades

A regra geral: **o núcleo resolve o que é genérico para qualquer app de dados; o app
descreve e decide o que é específico do domínio.** No monorepo, cada responsabilidade do
núcleo tem um diretório com nome próprio.

### O que o DDCore (núcleo) faz — e onde mora

| Responsabilidade | Onde |
|---|---|
| Meta-modelo: interpreta os DocTypes e valida a meta | `internal/meta` |
| DDL: cria/altera `tab_<snake>`, colunas, índices; migrate, patches, fixtures | `internal/db` |
| Executa o TypeScript do app (esbuild + goja), hot-reload | `internal/js` |
| `Document` (insert/save/submit/cancel/delete), hooks, permissões, jobs e filas | `internal/engine` |
| API REST, `/api/method`, meta, relatórios, SSE, upload, login/CSRF | `internal/api` |
| Servidor MCP (tools e resources) | `internal/mcp` |
| Scaffold (`ddcore init`, `new-app`, `scaffold_doctype`) | `internal/scaffold` |
| Geração de tipos (`ddcore types`) | `internal/typegen` |
| App embutido: User, Role, Has Role, File, Comment, Version, Error Log, API Key | `core/` |
| Desk: listas, formulários, grids, filtros, workspaces, relatórios, diálogos, i18n | `desk/` |
| Os SDKs que o app importa | `packages/sdk`, `packages/desk-sdk` |
| CLI | `cmd/ddcore` |

Em termos de comportamento, é o núcleo que garante:

- **Ciclo de vida** — `beforeValidate → validate → beforeSave → (insert|update) →
  afterInsert/onUpdate`; `beforeSubmit → onSubmit`; `beforeCancel → onCancel`;
  `onTrash → afterDelete`. `docstatus` 0/1/2, nomeação, `amended_from`, tabelas filhas
  (`parent`/`parenttype`/`parentfield`/`idx`).
- **Validações estruturais, no servidor** — `reqd`, `unique`, `options` de Select,
  existência dos links, `fetchFrom`, `mandatoryDependsOn`, e o bloqueio de alterar campo
  sem `allowOnSubmit` depois de enviado.
- **Transações** — uma por request/job/`it` de teste. Erro → rollback. O app não tem
  `commit()`.
- **Permissões** — as `permissions` por papel, mais os ganchos `hasPermission` e
  `permissionQuery` do controller.
- **Desk inteiro** — o app **não escreve componentes de UI**; descreve campos e, quando
  precisa, ajusta comportamento por script.
- **Agendador e filas** — executa o `scheduler` declarado pelo app e `ddcore.enqueue`.
- **Ferramental** — `dev`, `migrate`, `test`, `types`, `exec`, `eval`, `demo`, `jobs`,
  `user`, `apikey`, `doctor`, `docs`, `mcp`.

### O que o app (`apps/exemplo`) faz

O exemplo é deliberadamente pequeno: projetos, marcos e tarefas. Ele existe para ser
**tutorial executável, fixture de ponta a ponta e referência de boas práticas** — não para
carregar regras de um produto real. Dentro desse escopo, ele:

- **declara o domínio**: DocTypes `Projeto`, `Tarefa` e `Marco Projeto` (filho), com papéis
  `Gestor de Projetos` e `Colaborador de Projetos`;
- **escreve as regras** nos controllers e nos serviços: validação de intervalo de datas,
  transições idempotentes `iniciar`/`concluir`/`reabrir`, recálculo de progresso do projeto,
  marcação diária de tarefas atrasadas;
- **declara navegação e leitura**: workspace `Projetos` (sidebar, atalhos, cards, gráfico) e
  o relatório `Tarefas por Status`;
- **ajusta a UI pelas bordas**: `*.form.ts` (botões, indicadores, filtro do Link) e
  `client/listas.ts` (`defineListView` da lista de Tarefa);
- **declara a rotina periódica** em `ddcore.app.ts` (`daily:
  exemplo.services.tarefas.marcarAtrasadas`) — o núcleo é quem a executa;
- **semeia demonstração** em `services/demo.ts`, idempotente, descoberta por `ddcore demo`;
- **cobre tudo com testes** `*.test.ts`.

Por ser exemplo, ele também **evita de propósito** coisas que o framework suporta:
integrações externas, anexos, `hasPermission`/`permissionQuery`, SQL direto e patches sem
uma migração real a demonstrar. Quando você precisa de um caso mais pesado, ele está
coberto pelos testes do núcleo — não force o exemplo a crescer.

### A fronteira, em uma linha por caso

| Precisa de… | Quem resolve |
|---|---|
| uma coluna nova no banco | núcleo — basta declarar o campo no `.doctype.ts` |
| `concluido_em` obrigatório quando `concluido` | núcleo, via `mandatoryDependsOn` (desk **e** servidor) |
| impedir data limite antes do início do projeto | app — `validate` do controller |
| botão "Concluir" no formulário | app declara (`frm.addButton`), núcleo renderiza |
| marcar atrasadas todo dia | app declara no `scheduler`, núcleo executa |
| gravar campo derivado sem recursão de hooks | núcleo fornece `dbSet`, app decide quando |
| formatar percentual na listagem | núcleo, a partir do `fieldtype: "Percent"` |
| decidir que progresso = concluídas/total | app — `services/projetos.ts` |

---

## 3. Anatomia do repositório

```
cmd/ddcore/                    CLI
internal/                     meta · db · js · engine · api · mcp · scaffold · typegen
core/                         app embutido (User, Role, File, Version, …)
desk/                         SvelteKit, build embutido no binário
packages/sdk/                 @ddcore/sdk
packages/desk-sdk/            @ddcore/desk-sdk
docs/agent/                   referência da API, escrita para agentes
ddcore.json                    instância de dev: dsn :5455, porta 8090, apps: ["apps/exemplo"]
Makefile                      atalhos do fluxo de trabalho
bin/ddcore                     binário compilado (gerado por make build)
apps/exemplo/                 o app de exemplo
```

### Dentro de `apps/exemplo`

```
ddcore.app.ts                    manifesto: nome, papéis, scheduler, desk
tsconfig.json                   resolve @ddcore/sdk em packages/sdk/src
doctypes/<nome>/
  <nome>.doctype.ts             meta: campos, permissões, nomeação      [servidor, declarativo]
  <nome>.controller.ts          hooks de ciclo de vida e métodos        [servidor, síncrono]
  <nome>.form.ts                comportamento do formulário no desk     [navegador, async]
  <nome>.test.ts                testes                                  [ddcore test]
services/*.ts                   regras de negócio reutilizáveis         [servidor, síncrono]
client/listas.ts                scripts globais do desk                 [navegador, async]
reports/*.report.ts             relatórios                              [servidor, síncrono]
workspaces/*.workspace.ts       navegação e painel                      [servidor, declarativo]
translations/pt-BR.csv          traduções
.ddcore/                         GERADO por `ddcore types` — não editar
```

A convenção de nomes **é** o mecanismo de descoberta: o binário varre o diretório do app e
registra pelo sufixo (`.doctype.ts`, `.controller.ts`, `.form.ts`, `.report.ts`,
`.workspace.ts`, `.test.ts`). Não existe arquivo de índice para manter.

> O app está completo e coberto por `make test`; a especificação que o originou é
> `docs/superpowers/specs/2026-09-10-app-exemplo-design.md`.

### Os quatro arquivos de um DocType

Tomando `Projeto` como exemplo:

**1. `projeto.doctype.ts` — a meta.** Puramente declarativo. Campos com `fieldtype`
(`Data`, `Link`, `Select`, `Percent`, `Table`…), `reqd`, `unique`, `readOnly`,
`inListView`, `inStandardFilter`, `naming: { field: "codigo" }`, `titleField`,
`trackChanges`. Daqui o núcleo deriva: o schema no Postgres, o layout do formulário, as
colunas da listagem, os filtros e as tipagens em `.ddcore/types.d.ts`.

```ts
export default defineDoctype({
  name: "Projeto",
  module: "Projetos",
  naming: { field: "codigo" },
  titleField: "titulo",
  fields: [
    { fieldname: "codigo", fieldtype: "Data", label: "Código", reqd: true, unique: true, inListView: true },
    { fieldname: "responsavel", fieldtype: "Link", label: "Responsável", options: "User", reqd: true, inStandardFilter: true },
    { fieldname: "progresso", fieldtype: "Percent", label: "Progresso", readOnly: true },
    { fieldname: "marcos", fieldtype: "Table", label: "Marcos", options: "Marco Projeto", gridEditMode: "inline" },
    // ...
  ],
  permissions: [
    { role: "Gestor de Projetos", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "Colaborador de Projetos", read: true },
  ],
});
```

Repare em `status` e `progresso`: são `readOnly` porque são **derivados**. Campo derivado
nunca é digitado; quem o mantém é o serviço, via `dbSet`.

**2. `projeto.controller.ts` — as regras.** Síncrono. `validate` é o guarda do invariante e
roda em **toda** gravação, venha ela do desk, da API, de um job ou de um teste — é por isso
que a validação vive aqui e não no formulário. `methods` expõe ações chamáveis pelo desk
(`iniciar`, `concluir`, `reabrir` em Tarefa), que salvam pelo ciclo de vida normal e são
idempotentes. Use `ddcore.throw` com `title` para erro de negócio, `doc.dbSet` para gravar
coluna derivada sem reentrar no `validate`, e `doc.getDocBeforeSave()` quando o efeito
depende do valor anterior (mover uma tarefa de projeto recalcula **os dois**).

**3. `projeto.form.ts` — a UI.** Roda no navegador, **pode** ser assíncrono. Indicadores,
botões condicionais ao estado, `frm.setQuery` para restringir um Link, `frm.call(...)` para
invocar método do controller e recarregar. Aqui só mora conveniência: nada que o servidor
precise garantir deve depender deste arquivo.

**4. `projeto.test.ts` — a rede de segurança.** Cada `it` roda em transação revertida.

### Serviços: onde o domínio realmente mora

Um controller bom é fino. A lógica fica em `services/*.ts`, porque precisa ser chamada de
vários lugares com a mesma semântica: do `validate`, de um método de controller, de um
relatório, de um card do workspace, do scheduler, de `ddcore exec` e dos testes. No exemplo
isso aparece três vezes:

- `services/projetos.ts:recalcularProgresso` — chamado após inserir, atualizar ou excluir
  uma tarefa;
- `services/tarefas.ts:marcarAtrasadas` — chamado pelo scheduler diário **e**, pelo wrapper
  `marcarAtrasadasAgora` (`whitelisted`, restrito a `Gestor de Projetos`), por um botão do
  desk;
- `services/tarefas.ts:resumoPorStatus` — compartilhado pelo relatório **e** pelo gráfico do
  workspace, para que as contagens não possam divergir.

Funções marcadas com `whitelisted(...)` ganham um endpoint HTTP
(`POST /api/method/exemplo.services.tarefas.marcarAtrasadasAgora`) — é assim que o desk
chama o servidor fora do contexto de um documento.

Texto de interface é escrito como chave em inglês e traduzido em `translations/pt-BR.csv`
(`Start`, `Complete`, `Reopen`, `Open`, `In progress`, `Overdue`, `Completed`).

---

## 4. Ciclo de desenvolvimento

### Setup (uma vez)

```bash
make docker-up                               # Postgres de dev (container ddcore-pg, porta 5455)
make build                                   # desk (npm) + binário em bin/ddcore
make migrate                                 # DDL do core + instalação do app exemplo
./bin/ddcore user passwd Administrator admin
./bin/ddcore demo                             # dados de demonstração (idempotente)
make dev                                     # http://localhost:8090
```

Requisitos: Docker, Go e Node.js. Os testes Go usam o banco `ddcore_test` (recriado);
`DDCORE_DSN` e `DDCORE_TEST_DSN` sobrescrevem o DSN de qualquer comando.

### Os dois loops

**Mexendo no app (`apps/exemplo/**/*.ts`).** `make dev` fica rodando e recompila ao salvar:
não precisa reiniciar nem rebuildar. Se a mudança foi na **meta** de um DocType, rode
`./bin/ddcore migrate` (ou deixe o `--auto-migrate` do `dev` cuidar) e
`./bin/ddcore types` para regenerar as tipagens.

**Mexendo no núcleo (`internal/`, `core/`, `cmd/`, `packages/`, `desk/`).** Aí o binário
mudou: `make build` (ou `go build -o bin/ddcore ./cmd/ddcore`, quando o desk não mudou) e
reinicie o `dev`. Mudança em `packages/sdk` aparece no typecheck do app imediatamente,
porque o `tsconfig.json` do exemplo aponta para a fonte.

Em ambos os casos, quando a 8090 estiver ocupada use `make stop` em vez de subir um segundo
servidor.

### Comandos que importam

| Comando | Para quê |
|---|---|
| `make build` | desk + `bin/ddcore` |
| `make dev` / `make stop` | servidor em :8090 com hot-reload |
| `make migrate` | DDL + `afterInstall` + fixtures + patches + `afterMigrate` + types |
| `make check` | `svelte-check` do desk, `ddcore types` e `tsc` do `apps/exemplo` |
| `make test` | build + `go vet` + testes Go + `ddcore test --app exemplo` + testes do desk |
| `./bin/ddcore test --filter <regex> -v` | iterar em um teste do app |
| `./bin/ddcore demo` | semeia dados de demonstração (idempotente) |
| `./bin/ddcore exec exemplo.services.tarefas.marcarAtrasadas` | roda um serviço fora de requisição |
| `./bin/ddcore eval '<ts>' [--commit]` | TS avulso com `ddcore.*` (rollback por padrão) |
| `./bin/ddcore doctor` | banco, meta, DDL pendente, scheduler |
| `./bin/ddcore docs` / MCP `ddcore://docs/*` | a referência da API |
| `make docker-psql` | psql no `ddcore_dev` |

`make test` é o critério de pronto — inclui `internal/acceptance`, que valida instalação,
boot, tradução e `/app` contra Postgres e HTTP reais.

### Regras não negociáveis

- Servidor é **síncrono**: nenhum `await`/`Promise` em `*.controller.ts`, `services/`,
  `reports/`, `workspaces/`, `patches/`.
- Desk (`*.form.ts`, `client/*.ts`) **pode** ser assíncrono — e quase sempre é.
- **Nunca** editar `apps/exemplo/.ddcore/`; rodar `./bin/ddcore types`.
- Validação de invariante vive no servidor (`validate`), nunca só no formulário.
- Texto de interface passa por `_()` / `__()` e pelo CSV de tradução.
- Nenhum arquivo do app importa fonte por caminho relativo fora de `apps/exemplo`: os SDKs
  entram só como `@ddcore/sdk` e `@ddcore/desk-sdk`.
- O app exemplo não usa SQL direto; consultas vão por `ddcore.db.getList` e afins.
- Use as tools MCP em vez de mexer no banco à mão.
- `make test` antes de considerar qualquer coisa pronta.

---

## 5. Como ler um fluxo de ponta a ponta

"Concluir tarefa" atravessa todas as camadas e serve de mapa:

1. **Desk** — `doctypes/tarefa/tarefa.form.ts` mostra o botão `Complete` conforme o estado
   e chama `frm.call("concluir")`.
2. **Núcleo** — recebe `POST /api/resource/Tarefa/:name/concluir` (`internal/api`),
   autentica, checa permissão, abre a transação, carrega o documento (`internal/engine`) e
   invoca o método do controller no goja (`internal/js`).
3. **Controller** — `tarefa.controller.ts:methods.concluir` é idempotente: se já está
   concluída, devolve o estado atual; senão define `status = "Concluída"`,
   `concluida_em = ddcore.utils.now()` e salva pelo ciclo de vida normal.
4. **Hook + serviço** — o `onUpdate` da Tarefa chama
   `services/projetos.ts:recalcularProgresso(projeto)`, que conta tarefas concluídas e
   grava `progresso` e `status` do Projeto com `dbSet` (sem recursão de hooks).
5. **Núcleo** — sucesso → commit e `{ status, concluida_em }` na resposta; `ddcore.throw` →
   rollback e mensagem de negócio na tela.
6. **Desk** — recarrega o documento; o indicador e o progresso aparecem atualizados, e o
   card/gráfico do workspace refletem a mesma contagem porque leem o mesmo serviço.

O mesmo caminho é exercitado sem UI alguma por `doctypes/tarefa/tarefa.test.ts` e
`services/tarefas.test.ts`. Essa é a forma do framework: **o núcleo move o documento pelo
ciclo de vida e pela transação; o app diz o que é verdade sobre o domínio em cada ponto do
caminho.**
