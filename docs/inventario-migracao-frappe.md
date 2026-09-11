# Inventário pós-v1: evolução do ddcore e migração de apps Frappe

Data: 10/09/2026. Base inspecionada: commit `ee437e9`, documentação e código deste
checkout. Referência de origem: Frappe v16, conforme [plan-v1.md](plan-v1.md).
Status: recomendação de roadmap. As funcionalidades propostas aqui não estavam implementadas na
data acima; os itens marcados **Implementado** foram entregues depois e apontam seu contrato.

## 1. Direção recomendada

A v1 cumpriu o objetivo definido: executar um app externo ponta a ponta com DocTypes,
regras, Desk, relatórios, workspace, scheduler e testes. O próximo marco deve ser
**migrar uma operação real com dados reconciliados, permissões equivalentes e retorno
ensaiado ao sistema anterior**. O aceite de um app comprova o núcleo escolhido para a
v1; a cobertura necessária para outros apps depende de suas funcionalidades e dados.

Recomendo esta ordem:

1. Inventariar o próximo app/site Frappe e escolher um fluxo piloto completo.
2. Preparar importação, reconciliação, backup/restauração e controles de acesso.
3. Completar configurações Singleton e armazenamento de segredos, quando utilizados.
4. Entregar impressão/PDF, e-mail/notificações e workflows conforme o fluxo escolhido.
5. Ensaiar a migração, homologar com os usuários e executar a virada controlada.

O escopo sugerido para a v2 é **migração e operação de apps administrativos**, com
permissões granulares e os serviços compartilhados que evitam repetir infraestrutura
em cada app. Portais, construtores visuais e a cobertura integral do Frappe ficam
condicionados a demanda concreta.

### Limites da análise

- O estado do ddcore foi conferido nas referências canônicas em [agent/index.md](agent/index.md),
  no SDK e na implementação. A consulta MCP `list_doctypes`, pelo comando da
  [.mcp.json](../.mcp.json), retornou 11 DocTypes: oito do core e três do demo.
- Os repositórios externos `alugueis` e `rent-frappe`, os dados reais e as integrações
  de produção não foram auditados nesta análise. As prioridades por app precisam
  desse levantamento; não há estimativa de prazo fundamentada antes dele.
- As fontes oficiais do Frappe foram consultadas na data acima. São documentação
  viva: o inventário de cada origem deve registrar versão e commit exatos dos apps.
- “Existente” significa que há implementação identificada, sem afirmar compatibilidade
  integral com cada detalhe do Frappe. “Parcial” identifica uma base que requer extensão.
  “Não identificado” significa ausência no código/SDK inspecionado, não um teste de impossibilidade.

## 2. Leitura do plano da v1

As decisões de arquitetura continuam adequadas ao objetivo: Go como host, TypeScript
síncrono no goja, metadados em arquivos, Postgres, um tenant por instância e Desk
gerado por metadados. As próximas funcionalidades devem preservar esses contratos.

O plano já adiou explicitamente permissões por campo e User Permissions para a v2.
Também não incluiu um produto de importação de dados, workflows configuráveis,
impressão, comunicação ou administração de backups. São extensões de escopo após a v1.

Há diferenças entre o documento histórico e o checkout atual:

| No plano/histórico | Evidência atual | Consequência para o roadmap |
|---|---|---|
| App de exemplo adiado | [apps/demo](../apps/demo) e [DEVELOPMENT.md](../DEVELOPMENT.md) | Usar o demo como referência; validar domínio nos apps externos. |
| `internal/doc`, `internal/perm`, `internal/jobs` | Responsabilidades concentradas em [internal/engine](../internal/engine) | Planejar alterações a partir da estrutura real. |
| Hooks e instalação mais enxutos | `beforeInsert`, `onUpdateAfterSubmit`, fixtures e `afterMigrate` no [SDK](../packages/sdk/src/types.ts) | Mapear apenas os hooks ausentes ou com semântica diferente. |
| Revisão antiga com falhas | Há testes de regressão em [fixes_test.go](../internal/engine/fixes_test.go), [perm_test.go](../internal/engine/perm_test.go) e [acceptance_test.go](../internal/acceptance/acceptance_test.go) | O checklist de [check-v1-resolucao.md](check-v1-resolucao.md) permanece “pendente”; precisa de atualização por evidência, sem transformar todo achado antigo em bug atual. |
| Timeout e retry previstos | [jobs.go](../internal/engine/jobs.go) implementa timeout, heartbeat e recuperação por lease | Evoluir administração e observabilidade da fila; não recriar o worker. |

## 3. Base já disponível

| Capacidade | Estado e evidência | Uso na migração |
|---|---|---|
| DocTypes, campos, links e tabelas filhas | Existente: [meta](../internal/meta/meta.go), [schema](../internal/db/schema.go), [fieldtypes](agent/fieldtypes.md) | Reescrever JSON/metadados de origem em definições TS e mapear tipos. |
| Document e lifecycle | Existente: [doc.go](../internal/engine/doc.go), [controller-api](agent/controller-api.md) | Preservar regras de insert/save/submit/cancel/amend/rename e concorrência. |
| Regras e serviços | Existente: [runtime](../internal/js/runtime.go), [bridge](../internal/engine/host.go) | Portar Python para TS síncrono, verificando chamadas e efeitos de cada hook. |
| Permissões básicas | Existente: [perm.go](../internal/engine/perm.go) | Reutilizar roles, `ifOwner`, `hasPermission` e `permissionQuery`. |
| Login e integração autenticada | Existente: [auth.go](../internal/engine/auth.go) | Sessões, Argon2id e API keys são a base; migração de identidades requer política própria. |
| REST/RPC, uploads e SSE | Existente: [api.go](../internal/api/api.go) e [hub.go](../internal/engine/hub.go) | Adaptar consumidores e preservar privacidade de documentos/arquivos. |
| Desk, listas, formulários e scripts | Existente: [components](../desk/src/lib/components), [form-api](agent/form-api.md) | Recriar fluxos com metadados e SDK do Desk. |
| Relatórios, cards e workspaces | Existente: [report-api](agent/report-api.md) | Portar consultas e resultados; comparar com datas de referência idênticas. |
| Jobs e scheduler | Existente: [jobs.go](../internal/engine/jobs.go) | Portar rotinas e testar repetição/interrupção; efeitos externos exigem idempotência. |
| Schema, patches, fixtures e dependências | Existente: [migrate.go](../internal/engine/migrate.go), [fixes_test.go](../internal/engine/fixes_test.go) | Instalar e evoluir apps; distinguir migração de schema da carga do legado. |
| Histórico e comentários | Existente: [core/doctypes](../core/doctypes) | Version/Comment ajudam a operação; preservar também histórico de origem. |
| Idiomas, datas e ferramentas de desenvolvimento | Existente: [i18n](agent/i18n.md), [cli](agent/cli.md), [Makefile](../Makefile) | Manter inglês canônico, fuso do site, geração de tipos, MCP e testes. |

## 4. Inventário priorizado de funcionalidades

Prioridades são propostas para o roadmap, não severidades de bugs:

- **P0:** requisito para uma virada de produção; pode ser atendido por ferramenta
  operacional documentada, sem obrigatoriamente criar uma tela ou comando do core.
- **P1:** funcionalidade importante para ampliar a migração de apps administrativos.
- **P2:** implementar quando houver uso demonstrado no app de origem.

Um item P1/P2 passa a bloquear a virada do app quando sustenta um fluxo obrigatório,
um controle de acesso ou um dado que precisa ser preservado.

### 4.1 Acesso, identidade e segredos

O Frappe oferece permissões por papel, nível de campo e valores de Link por usuário,
além de ações como compartilhar, imprimir e importar. O ddcore já possui o primeiro
nível; precisa ampliar o modelo quando essas restrições existirem na origem.
Fonte: [Users and Permissions](https://docs.frappe.io/framework/user/en/basics/users-and-permissions).

| ID | Capacidade | Situação no ddcore | Prioridade e entrega mínima |
|---|---|---|---|
| SEC-01 | User Permissions: restringir empresa, unidade, carteira etc. por usuário | Parcial: hooks permitem regras específicas; não há entidade/configuração genérica equivalente. | P1; bloqueia apps com segregação. Configurar escopos e aplicá-los em leitura, busca, escrita, relatórios, exportação e eventos. |
| SEC-02 | Permissão por campo (`permlevel` ou equivalente) | Não identificada em `FieldDef`/`PermDef`. `hidden` e `readOnly` não substituem autorização de leitura. | P1; bloqueia campos restritos. Omitir campos não autorizados da resposta e negar escrita; incluir filhos, histórico, relatórios e downloads. |
| SEC-03 | Compartilhamento por documento e administração de permissões | Não identificado: não há DocShare, perfis de papel ou editor genérico de permissões. | P1 para compartilhar; P2 para editores/perfis. Definir precedência, revogação e auditoria sem criar fonte conflitante de metadados. |
| SEC-04 | Recuperação de senha, convite e proteção do login | **Implementado**: a política de acesso é declarada em `auth` no `ddcore.json` e o ambiente (URL pública, proxy, e-mail) no `.env`. O login é limitado por `ddcore_login_attempt` — verificação **antes** do lookup e do Argon2, chave pelo que foi digitado, hash-isca para endereço inexistente — e responde **429** com `Retry-After`. Uma fonte só para o TTL de sessão (SQL e cookie), cookie `Secure` decidido por requisição, `ip`/`user_agent` na sessão e `DropSessions(user, exceptSid)`. Recuperação e convite por token de 192 bits guardado em SHA-256, uso único atômico, em `ddcore_auth_token`, por quatro rotas públicas com throttle próprio; `forgot-password` responde igual para endereço conhecido, desconhecido e desativado. Política de senha no gargalo do hash, valendo nos cinco caminhos. `internal/mail` com SMTP mínimo e transporte plugável por caminho pontilhado. Autosserviço em `core/services/` (perfil, idioma, senha, sessões, chaves) e administração em `users.ts`. Desk: menu no avatar, `/app/profile`, "esqueci minha senha" e a página de redefinição. Contrato em [auth](agent/auth.md). | Entregue, com o abuso coberto por testes (`TestSEC04_*`). Residual: o CSRF continua sendo só presença de header e não há prefixo `__Host-`; não há verificação do e-mail alterado (trocar o e-mail é um *rename*, porque ele é o `name`); falta tela administrativa para destravar conta e listar sessões de terceiros — a tabela já guarda o dado. MFA/SSO seguem no SEC-05. |
| SEC-05 | MFA e SSO/OIDC/LDAP | Não identificados. | P2; obrigatórios antes da virada quando exigidos pela organização. Priorizar o provedor efetivamente usado. |
| SEC-06 | Segredos de integração e campos Password | **Implementado**: credencial de integração não mora em coluna — `ddcore.secret("stripe_key")` lê `DDCORE_SECRET_STRIPE_KEY` do ambiente (`.env` no desenvolvimento, a plataforma na produção), e o prefixo é a fronteira que impede um app de ler o DSN ou a senha do SMTP pelo mesmo caminho. `ddcore doctor` lista os nomes e nunca os valores. Um campo `Password` deixou de sair por leitura: `RedactPassword` apaga o valor em toda resposta da API, somando-se à exclusão que ele já tinha de `Version` e da exportação. Contrato em [fieldtypes](agent/fieldtypes.md#secrets-ddcoresecret-not-a-column) e [configuração](../DEVELOPMENT.md). | Residual: o segredo de integração sai do banco por construção — não está em backup, réplica, exportação nem diff, e rotacioná-lo é um redeploy, não uma migração. **Não** há cofre com criptografia em repouso: um campo `Password` que uma *pessoa* digita continua texto puro na coluna, protegido apenas nas saídas. Quem precisar cifrar em repouso, ou rotacionar credencial guardada, ainda tem trabalho pela frente; para a migração de segredos Frappe continua valendo pedir a chave de criptografia da origem separada do pacote de dados. |

Evidências locais: [tipos do SDK](../packages/sdk/src/types.ts), [permissões](../internal/engine/perm.go),
[autenticação](../internal/engine/auth.go), [fieldtypes](agent/fieldtypes.md).
Para a migração de segredos Frappe, registrar a necessidade da chave de criptografia
da origem, sem colocá-la no pacote de dados comum. Fonte: [Site configuration](https://docs.frappe.io/framework/user/en/basics/site_config).

### 4.2 Dados, modelagem e compatibilidade

| ID | Capacidade | Situação no ddcore | Prioridade e entrega mínima |
|---|---|---|---|
| DAT-01 | Importação com mapeamento, validação e retomada | Não identificado importador genérico. CRUD/MCP, patches e fixtures são primitivas disponíveis. | P0 para o migrador do piloto. Manifesto, simulação, erros por registro, checkpoint e idempotência; P1 para UI de CSV/XLSX. |
| DAT-02 | Exportação completa e reconciliável | **Implementado**: `Ctx.Export` percorre o conjunto por keyset, com tabelas filhas e manifesto de anexos, exposto por `GET /api/export/<DocType>` (streaming, com teto) e por `ddcore export` (NDJSON/CSV, bytes dos anexos, checksums). Contrato em [export](agent/export.md). | Entregue. A permissão `export` passou a ser verificada no servidor; campos `Password` e colunas de credencial nunca saem. Falta apenas a importação correspondente (DAT-01). |
| DAT-03 | Single DocType/configurações | Parcial/incompleto: `isSingle` no SDK e na meta; schema pula a criação da tabela, sem caminho equivalente identificado no Document/bridge. | P1; antecipar se houver Settings no piloto. Persistência, defaults, leitura/escrita, permissões, Desk e testes de singleton. Não anunciar suporte só pela flag. |
| DAT-04 | Evolução de schema e dados com renomes/conversões | **Implementado**: `renamedFrom` renomeia coluna e tabela com índices e referências guardadas; patches têm fases `beforeSchema`/`afterSchema` e `ctx.sql` para preenchimento; conversão de tipo que possa perder dado é recusada até declarar `convert`; as retiradas rodam depois dos patches e `--prune` só derruba o que está vazio. Contrato em [migrations](agent/migrations.md). | Residual: fixtures continuam inserindo ou pulando por nome, sem atualizar; um preenchimento grande segura os locks da transação única — enfileirar e validar no release seguinte; `ddcore_job.args` e `tab_version.data` não são varridos num renome de DocType. |
| DAT-05 | Unicidade composta e chaves de negócio | **Implementado**: `uniqueKeys: [{ name, fields }]` no DocType vira um índice único parcial por chave; o índice leva o nome da chave, então reordenar ou renomear um campo componente não o reconstrói, e retirar a declaração derruba o índice sem `--prune`. Uma linha com qualquer componente nulo ou vazio fica fora da chave, como já ocorre no `unique` por campo. A checagem prévia nomeia o documento em conflito; o `23505` é lido pelo nome da constraint e devolve a mesma frase — é ele que sustenta duas transações concorrentes. Contrato em [fieldtypes](agent/fieldtypes.md) e [migrations](agent/migrations.md); desenho em [specs/2026-09-11-composite-uniqueness.md](superpowers/specs/2026-09-11-composite-uniqueness.md). | Fora de escopo: tabelas filhas, `extendDoctype` e recusa prévia quando o banco já tem duplicatas — nesse caso o `CREATE UNIQUE INDEX` falha com o erro do próprio Postgres e nada é aplicado. Residual: retirar `unique: true` de um *campo* ainda deixa o índice órfão; a varredura cobre só o espaço `uk_`. |
| DAT-06 | Precisão decimal e semântica de datas | **Implementado**: precisão e regra de arredondamento por site, `Currency` arredondado na escrita, mesma regra em Go/goja/desk sobre uma tabela de vetores única, `roundCurrency`/`splitAmount`, e `Datetime`/scheduler no fuso do site. Desenho em [specs/2026-09-10-precisao-decimal-e-datas.md](superpowers/specs/2026-09-10-precisao-decimal-e-datas.md). | Sem API decimal: um double é exato até 2^53 e cobre dinheiro. Restam `SET TIME ZONE` na conexão e os totais/CSV do relatório, ambos registrados no desenho. |
| DAT-07 | Árvores (`is_tree`/NestedSet) e DocTypes virtuais | Não identificados como recursos completos. | P2; antecipar para hierarquias ou entidades externas indispensáveis. Definir consultas, integridade e permissões; Link para si próprio não cobre automaticamente uma árvore. |
| DAT-08 | Tipos e propriedades adicionais de campo | Parcial: conjunto v1 ampliado com Month/Password; Text Editor usa textarea. | P2 por uso: rich text, Attach Image, Table MultiSelect, Duration, Geolocation, Barcode, Rating etc. Mapear conversão e fidelidade dos dados antes de escolher controles. |
| DAT-09 | Custom Fields, Property Setters, Client/Server Scripts | **Implementado**: `extendDoctype` acrescenta campos e altera propriedades de um DocType de outro app em `extensions/<snake>.extend.ts`, com property setters por campo e no DocType, papéis adicionais, `hasPermission`/`permissionQuery` e script de formulário que soma ao do dono. A fusão ocorre antes da validação da meta, então coluna, tipos, API, lista e formulário enxergam um DocType só. Conflitos (campo tomado, dois apps na mesma propriedade, host fora de `requires`) recusam a carga em vez de depender da ordem de instalação. Contrato em [extending](agent/extending.md). | Entregue o caminho versionado — é onde Custom Field e Property Setter do Frappe aterrissam. `fieldname`, `fieldtype`, `options` de Link/Table e a identidade do DocType continuam do dono. Permanece P2 o editor visual de customização persistida; inventariar e converter as customizações de cada origem segue sendo trabalho de migração, não do core. |

Evidências locais: [schema.go](../internal/db/schema.go), [migrate.go](../internal/engine/migrate.go),
[rename.go](../internal/engine/rename.go), [export.go](../internal/engine/export.go),
[extend.go](../internal/meta/extend.go),
[host.go](../internal/engine/host.go), [ListView](../desk/src/lib/components/ListView.svelte),
[ReportView](../desk/src/lib/components/ReportView.svelte), [Control](../desk/src/lib/controls/Control.svelte).
Referências Frappe: [Single DocType](https://docs.frappe.io/framework/user/en/basics/doctypes/single-doctype),
[importação em lote](https://docs.frappe.io/framework/user/en/guides/data/import-large-csv-file)
e [migrações de banco](https://docs.frappe.io/framework/user/en/database-migrations).

### 4.3 Operação diária, automação e integrações

| ID | Capacidade | Situação no ddcore | Prioridade e entrega mínima |
|---|---|---|---|
| OPS-01 | Print Format e PDF | Não identificado motor de impressão/template/PDF. | P1; bloqueia operações que emitem contratos, recibos ou outros documentos. Modelo padrão por DocType, templates de app, prévia, paginação, locale e autorização. |
| OPS-02 | Envio de e-mail e histórico de comunicação | Não identificado envio SMTP/API nativo; `email.go` valida formato de endereço. | P1; transporte configurável, fila persistente, retry, template, anexos autorizados e registro de entrega/falha. IMAP/inbox fica P2. |
| OPS-03 | Notification por evento, data e destinatário | Parcial: scheduler, jobs e SSE; não há regras genéricas nem caixa persistente de notificações. | P1; regras declarativas, destinatários autorizados, lido/não lido e deduplicação. SSE sozinho não entrega notificações a usuários desconectados. |
| OPS-04 | Workflow de aprovação | Parcial: docstatus e métodos de controller; não identificado motor declarativo de estados/transições. | P1; bloqueia aprovações obrigatórias. Estados, ações, papéis, condições, histórico e vínculo com docstatus, validados no servidor. |
| OPS-05 | Atribuição/ToDo, responsáveis e lembretes | Parcial: Comment/Version existem; Task/assignee pertencem ao demo, sem serviço genérico de atribuição. | P1 se a equipe trabalha por pendências. Atribuir/revogar/concluir, vencimento e “minhas pendências”, sem conceder acesso implicitamente. |
| OPS-06 | Webhooks e entrega confiável a terceiros | Parcial: HTTP síncrono + fila; não identificado recurso declarativo de webhook. | P1; bloqueia integrações ativas. Entrega após commit, assinatura, idempotência, timeout, tentativas e registro/reenvio controlado. |
| OPS-07 | Relatórios pesados, preparados e envio agendado | Parcial: Script Reports, filtros, CSV, cards e gráficos. | P2; processamento em job, resultado protegido, expiração, totais e limites explícitos conforme o volume real. |
| OPS-08 | Busca global e visualizações especializadas | Parcial: busca de Link/lista, filtros e navegação; não identificado equivalente completo de busca global, Calendar, Kanban e Gantt. | P2 por fluxo; indexação com autorização e preferências salvas antes de multiplicar visualizações. |
| OPS-09 | Auto Repeat e regras genéricas de atribuição | Parcial: scheduler e serviços permitem implementação no app. | P2; extrair para serviço comum quando dois apps precisarem da mesma semântica. |
| OPS-10 | Portal, Web Forms e acesso de clientes/fornecedores | Parcial: métodos `allowGuest` existem; não identificado produto de portal/web forms. | P2; bloqueia apps que dependam de autosserviço externo. Separar perfis e rotas, validar abuso e anexos, definir publicação explícita. |
| OPS-11 | Compatibilidade de APIs e bibliotecas | Parcial: modelo REST/RPC semelhante; servidor usa Go/goja e bridge próprio. | P0 para consumidores ativos. Mapear paths, verbos, payloads, erros, autenticação e paginação; portar Python/Jinja/SQL e substituir dependências Python/Node incompatíveis. |

Fontes oficiais para o comportamento de referência: [impressão e PDF](https://docs.frappe.io/framework/user/en/desk/printing),
[notificações](https://docs.frappe.io/framework/notifications),
[workflows](https://docs.frappe.io/erpnext/workflows),
[atribuições e ToDos](https://docs.frappe.io/framework/assignments-and-todos)
e [webhooks](https://docs.frappe.io/framework/user/en/guides/integration/webhooks).
A documentação de Workflow está no manual do ERPNext; o requisito aqui é o mecanismo
de aprovação compartilhado. Regras contábeis, fiscais, de estoque e de aluguel pertencem
aos apps de domínio, não ao core.

### 4.4 Produção, recuperação e evolução

| ID | Capacidade | Situação no ddcore | Prioridade e entrega mínima |
|---|---|---|---|
| PRD-01 | Backup e restore de uma instância | Não identificados comandos próprios na CLI. | P0: procedimento automatizado para banco, arquivos públicos/privados, configuração, segredos e versões; restauração ensaiada. CLI pode vir depois. |
| PRD-02 | Deploy, manutenção e rollback | Parcial: `start`, migrações e configuração por instância existem. | P0: artefatos versionados, TLS/proxy, supervisão do processo, pausa de escritas/jobs e roteiro de retorno compatível com o banco. |
| PRD-03 | Observabilidade e saúde operacional | **Implementado**: liveness (`/healthz`, `/api/health`) separado de readiness (`/readyz`, `/api/ready`), este provando o banco com deadline e respondendo **503**; corpo booleano para anônimo e relatório com fila, Error Log e pool em `/api/health/report` sob System Manager. As sondas ignoram chave de API vencida, por igualdade exata de caminho. Todo request ganha `X-Request-Id` — sanitizado na entrada, ecoado, repetido como `requestId` no envelope de erro, gravado na coluna `request_id` do `Error Log` e legível pela app em `ddcore.session.requestId`. Log de acesso com duração, filtrado por nível, e `DDCORE_LOG_FORMAT=json`. Pânico passou a produzir envelope JSON e linha de `Error Log` com pilha, no lugar do 500 pelado do chi. Sinais de fila distinguem `runnable` de agendado e contam lease vencido. Limiares no bloco `ops` do `ddcore.json`. `ddcore doctor [--json] [--strict]` sonda o banco antes do engine, relata em vez de morrer quando ele está fora do ar, redige o DSN e sai com código != 0 no crítico. Contrato em [operação](agent/ops.md). | Entregue. Residual: alertas de backup dependem de PRD-01/02 (nada produz evento de backup ainda); administração de jobs — retry, cancelamento, retenção — é PRD-04; sem `/metrics` Prometheus ou OpenTelemetry; nada persiste última execução do scheduler, então `entries` diz o que este build instalaria e não que há cron vivo; `ddcore_job` não ganhou `request_id` (é schema do PRD-04) e o handle por execução vive só na linha de `Error Log`. |
| PRD-04 | Administração de jobs | Parcial: CLI, retry, timeout, lease e heartbeat. | P1: inspeção de falhas, reenvio/cancelamento autorizado, retenção e métricas. Testar tarefas externas repetidas e desligamento durante execução. |
| PRD-05 | Ciclo de vida de arquivos | Parcial: File, upload, caminhos públicos/privados. | P0: copiar bytes, validar checksums, preservar vínculos e acesso. P2: armazenamento S3 compatível, quotas e políticas de retenção segundo demanda. |
| PRD-06 | Auditoria além de Version | Parcial: mudanças e comentários por documento. | P1; bloqueia quem depende de trilha específica. Registrar alterações de permissões, importações, aprovações e ações administrativas, protegendo dados sensíveis. |
| PRD-07 | Contrato de compatibilidade core/apps | Parcial: `requires`, versão no manifesto, SDK gerado e testes. | P1: faixa de versões suportadas, changelog, verificação de atualização e testes de consumidores; fixar versões no piloto desde P0. |
| PRD-08 | Multisite e múltiplas réplicas | Um tenant por instância é decisão da v1; eventos/cache locais fazem parte da arquitetura. | P2. Primeiro automatizar instâncias isoladas. Antes de várias réplicas do mesmo tenant, validar scheduler, cache, eventos e distribuição dos jobs. |

Evidências locais: [CLI](../cmd/ddcore/main.go), [doctor](../cmd/ddcore/doctor.go),
[configuração](../internal/config/config.go), [API](../internal/api/api.go),
[sondas](../internal/api/health.go), [correlação](../internal/api/observe.go),
[saúde](../internal/engine/health.go), [probe do banco](../internal/db/health.go),
[jobs](../internal/engine/jobs.go) e [hub](../internal/engine/hub.go).
Referência operacional Frappe: [Bench commands](https://docs.frappe.io/framework/user/en/bench/resources/bench-commands-cheatsheet).

## 5. Critérios de aceite dos componentes prioritários

Os detalhes abaixo são requisitos propostos para o ddcore; não representam garantia
de que o Frappe ou a v1 já os atendam em todos os cenários.

| Entrega | Evidência mínima para aceitar |
|---|---|
| Permissões granulares | Dois usuários com o mesmo papel e escopos diferentes não acessam documentos/campos um do outro por URL, REST, busca, relatório, exportação, histórico, arquivos ou SSE. Testar também contextos administrativos e jobs explicitamente privilegiados. |
| Importador | Duas execuções do mesmo lote mantêm contagens, nomes e totais; interrupção e retomada não duplicam filhos nem efeitos externos; erros apontam origem, campo e motivo. |
| Single/Settings | Uma configuração por DocType/instância; salvar, recarregar, consultar via API, aplicar defaults e negar acesso funcionam sem tabela de documento ausente. |
| Segredos | Leitura comum/API/export/Version/log nunca revela o valor; recuperar e rotacionar o segredo continua funcionando após restore. |
| Impressão | Documento longo com filhos, acentos, valores, fuso e múltiplas páginas produz saída legível; download nega usuário sem permissão e não inclui campo restrito. |
| E-mail/notificações/webhooks | Rollback não envia; queda do worker permite retomada; tentativas usam chave estável e ficam auditadas. SMTP não promete exatamente uma entrega: registrar a possibilidade de duplicação quando houver resultado incerto. |
| Workflow | Alterar diretamente o campo de estado ou chamar submit pela API não contorna aprovação; duas aprovações simultâneas não executam o efeito duas vezes. |
| Backup/retorno | Restaurar em instância isolada recupera banco, anexos e configuração; login e fluxo crítico funcionam dentro do tempo de recuperação definido. |

## 6. Roteiro de migração de um app/site

### Etapa A — descobrir a origem e fixar o contrato

Escolher um app e um fluxo com início, término e efeito operacional verificável.
Para `alugueis`, candidato a confirmar no repositório externo: cadastro → contrato →
faturamento → baixa → recibo → relatório. Usar cópia isolada dos dados de origem.

Preencher um manifesto por site, com:

- versões/commits do Frappe e de cada app, banco, volumes e taxa de alteração;
- DocTypes padrão/customizados, singles, árvores, filhos, contagens e tamanho de anexos;
- metadados efetivos, incluindo Custom Fields, Property Setters, permissões e defaults;
- controllers, hooks, scripts, patches, fixtures, relatórios, formatos de impressão e SQL;
- usuários/papéis/escopos, compartilhamentos, workflow, atribuições e documentos privados;
- scheduler, jobs pendentes, integrações de entrada/saída e consumidores de API;
- idioma, fuso, precisão monetária, valores de Select e política de histórico;
- responsável por homologar cada fluxo, indisponibilidade tolerada, recuperação e retorno.

Para cada dependência, registrar: recurso Frappe → evidência de uso → ID deste
inventário → solução ddcore → teste de equivalência → responsável → bloqueia virada?
Uma API parecida ou um mesmo nome de fieldtype não bastam para marcar equivalência.

### Etapa B — portar comportamento e preparar o destino

Portar regras e testes do app para TS síncrono; portar scripts de tela para o SDK do
Desk. Identificar diferenças na ordem dos hooks, `dbSet`, permissões, transações,
consultas SQL e bibliotecas. Adaptar integrações por contrato ou por uma camada
pequena de compatibilidade quando houver consumidor que não possa mudar junto.

Provisionar uma instância isolada com as versões fixadas do core e do app. Usar
`migrate`/MCP para schema e as ferramentas suportadas para dados. `ddcore migrate`
atualiza o schema do destino; não converte automaticamente um banco Frappe.

### Etapa C — construir a carga e reconciliar

1. Extrair os dados com paginação estável e um snapshot consistente ou janela de
   congelamento. Salvar manifesto e checksums dos arquivos extraídos.
2. Definir uma tabela de mapeamento de DocType, campo, nome, tipo e valor. Preservar
   `name` quando possível; se mudar, remapear todo Link/Dynamic Link e referências.
3. Carregar identidades e configurações; depois cadastros de referência e documentos
   na ordem de dependência, com seus filhos (`parent`, `parenttype`, `parentfield`, `idx`).
   Dependências cíclicas exigem resolução em fases e validação final explícita.
4. Preservar significado de `owner`, `creation`, `modified`, `modified_by`, `docstatus`
   e `amended_from`. Se o CRUD normal os reescreve, criar mecanismo de migração restrito,
   com validação e auditoria, em vez de pressupor que `insert_doc` conserva tudo.
5. Tratar histórico submetido/cancelado sem refazer cobranças, lançamentos ou envios.
   Escolher por DocType entre replay controlado e restauração de estado histórico;
   em ambos, conferir integridade e registrar quais efeitos foram suprimidos.
6. Migrar anexos e suas permissões; comparar checksums, vínculos e quantidade de bytes.
   Importar ou arquivar com acesso definido Version, Comment, Communication e outras
   trilhas que precisem continuar consultáveis, preservando a origem do evento.
7. Mapear Selects localizados para inglês canônico e gerar catálogos. Converter datas
   civis e instantes com regra explícita; não deslocar Date por timezone.
8. Definir acesso inicial dos usuários. O verificador atual aceita seu formato Argon2id;
   não pressupor que hashes do Frappe sejam compatíveis. Usar redefinição/convite ou
   compatibilidade temporária testada; reemitir API keys e não importar sessões ativas.
9. Reconciliar registros, filhos, links órfãos, anexos, status, totais e relatórios em
   datas históricas. Para valores monetários, definir arredondamento e tolerância por
   indicador; não aceitar apenas comparação visual ou quantidade total de linhas.

Critério de saída: todas as divergências explicadas e aprovadas, zero referência
inválida e nenhuma duplicação após reexecução. Guardar origem, transformação e destino
de cada lote para permitir investigação posterior.

### Etapa D — ensaiar a operação e o retorno

Executar o roteiro completo pelo menos duas vezes em ambiente isolado: uma carga
inicial e uma repetição com interrupção/retomada. Medir tempo, recursos e janela de
indisponibilidade com volume representativo. Homologar por perfil de usuário.

Comparar resultados do Frappe e ddcore usando a mesma referência temporal. Durante o
ensaio, impedir que o destino envie cobranças, e-mails ou webhooks reais. A comparação
pode ocorrer em paralelo, mas deve haver um único sistema responsável pelas escritas
operacionais; sincronização bidirecional seria um projeto adicional.

Restaurar o backup em uma terceira instância e executar o fluxo crítico. Documentar
o ponto de decisão de retorno, responsáveis e tratamento das escritas realizadas
depois da virada: o backup anterior sozinho não contém essas novas operações.

### Etapa E — virar e acompanhar

Congelar escritas na origem, pausar scheduler/consumidores externos, capturar a carga
final ou delta (incluindo exclusões/cancelamentos) e reconciliar novamente. Liberar
acesso e integrações no destino só após os critérios acordados.

Manter o Frappe anterior em consulta restrita durante o período definido. Monitorar
erros, fila, notificações, latência e reconciliação diária. Desativar a origem somente
após aceite operacional e confirmação da recuperação do destino.

## 7. Sequência sugerida de entregas

| Marco | Escopo | Dependências e condição de saída |
|---|---|---|
| M0 — diagnóstico do piloto | Manifesto de origem, diferenças de contrato e prioridades aplicáveis | Todos os fluxos críticos têm dono e critério de equivalência; versões fixadas. |
| M1 — migração e recuperação | DAT-01/02/04/06/09, OPS-11, PRD-01/02/03/05 e requisitos de identidade | Carga repetível/reconciliada, backup restaurado, operação básica e acesso inicial demonstrados. |
| M2 — capacidades exigidas pelo piloto | SEC-01/02/03/06, DAT-03/05 e OPS-01…06 conforme uso | Nenhuma permissão, aprovação, documento emitido ou integração obrigatória sem equivalente. Pode evoluir em paralelo com M1 depois de M0. |
| M3 — piloto em produção | Ensaio final, treinamento, virada e acompanhamento | Fluxos homologados, metas operacionais atendidas e retorno documentado/ensaiado. |
| M4 — ampliar a migração | Segundo app com necessidades diferentes, PRD-07 e P2 comprovados | APIs genéricas reutilizadas, atualização testada e ausência de regras de domínio vazando para o core. |

Não atrelar a virada à conclusão de todos os itens P1/P2. O gate é o conjunto de
requisitos obrigatórios do app escolhido. Da mesma forma, não dispensar uma
funcionalidade indispensável apenas porque sua prioridade geral é P2.

## 8. O que preservar como decisão de produto

- Um tenant por instância; automação de provisionamento antes de multisite no core.
- Postgres como serviço externo obrigatório; fila, outbox e notificações podem usar
  a infraestrutura já existente. Provedores de e-mail são integrações configuráveis.
- Metadados de estrutura em arquivos. Configurações e estado operacional podem ser
  dados; editor visual de estrutura precisaria de fluxo explícito de exportação/versionamento.
- TypeScript síncrono no servidor e ausência de dependência Node/Python para o app
  em produção. Portabilidade de bibliotecas deve ser avaliada, não presumida.
- Impressão exige decisão própria: um renderizador HTML→PDF externo adiciona dependência
  de implantação. Avaliar biblioteca Go, navegador opcional ou serviço, com fidelidade e
  custo demonstrados, antes de prometer simultaneamente PDF complexo e binário isolado.
- Internacionalização e regras garantidas no servidor continuam contratos do core.
  A migração não deve recolocar traduções em valores armazenados nem validação só na tela.

## 9. Validação deste inventário

Verificação documental: links locais, coerência entre estado atual e proposta, evidências
de código e fontes oficiais. Consulta MCP somente de metadados; nenhuma migração de
dados de produto foi executada para produzir o inventário.

A validação do repositório deve incluir `make test` conforme [AGENTS.md](../AGENTS.md).
`make check` complementa o gate com o TypeScript do demo e o catálogo de traduções;
atualmente não é dependência de `make test` no Makefile. A execução desses comandos
valida o checkout, não comprova as funcionalidades futuras listadas aqui.

Resultados obtidos nesta análise:

| Verificação | Resultado |
|---|---|
| Links locais dos dois documentos e `git diff --check` | Sem links quebrados ou problemas de whitespace. |
| `make test` | Reprovado: build, vet e etapa Go concluídos; demo com 25 testes, 2 falhas. O alvo interrompe antes dos testes do Desk. |
| `make check` | Aprovado: tipos do Desk/demo e catálogos sem traduções ausentes; um aviso preexistente de `autofocus` no login. |
| `make test-desk`, executado separadamente | Aprovado: 79 testes em 10 arquivos. |

As duas falhas do demo são incompatibilidades entre a expectativa textual do teste e
o erro recebido: [project.test.ts](../apps/demo/doctypes/project/project.test.ts)
procura `final`, enquanto o controller retorna `The end date cannot be earlier than
the start date.`; [task.test.ts](../apps/demo/doctypes/task/task.test.ts) procura
`limite`, enquanto recebe `The due date cannot be earlier than the project start (…)`.
As validações rejeitaram as datas inválidas; a comparação de mensagem falhou.
São arquivos preexistentes, sem alterações nesta entrega documental. Alinhar os
testes ao contrato de erros/idioma e obter `make test` verde antes do piloto.

Resolvido depois: as duas expectativas passaram a casar o texto em inglês que os
controllers lançam, e `make test` fecha verde. A tabela acima permanece como registro
do que foi medido em 10/09/2026, não do estado atual.
