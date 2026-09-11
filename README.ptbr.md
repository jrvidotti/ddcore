<!-- mirror-of: README.md -->

# ddcore

**ddcore** (*Data Driven Core*) é um port do modelo do Frappe Framework para **Go + TypeScript + PostgreSQL**, feito para ser
desenvolvido por ferramentas agênticas. Um binário (`ddcore`) embute o esbuild e o goja: os apps
são TypeScript (DocTypes, regras, relatórios, scripts de tela), executados no servidor sem Node.
O desk (Svelte 5) é gerado a partir da meta. Só depende de um Postgres.

O design completo está em [`docs/plan-v1.md`](docs/plan-v1.md); a referência para agentes em
[`docs/agent/`](docs/agent/) (`ddcore docs`, ou os resources MCP `ddcore://docs/*`), em inglês.

## Desenvolver o framework

```bash
make docker-up                              # Postgres de dev (container ddcore-pg, porta 5455)
make build                                  # desk (npm) + binário em bin/ddcore
make migrate                                # DDL do core
./bin/ddcore user passwd Administrator admin1234 # senha do Administrator
make dev                                    # http://localhost:8090  (Administrator / admin)
```

O `ddcore.json` deste checkout carrega `apps/demo` — um app pequeno de projetos e
tarefas que serve de tutorial executável e de fixture ponta a ponta (`./bin/ddcore demo`
semeia o projeto `DEMO`). Apps de produto vivem em repositórios separados. Em um projeto
externo, o roteiro começa com `ddcore init && ddcore new-app <nome>`; `DDCORE_DSN`
sobrescreve o DSN de qualquer comando.

O processo de desenvolvimento (núcleo x app, ciclo de trabalho) está em
[`DEVELOPMENT.ptbr.md`](DEVELOPMENT.ptbr.md).

## Idioma

Inglês é o idioma-fonte. Toda string que uma pessoa lê — uma chamada a `_()`, um `label:`,
os valores de um Select — é escrita em inglês e é uma chave de catálogo; as traduções ficam
em `translations/<lang>.csv` e são derivadas do código por `ddcore i18n extract`. O contrato
está em [`docs/agent/i18n.md`](docs/agent/i18n.md) (em inglês).

## Verificação

```bash
make check   # typecheck do desk + catálogo de traduções
make test    # build + go vet + testes Go e do desk
```

Os testes Go incluem `internal/acceptance`, que cria um app externo temporário e valida
instalação, boot, tradução e `/app` contra Postgres e HTTP reais. O banco descartável de
`DDCORE_TEST_DSN` é recriado a cada teste.

## Layout

```
cmd/ddcore/        CLI
internal/         meta · db (migrate) · js (esbuild+goja) · engine (Document, permissões, jobs) · api · mcp · scaffold · typegen · i18nx (extractor)
core/             app embutido: User, Role, Has Role, File, Comment, Version, Error Log, API Key
desk/             SvelteKit (build embutido no binário)
packages/sdk      @ddcore/sdk — defineDoctype/defineController/… e tipos do bridge
packages/desk-sdk @ddcore/desk-sdk — defineForm, frm.*, Dialog
docs/agent        referência para agentes
```

## MCP

`ddcore mcp` (stdio) ou `http://localhost:8090/mcp` com `ddcore dev`. Tools: scaffold/migrate/types,
CRUD de documentos, `call_method`, `sql_query`, `eval`, `run_tests`, `get_logs`. Este repo já traz
`.mcp.json` para o Claude Code.
