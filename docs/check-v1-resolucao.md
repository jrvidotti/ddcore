# Resolução da revisão v1

Referência: `docs/check-v1.md` (revisão do commit `f54ea30`). Este documento mapeia cada achado
à correção, ao teste que o cobre e à evidência de revalidação. Preenchido durante a integração
das quatro frentes de correção.

| Achado | Prioridade | Frente | Situação | Correção / teste |
|---|---|---|---|---|
| B01 MCP HTTP sem autenticação | P0 | A | pendente | |
| B02 CRUD direto em filhos | P0 | A | pendente | |
| B03 filho transferido entre pais | P1 | B | pendente | |
| B04 histórico vaza documentos e hashes | P1 | A | pendente | |
| B05 gráfico/cards do workspace públicos | P1 | A | pendente | |
| B06 SQL "readonly" aceita escrita | P1 | B | pendente | |
| B07 perda de atualização concorrente | P1 | B | pendente | |
| B08 reload devolve runtime antigo | P1 | B | pendente | |
| B09 apuração histórica com baixas futuras | P1 | C | pendente | |
| B10 "Recebido no Mês" soma outros meses | P1 | C | pendente | |
| B11 reajuste aplicado duas vezes | P1 | C | pendente | |
| B12 faturamento não idempotente sob concorrência | P1 | C | pendente | |
| B13 renovação automática não fatura | P2 | C | pendente | |
| B14 índices únicos por tipo e mudança de meta | P1 | B | pendente | |
| B15 jobs sem timeout nem recuperação | P1 | B | pendente | |
| B16 typecheck do desk e do app reprovado | P1 | D | pendente | |
| B17 assinaturas SSE após desmontar | P2 | D | pendente | |
| B18 datas divergentes cliente/servidor | P2 | D | pendente | |
| B19 contagem/paginação com filtros de filhos | P2 | B | pendente | |
| B20 eventos ignoram transação e permissão | P2 | A+B | pendente | |
| B21 dbSet devolve timestamp desatualizado | P2 | B | pendente | |
| B22 CLI diverge dos exemplos; eval sem TS | P2 | B+D | pendente | |
| B23 suíte e bootstrap incompletos | P2 | D | pendente | |

## Revalidação dos critérios de pronto

| Critério | Evidência |
|---|---|
| 1. migrate em banco vazio, idempotente | pendente |
| 2. `cerne test` verde | pendente |
| 3. fluxo no navegador | pendente |
| 4. cadeia MCP completa + HTTP sem credencial negado | pendente |
| 5. mordidas do Frappe cobertas | pendente |
