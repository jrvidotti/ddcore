# CLI e fluxo de desenvolvimento

`cerne.json` no diretório do site: `dsn`, `apps` (diretórios), `port`, `workers`, `scheduler`, `site`, `lang`, `currency`, `dev`.
`CERNE_DSN` sobrescreve o dsn.

As opções podem vir **antes ou depois** dos argumentos posicionais, nas formas `--flag valor` e
`--flag=valor`; `--` encerra as opções e o que vier depois é posicional. Flag não declarada é um
erro (não vira argumento silenciosamente).

| Comando | O que faz |
|---|---|
| `cerne init --dsn ... --port ...` | cria `cerne.json`; se já existir, atualiza dsn/port informados (idempotente) |
| `cerne new-app <nome>` | scaffold do app + registro no cerne.json + CLAUDE.md |
| `cerne dev` | servidor com hot-reload (recompila ao salvar .ts/.csv) e `--auto-migrate`; expõe `/mcp` |
| `cerne start` | servidor de produção (sem watcher) |
| `cerne migrate [--dry-run] [--prune]` | DDL + afterInstall + fixtures + patches + afterMigrate; depois gera types |
| `cerne types` | gera `.cerne/types.d.ts` e materializa as tipagens embutidas dos SDKs por app |
| `cerne test [--filter re] [-v]` | roda `*.test.ts` (cada `it` numa transação revertida) |
| `cerne exec app.mod.fn --args '{}'` | executa uma função como Administrator |
| `cerne eval '<ts>' [--commit]` | executa TS avulso com `cerne.*` (rollback por padrão) |
| `cerne demo [--app nome]` | roda `<app>.services.demo.gerar` das apps que têm `services/demo.ts` |
| `cerne jobs list|run <fn>|work` | scheduler e fila |
| `cerne user add <email> <nome> --password x --role R` / `user passwd` | usuários |
| `cerne apikey <usuario>` | gera `key:secret` para `Authorization: token key:secret` |
| `cerne mcp` | servidor MCP (stdio) |
| `cerne docs [nome]` | esta documentação |
| `cerne doctor` | banco, meta, DDL pendente, scheduler |

## App em repositório próprio

```bash
mkdir minha_app && cd minha_app
cerne init
cerne new-app minha_app --dir .
cerne migrate
cerne test
cerne dev
```

Use `apps: ["."]` quando a raiz do repositório for o próprio app. O binário resolve os
imports dos SDKs em runtime e `cerne types` grava as declarações correspondentes em
`.cerne/`; não é necessário manter o framework como checkout irmão nem instalar os SDKs
do npm. `CERNE_TEST_DSN` aponta o banco descartável usado pelos testes.

## API HTTP

- `POST /api/login {usr, pwd}` → cookie `sid`; requisições mutantes com cookie exigem header `X-Cerne-CSRF: 1`.
- `GET /api/resource/<DocType>?filters=[...]&fields=[...]&order_by=&limit=&start=&with_count=1`
- `POST /api/resource/<DocType>` (insert), `GET/PUT/DELETE /api/resource/<DocType>/<name>`
- `POST /api/resource/<DocType>/<name>/<submit|cancel|amend|rename|metodo>`
- `POST /api/method/<app.pasta.arquivo.fn>` (whitelisted)
- `GET /api/meta/<DocType>`, `/api/boot`, `/api/search/link?doctype=&txt=`, `/api/report/<nome>`, `/api/events` (SSE), `POST /api/upload`
- Erros: `{ "error": { "type", "title", "message" } }` com 417 (validação), 403, 404, 409, 401.

## MCP (`cerne mcp` ou `http://localhost:<port>/mcp` no dev)

Tools: `list_doctypes`, `get_doctype`, `scaffold_doctype`, `validate_meta`, `migrate`, `generate_types`, `get_doc`, `list_docs`, `insert_doc`,
`update_doc`, `delete_doc`, `submit_doc`, `cancel_doc`, `call_method`, `sql_query`, `eval`, `run_tests`, `get_logs`, `reload`, `list_apps`.
Resources: `cerne://docs/<nome>`, `cerne://meta/<DocType>`.
