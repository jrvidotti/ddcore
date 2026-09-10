# cerne

Port do modelo do Frappe Framework para **Go + TypeScript + PostgreSQL**, feito para ser
desenvolvido por ferramentas agênticas. Um binário (`cerne`) embute o esbuild e o goja: os apps
são TypeScript (DocTypes, regras, relatórios, scripts de tela), executados no servidor sem Node.
O desk (Svelte 5) é gerado a partir da meta. Só depende de um Postgres.

O design completo está em [`docs/plan-v1.md`](docs/plan-v1.md); a referência para agentes em
[`docs/agent/`](docs/agent/) (`cerne docs`, ou os resources MCP `cerne://docs/*`).

## Desenvolver o framework

```bash
make docker-up                              # Postgres de dev (container cerne-pg, porta 5455)
make build                                  # desk (npm) + binário em bin/cerne
make migrate                                # DDL do core
./bin/cerne user passwd Administrator admin # senha do Administrator
make dev                                    # http://localhost:8090  (Administrator / admin)
```

O `cerne.json` deste checkout carrega `apps/exemplo` — um app pequeno de projetos e
tarefas que serve de tutorial executável e de fixture ponta a ponta (`./bin/cerne demo`
semeia o projeto `DEMO`). Apps de produto vivem em repositórios separados, como o
`alugueis`. Em um projeto externo, o roteiro começa com `cerne init && cerne new-app
<nome>`; `CERNE_DSN` sobrescreve o DSN de qualquer comando.

O processo de desenvolvimento (núcleo x app, ciclo de trabalho) está em
[`DESENVOLVIMENTO.md`](DESENVOLVIMENTO.md).

## Verificação

```bash
make check   # typecheck do desk
make test    # build + go vet + testes Go e do desk
```

Os testes Go incluem `internal/acceptance`, que cria um app externo temporário e valida
instalação, boot, tradução e `/app` contra Postgres e HTTP reais. O banco descartável de
`CERNE_TEST_DSN` é recriado a cada teste.

## Layout

```
cmd/cerne/        CLI
internal/         meta · db (migrate) · js (esbuild+goja) · engine (Document, permissões, jobs) · api · mcp · scaffold · typegen
core/             app embutido: User, Role, Has Role, File, Comment, Version, Error Log, API Key
desk/             SvelteKit (build embutido no binário)
packages/sdk      @cerne/sdk — defineDoctype/defineController/… e tipos do bridge
packages/desk-sdk @cerne/desk-sdk — defineForm, frm.*, Dialog
docs/agent        referência para agentes
```

## MCP

`cerne mcp` (stdio) ou `http://localhost:8090/mcp` com `cerne dev`. Tools: scaffold/migrate/types,
CRUD de documentos, `call_method`, `sql_query`, `eval`, `run_tests`, `get_logs`. Este repo já traz
`.mcp.json` para o Claude Code.
