# Diretrizes para Agentes de IA

Este repositório adota as convenções e a arquitetura documentadas em [`CLAUDE.md`](file:///Users/junior/dev/ddcore/CLAUDE.md), que deve ser utilizado como referência primária para o framework DDCore, organização de pastas, scripts e convenções de código.

---

## Servidor de Desenvolvimento (`dev`)

- **NÃO inicie um servidor de desenvolvimento (`./bin/ddcore dev` ou `make dev`) se já houver um em execução.**
- Antes de tentar subir o servidor, verifique sempre se a porta 8090 já está ocupada ou respondendo:
  ```bash
  lsof -ti tcp:8090
  # ou
  curl -I http://localhost:8090
  ```
- O servidor de desenvolvimento possui **hot-reload automático** (vigilância de arquivos via `watch.Apps`). Ao editar doctypes, controllers ou serviços, o servidor recarrega as definições automaticamente em memória, portanto **não é necessário reiniciar o processo**.
- Para reiniciar explicitamente caso estritamente necessário (ex.: após recompilar o desk em `make build`), utilize `make stop` antes de iniciar novamente.

---

## Referências e Boas Práticas Essenciais

Consulte sempre o [`CLAUDE.md`](file:///Users/junior/dev/ddcore/CLAUDE.md) e a documentação em [`docs/agent/index.md`](file:///Users/junior/dev/ddcore/docs/agent/index.md):

1. **TypeScript Síncrono no Servidor:** Código de apps que roda no servidor (goja) é síncrono; nunca utilize `await` em controllers ou serviços de servidor. Scripts do desk em `@ddcore/desk-sdk` rodam no navegador e podem ser assíncronos.
2. **Tipagens Geradas:** Nunca edite manualmente `.ddcore/types.d.ts`. Use `./bin/ddcore types` ou `make test`.
3. **Testes:** Valide sempre suas alterações com `make test` (testes Go + TypeScript de desk e apps).
4. **MCP:** Utilize as tools do DDCore MCP definidas em `.mcp.json` para inspecionar metadados, executar métodos e rodar migrações.
