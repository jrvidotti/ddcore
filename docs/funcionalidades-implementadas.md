# Funcionalidades implementadas no ddcore

Data de referência: 10/09/2026. Este documento inventaria somente capacidades
com implementação identificada no checkout atual. Para lacunas e prioridades de
evolução, consulte o [inventário de migração](inventario-migracao-frappe.md).

## Visão geral

O ddcore já executa apps de dados em TypeScript sobre Postgres: DocTypes se tornam
tabelas, formulários, listas e API; regras rodam no servidor; o Desk é gerado por
metadados; jobs, relatórios, workspaces, traduções, testes, CLI e MCP estão no binário.

## Metadados, banco e documentos

| Capacidade | Implementação disponível |
|---|---|
| DocTypes em TypeScript | `defineDoctype` declara campos, naming, permissões, ordenação, título, ícone e descrição; arquivos são transpilados com esbuild e carregados no goja. |
| Schema Postgres | `ddcore migrate` cria/altera tabelas `tab_<snake_case>`, colunas e índices; `--dry-run` relata o plano classificado e `--prune` remove estruturas não declaradas — só as vazias, recusando derrubar o que ainda tem dado. |
| Migrações de app | Patches ordenados em duas fases (`beforeSchema`/`afterSchema`), com `ctx.sql` para preenchimento em lote; fixtures, `afterInstall`, `afterMigrate` e validação/ordenação de `requires`. Tudo numa transação. |
| Renomes e conversões | `renamedFrom` em campo e em DocType renomeia coluna/tabela, índices e as referências guardadas; mudança de fieldtype que possa perder dado é recusada até declarar `convert`. |
| Fieldtypes | Data, Email, Small Text, Text, Text Editor, Int, Float, Currency, Percent, Check, Date, Month, Datetime, Time, Select, Link, Dynamic Link, Table, Attach, JSON, Password e campos de layout. |
| Filhos e naming | DocTypes `isChild`, `parent`/`parenttype`/`parentfield`/`idx`; séries, campo, hash, prompt e formato para nomes. |
| Validação no servidor | Obrigatoriedade, unicidade, Select, Email, Link/Dynamic Link, `fetchFrom`, dependências, `mandatoryDependsOn` e `allowOnSubmit`. |
| Document | Inserir, salvar, enviar, cancelar, emendar, renomear, apagar, recarregar, `append`, `dbSet`, mudança de campo e concorrência por `modified`. |
| Histórico e transações | `docstatus`, `amended_from`, Version com diffs para `trackChanges`, Comment e uma transação por requisição/job/teste, com rollback em erro. |
| Precisão monetária | Precisão por site (`currencyPrecision`, padrão = unidade menor ISO da moeda) e regra de arredondamento (`commercial`/`bankers`); `Currency` é arredondado na escrita e a mesma regra vale em Go, no runtime da app e no desk. `Percent`, `Float` e `Int` ficam de fora, por contrato. |
| Extensão entre apps | `extendDoctype` acrescenta campos e sobrescreve propriedades de um DocType de outro app, com permissões aditivas e `hasPermission`/`permissionQuery` encadeados; conflito entre dois apps recusa a carga. Scripts de formulário de quem estende somam-se ao do dono. |

Referências: [fieldtypes](agent/fieldtypes.md), [migrações](agent/migrations.md),
[controller API](agent/controller-api.md), [extensões](agent/extending.md),
[metadados](../internal/meta/meta.go), [merge](../internal/meta/extend.go),
[schema](../internal/db/schema.go) e
[Document](../internal/engine/doc.go).

## Regras, permissões e identidade

| Capacidade | Implementação disponível |
|---|---|
| Runtime de app | Controllers, serviços, reports, workspaces, patches e testes em TypeScript síncrono no goja. |
| Hooks | `beforeValidate`, `validate`, `beforeSave`, `beforeInsert`, `afterInsert`, `onUpdate`, submit/cancel, `onUpdateAfterSubmit`, delete e rename; há também `docEvents` no manifesto. |
| RPC | Métodos do controller e funções `whitelisted`, acessíveis por HTTP e pelo Desk. |
| Bridge `ddcore.*` | Banco, documentos, mensagens/erros, sessão, papéis, permissões, cache, HTTP síncrono, jobs, SSE, logs e utilitários. |
| Permissões por papel | `read`, `write`, `create`, `delete`, `submit`, `cancel`, `amend`, `report`, `export` e `ifOwner`; Administrator passa pelas verificações. |
| Regras de app | `hasPermission` e `permissionQuery` complementam a regra declarativa. |
| Autenticação | User, Role, Has Role, sessão por cookie, CSRF, senha Argon2id e API key `key:secret`; usuários/chaves desativados não autenticam. |
| Cache | `ddcore.cache.get/set/del`, usado também para papéis, sessões e chaves de API. |

Referências: [controller API](agent/controller-api.md), [permissões](../internal/engine/perm.go),
[autenticação](../internal/engine/auth.go) e [bridge](../internal/engine/host.go).

## API, arquivos e tempo real

| Capacidade | Implementação disponível |
|---|---|
| REST | Listar, contar, criar, obter, atualizar, apagar, enviar, cancelar, emendar, renomear e executar métodos. |
| Consultas | Filtros, `or_filters`, filtros em filhos, campos selecionados, agregações, agrupamento, ordenação, paginação e contagem. |
| Boot e metadados | `/api/boot`, `/api/meta/:doctype` e `/api/translations`, com tradução no limite HTTP, ETag e negociação de idioma. |
| Busca de Link | Busca por nome, título e campos configurados, com filtros e resolução em lote de títulos. |
| Relatórios e dashboards | Endpoints para Script Reports, cards e gráficos de workspace, com permissões aplicáveis. |
| Arquivos | Upload multipart, DocType File, caminhos públicos/privados e download privado autenticado. |
| SSE | `/api/events` e `ddcore.publish` para atualizações de documento/lista, progresso, jobs e eventos destinados a usuário. |
| Servidor e MCP | Healthcheck, SPA e assets embutidos; `/mcp` no dev protegido por API key administrativa. |

Referências: [CLI/API HTTP](agent/cli.md), [API](../internal/api/api.go) e
[hub de eventos](../internal/engine/hub.go).

## Desk

| Capacidade | Implementação disponível |
|---|---|
| Rotas | Login, página inicial, workspace, lista, novo documento, formulário e relatório. |
| Listas | Colunas da meta ou `defineListView`, busca, filtros, ordenação, paginação, contagem, Links, indicadores de status e CSV da página carregada. |
| Formulários | Layout por seção/coluna/aba; controles dos fieldtypes; estados calculados; salvar, enviar, cancelar, emendar, apagar e renomear. |
| Grid de filhos | Edição inline ou dialog, inclusão/remoção, reordenação, larguras e propriedades alteráveis por form script. |
| Scripts de tela | `defineForm`, `defineListView`, `frm.*`, filtros de Link, botões, ações, indicadores, chamadas assíncronas e propriedades dinâmicas. |
| Interação | Dialog, prompt, confirmação, mensagens, toasts e exibição padronizada de erros. |
| Histórico | Sidebar com comentários, versões, criação e modificação. |
| Reports/workspaces | Filtros, tabela, totais, summary, gráficos bar/line/pie/donut, CSV, sidebar explícita, atalhos, links e cards. |
| Localização visual | Moeda, número, data e status; Date/Month civis e Datetime no fuso do site. |

Referências: [form API](agent/form-api.md), [report API](agent/report-api.md),
[componentes](../desk/src/lib/components) e [controls](../desk/src/lib/controls).

## Jobs, scheduler e internacionalização

| Capacidade | Implementação disponível |
|---|---|
| Fila persistente | `ddcore.enqueue` cria jobs no Postgres; workers usam `FOR UPDATE SKIP LOCKED`. |
| Scheduler | Cron e frequências `all`, `hourly`, `daily`, `weekly` e `monthly` declaradas no manifesto da app. |
| Robustez de jobs | Timeout, tentativas, resultado/erro, lease com heartbeat e retorno à fila após interrupção do worker. |
| Gestão de jobs | `ddcore jobs list`, `jobs run <fn>` e `jobs work`. |
| Traduções | Inglês como valor canônico; catálogos CSV do core/apps, extração e validação com `ddcore i18n extract`. |
| Idioma e fuso | Resolução por `X-Lang`, User, `Accept-Language` e instância; utilitários e controles seguem o fuso configurado. |
| Semântica de datas | `Date`/`Month` são datas civis e nunca se deslocam; um `Datetime` sem offset é lido no fuso do site; `daily` dispara à meia-noite do site; `Time` é validado. |

Referências: [jobs](../internal/engine/jobs.go), [i18n](agent/i18n.md) e [CLI](agent/cli.md).

## Ferramentas e exemplo

| Capacidade | Implementação disponível |
|---|---|
| Scaffold | `init`, `new-app` e `scaffold_doctype`. |
| Desenvolvimento | `dev` com watcher/hot reload e `--auto-migrate`; `start` sem watcher; configuração em `ddcore.json`. |
| Tipos e testes | `ddcore types` gera `.ddcore/types.d.ts`; `ddcore test` executa testes TS em transações revertidas; a suíte Go cobre engine, API, runtime, i18n e aceitação HTTP. |
| Administração | `exec`, `eval` com rollback padrão, `user add`, `user passwd`, `apikey`, `demo` e `doctor`. |
| MCP stdio | Tools para meta, migração, dados, métodos, SQL somente leitura, avaliação, testes, logs, reload e apps; resources para documentação e meta. |
| App de exemplo | `apps/demo` contém Project, Task e Project Milestone, serviços, scheduler, scripts, workspace, report, traduções e testes. |

Referências: [CLI](agent/cli.md), [MCP](../internal/mcp/mcp.go),
[scaffold](../internal/scaffold/scaffold.go), [typegen](../internal/typegen/typegen.go) e
[demo](../apps/demo).

## Superfícies não classificadas como completas

- `isSingle` existe no tipo de DocType, mas o migrador não cria a tabela de um
  Singleton; este documento não considera Settings persistente implementado.
- `Password` é `text`; apenas senhas de login e secrets de API key recebem hash.
- O CSV das listas/reports contém o resultado carregado no Desk, não uma exportação
  integral e reconciliável de um DocType.
- HTTP de saída e fila existem, mas não há produto de e-mail, notificações declarativas
  ou webhooks configuráveis.
- Docstatus e controllers permitem aprovações específicas da app; não há motor
  declarativo de workflow listado aqui.
- Fixtures continuam inserindo ou pulando por nome: não atualizam documento existente.
  Só a ordem entre DocTypes passou a ser determinística.
- Um preenchimento grande segura os locks da migração pela duração inteira; não há
  janela fora da transação única, e o caminho previsto é enfileirar e validar no
  release seguinte.

## Evidência

O inventário foi confrontado com `docs/agent/`, SDK, core, engine, API, Desk e demo.
Uma alteração de código continua exigindo `make test` conforme [AGENTS.md](../AGENTS.md).
