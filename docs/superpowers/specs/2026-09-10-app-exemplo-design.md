# App Exemplo do DDCore — Especificação

## Objetivo

Criar `apps/exemplo`, um app pequeno de projetos e tarefas que funcione como tutorial
executável, fixture ponta a ponta e referência de boas práticas do DDCore. Ele deve mostrar
como as partes principais do framework se conectam sem carregar regras de um produto real.

O app será rastreado no repositório do DDCore, carregado pelo `ddcore.json` de desenvolvimento
e coberto por `make test`. O código de servidor continuará estritamente síncrono; somente
scripts do desk poderão usar APIs assíncronas.

## Escopo funcional

O fluxo demonstrado será:

1. criar um projeto com marcos;
2. cadastrar tarefas ligadas ao projeto;
3. iniciar, concluir e reabrir tarefas por métodos do controller;
4. recalcular automaticamente o progresso do projeto;
5. marcar diariamente tarefas vencidas como atrasadas;
6. acompanhar cards, gráfico e relatório no workspace Projetos;
7. instalar dados de demonstração de forma idempotente.

Não fazem parte do app: integrações externas, e-mail, anexos, aprovação, timesheets,
dependência de outro app, customização de autenticação ou patches sem uma migração
real a demonstrar.

## Estrutura

```text
apps/exemplo/
  ddcore.app.ts
  tsconfig.json
  client/listas.ts
  doctypes/
    marco_projeto/marco_projeto.doctype.ts
    projeto/projeto.doctype.ts
    projeto/projeto.controller.ts
    projeto/projeto.form.ts
    projeto/projeto.test.ts
    tarefa/tarefa.doctype.ts
    tarefa/tarefa.controller.ts
    tarefa/tarefa.form.ts
    tarefa/tarefa.test.ts
  reports/tarefas_por_status.report.ts
  services/demo.ts
  services/demo.test.ts
  services/projetos.ts
  services/tarefas.ts
  services/tarefas.test.ts
  translations/pt-BR.csv
  workspaces/projetos.workspace.ts
  .ddcore/                         # gerado e ignorado pelo Git
```

O manifesto usará:

- nome `exemplo`, título `Exemplo: Projetos` e versão `0.1.0`;
- roles `Gestor de Projetos` e `Colaborador de Projetos`;
- `desk.home: "Projetos"` e `desk.include: ["client/listas.ts"]`;
- scheduler diário `exemplo.services.tarefas.marcarAtrasadas`;
- nenhuma dependência em `requires`.

## Modelo de dados

### Marco Projeto

DocType filho usado na tabela `marcos` de Projeto.

| Campo | Tipo | Regras |
|---|---|---|
| `titulo` | Data | obrigatório |
| `data_prevista` | Date | obrigatório |
| `concluido` | Check | padrão falso |
| `concluido_em` | Date | obrigatório quando `concluido` for verdadeiro |

`concluido_em` usará `mandatoryDependsOn: "doc.concluido"`, demonstrando validação
condicional tanto no desk quanto no servidor.

### Projeto

DocType principal com `naming: { field: "codigo" }`, `titleField: "titulo"` e
`trackChanges: true`.

| Campo | Tipo | Regras |
|---|---|---|
| `codigo` | Data | obrigatório, único, em lista |
| `titulo` | Data | obrigatório, em lista |
| `descricao` | Text | opcional |
| `responsavel` | Link/User | obrigatório, filtro padrão |
| `status` | Select | `Planejado`, `Em andamento`, `Concluído`; somente leitura |
| `data_inicio` | Date | obrigatório |
| `data_final` | Date | opcional, nunca anterior ao início |
| `progresso` | Percent | derivado e somente leitura |
| `marcos` | Table/Marco Projeto | editável em grid |

O controller validará o intervalo de datas e manterá os marcos consistentes:
marco concluído exige data; marco reaberto limpa `concluido_em`. O status e o progresso
do projeto serão derivados das tarefas, não digitados pelo usuário.

### Tarefa

DocType principal com `naming: { field: "codigo" }`, `titleField: "titulo"` e
`trackChanges: true`.

| Campo | Tipo | Regras |
|---|---|---|
| `codigo` | Data | obrigatório, único, em lista |
| `projeto` | Link/Projeto | obrigatório, em lista e filtro padrão |
| `titulo` | Data | obrigatório, em lista |
| `descricao` | Text | opcional |
| `responsavel` | Link/User | obrigatório, filtro padrão |
| `prioridade` | Select | `Baixa`, `Média`, `Alta`; padrão `Média` |
| `status` | Select | `Aberta`, `Em andamento`, `Atrasada`, `Concluída`; somente leitura |
| `data_limite` | Date | obrigatório |
| `concluida_em` | Datetime | derivado e somente leitura |

A data limite não poderá anteceder o início do projeto. Tarefas novas começarão
como `Aberta`. Os métodos `iniciar`, `concluir` e `reabrir` serão idempotentes, salvarão
o documento pelo lifecycle normal e devolverão `{ status, concluida_em }`.

Ao concluir, `concluida_em` receberá `ddcore.utils.now()`; ao reabrir, será limpo.
Depois de inserir, atualizar ou excluir uma tarefa, o serviço de projetos recalculará o
projeto relacionado.

## Regras e serviços

`services/projetos.ts` exportará `recalcularProgresso(projeto: string): void`:

- sem tarefas: progresso `0` e status `Planejado`;
- com tarefas e pelo menos uma não concluída: percentual de concluídas, arredondado para
  duas casas, e status `Em andamento`;
- todas concluídas: progresso `100` e status `Concluído`.

A atualização usará `dbSet` nos dois campos derivados para não criar recursão de hooks.
Ao mover uma tarefa entre projetos, tanto o projeto anterior quanto o novo serão
recalculados usando `getDocBeforeSave()`.

`services/tarefas.ts` exportará:

- `marcarAtrasadas(): number`, usado pelo scheduler, que seleciona tarefas `Aberta` ou
  `Em andamento` com `data_limite` anterior a hoje, salva cada documento como `Atrasada`,
  registra falhas isoladas e devolve a quantidade alterada;
- `marcarAtrasadasAgora`, wrapper whitelisted da mesma operação, restrito a
  `Gestor de Projetos`, para permitir execução manual no desk e demonstrar `/api/method`;
- `resumoPorStatus(filters): { status: string; quantidade: number; percentual: number }[]`,
  compartilhado pelo relatório e pelo gráfico do workspace.

Tarefas concluídas nunca serão marcadas como atrasadas. Reabrir uma tarefa vencida a
colocará diretamente em `Atrasada`; caso contrário, em `Aberta`.

## Desk

O form de Projeto mostrará um indicador de progresso e botão para abrir a lista de tarefas
filtrada pelo projeto. O grid de marcos permitirá inclusão e edição direta.

O form de Tarefa:

- limitará o Link Projeto a projetos ainda não concluídos;
- mostrará indicador colorido do status;
- exibirá, conforme o estado, os botões `Iniciar`, `Concluir` e `Reabrir`;
- chamará exclusivamente os métodos do controller e recarregará o documento depois.

`client/listas.ts` registrará `defineListView` para Tarefa, com colunas projeto, título,
responsável, prioridade, status e data limite; ordenação por `data_limite asc`; e
indicadores azul, amarelo, vermelho e verde para os quatro estados.

Todo texto acionável será escrito como chave traduzível em inglês e mapeado em
`translations/pt-BR.csv`, incluindo `Start`, `Complete`, `Reopen`, `Open`, `In progress`,
`Overdue` e `Completed`.

## Relatório e workspace

O relatório `Tarefas por Status` aceitará filtros opcionais `projeto`, `responsavel` e
`data_limite_ate`. Retornará uma linha por status, com quantidade e percentual do total,
além de gráfico de barras. Filtros serão aplicados via `ddcore.db.getList`; SQL direto não
será usado no app exemplo.

O workspace `Projetos` será visível para `System Manager`, `Gestor de Projetos` e
`Colaborador de Projetos` e conterá:

- sidebar explícita para Projetos, Tarefas e Tarefas por Status;
- shortcuts para as listas de Projetos e Tarefas;
- cards de projetos em andamento, tarefas abertas e tarefas atrasadas;
- gráfico de tarefas por status;
- links agrupados em Planejamento e Acompanhamento.

Cards e gráfico usarão as APIs de DocType ou serviços compartilhados com o relatório,
sem duplicar regras de contagem.

O relatório declarará as roles `System Manager`, `Gestor de Projetos` e
`Colaborador de Projetos`, as mesmas que recebem acesso ao seu link no workspace.

## Instalação e demo

As roles serão declaradas no manifesto e criadas pelo fluxo normal de instalação. O app
não criará dados de negócio em `afterInstall`.

`services/demo.ts` exportará `gerar()`, detectada por `ddcore demo`. Ela criará um projeto
`DEMO` com três marcos e três tarefas (`DEMO-01` a `DEMO-03`) nos estados aberta, em
andamento e concluída. A rotina consultará cada código antes de inserir, poderá ser
executada repetidamente e retornará os nomes criados e a quantidade de novos registros.
As datas serão relativas a `ddcore.utils.today()`, e os estados não iniciais serão obtidos
pelos métodos do controller, sem gravar campos derivados diretamente.

## Permissões

- `System Manager`: bypass normal do core.
- `Gestor de Projetos`: criar, ler, editar, excluir, reportar e exportar Projeto e Tarefa.
- `Colaborador de Projetos`: ler Projeto; criar, ler, editar e reportar Tarefa; sem excluir,
  exportar ou executar a rotina manual de atraso.
- Marco Projeto herdará o acesso do pai por ser tabela filha.

O app não implementará `hasPermission` ou `permissionQuery`; esses recursos continuam
cobertos pelos testes do framework e adicionariam complexidade sem servir ao fluxo exemplo.

## Testes e integração com o monorepo

Os testes TypeScript cobrirão:

- rejeição de intervalo inválido em Projeto;
- consistência das datas dos marcos;
- criação de Tarefa como aberta e rejeição de prazo anterior ao projeto;
- transições idempotentes iniciar, concluir e reabrir;
- recálculo do projeto com zero, parte e todas as tarefas concluídas;
- recálculo dos dois projetos quando uma tarefa muda de pai;
- scheduler alterando somente tarefas vencidas não concluídas;
- filtros e totais do relatório;
- duas execuções da demo sem duplicidade.

Ao implementar o app:

- `ddcore.json` passará a carregar `apps/exemplo`;
- `make check` incluirá o `tsc` do app;
- `make test` executará os testes TS do app além das suítes Go e do desk;
- a aceitação deixará de criar uma fixture temporária equivalente e validará o app real
  em banco descartável;
- `.ddcore/` permanecerá ignorado e será sempre regenerado por `ddcore types`.

## Critérios de aceite

1. `make test` passa em checkout limpo com Postgres disponível.
2. `ddcore migrate` em banco vazio instala core e exemplo; o dry-run seguinte fica vazio.
3. `ddcore demo` executado duas vezes não duplica Projeto, Marco Projeto nem Tarefa.
4. Login abre o workspace Projetos e todos os destinos da sidebar existem.
5. Criar projeto e tarefas, concluir uma tarefa e reabri-la atualiza documento e progresso
   sem refresh manual fora do fluxo normal do form.
6. A rotina de atraso pode rodar pelo scheduler e manualmente com a mesma regra.
7. Relatório, cards e gráfico apresentam contagens coerentes para os mesmos filtros.
8. Nenhum arquivo do app importa fontes por caminho relativo fora de `apps/exemplo`;
   SDKs são consumidos somente por `@ddcore/sdk` e `@ddcore/desk-sdk`.
