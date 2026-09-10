# ddcore

**ddcore** (*Data Driven Core*) — framework Go + TypeScript + Postgres no espírito do Frappe. Leia `docs/plan-v1.md` (design) e
`docs/agent/index.md` (referência da API para escrever apps) e `DESENVOLVIMENTO.md`
(processo de desenvolvimento). `apps/exemplo` é o app de exemplo carregado pelo
`ddcore.json` deste checkout; apps de produto vivem em repositórios separados.

- `make build` compila desk + binário; `./bin/ddcore dev` sobe em :8090 com hot-reload; `make test` roda Go + TS.
- Postgres de dev: container `ddcore-pg` na porta 5455 via `make docker-up` (Docker Compose). Testes Go usam o banco `ddcore_test` (recriado).
- Código de app é TypeScript **síncrono** rodando no goja (sem `await` no servidor). Scripts de tela usam `@ddcore/desk-sdk`.
- `.ddcore/types.d.ts` é gerado (`ddcore types`) — não edite.
- MCP: `.mcp.json` aponta para `./bin/ddcore mcp`; use as tools em vez de mexer no banco à mão.
