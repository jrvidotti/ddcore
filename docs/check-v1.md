# Revisão da implementação v1

Data: 10/09/2026. Referência: `docs/plan-v1.md`. Commit revisado: `f54ea30`.

> Registro histórico: nesta revisão, o app `alugueis` ainda residia em
> `apps/alugueis`. Ele foi posteriormente extraído para um repositório independente.

## Parecer

A implementação cobre boa parte da arquitetura e das funcionalidades previstas, mas **a v1 ainda não atende integralmente ao plano nem está pronta para exposição a usuários reais**. Há falhas críticas de autorização, perda de atualização concorrente, resultados financeiros incorretos e requisitos incompletos. Os testes existentes passam, porém não cobrem esses cenários.

Esta revisão produziu somente este relatório. As reproduções adicionais foram feitas com código temporário, removido ao concluir, usando um schema descartável no banco `ddcore_test`. Não foram aplicadas correções à implementação nem usadas as tabelas de `ddcore_dev` para as reproduções.

Prioridades: **P0** = corrigir antes de expor o serviço; **P1** = bloqueia confiabilidade/aceite da v1; **P2** = funcionalidade incompleta ou defeito de menor alcance. “Reproduzido” indica execução observada; “inspeção” indica causa identificada no código, sem teste integrado daquele cenário.

## Verificações executadas

| Verificação | Resultado |
|---|---|
| `go test -count=1 -v ./internal/...`, antes dos probes temporários | Passou: lifecycle com Postgres, bundle/runtime e meta. Vários pacotes não possuem testes. |
| Migração do core + alugueis em schema vazio | Passou, com 20 DocTypes carregados. |
| Planejamento de DDL imediatamente após migrar | Nenhuma instrução pendente. |
| Suíte TS via `Engine.RunTests`, mesma API usada por `ddcore test` | **57 testes, nenhuma falha**. Executada no schema isolado após migração. |
| `npm run check`, em `desk/` | **Falhou: 10 erros e 7 avisos** em 11 arquivos. |
| `npm run build`, em `desk/` | Passou, com avisos de acessibilidade. O build não substitui a checagem de tipos. |
| `go vet ./...` | Passou. |
| Compilação do CLI atual para `/private/tmp/ddcore-check-v1` | Binário gerado. |
| `tsc --noEmit -p apps/alugueis/tsconfig.json`, com o TypeScript instalado no Desk | Falhou: `ignoreDeprecations: "6.0"` é inválido no TypeScript 5.9.3 instalado. |
| Mesmo comando com `--ignoreDeprecations 5.0`, sem editar configuração | Revelou mais **5 erros de tipos** no app. |
| Probes adicionais usando `httptest`, engine, runtime e Postgres | Confirmaram os casos indicados abaixo, incluindo MCP sem autenticação, edição de filhos como Guest, CTE de escrita e perda de atualização. |

A primeira execução Go dentro do sandbox pulou o lifecycle por falta de acesso ao Postgres. A execução seguinte, com acesso autorizado, realmente executou e passou esse teste; o resultado com `SKIP` não foi usado como evidência de aprovação.

Limites: não foi executado o fluxo visual completo de login → cadastros → pagamento no navegador; não foi executada a cadeia inteira de scaffold → migrate → insert → tests via cliente MCP; não houve teste de carga, interrupção real de workers ou integração externa real com ViaCEP. A migração em schema vazio valida o mecanismo, mas não equivale ao roteiro literal de bootstrap em checkout limpo.

## Aderência ao plano

| Área do plano | Situação | Evidência / diferença |
|---|---|---|
| Stack e distribuição | Implementada | Go, goja, esbuild, pgx/Postgres e Desk Svelte 5 com adapter-static e embed. |
| Layout do core | Adaptado | Document, permissões, reports e jobs estão concentrados em `internal/engine`, em vez dos pacotes separados do plano. É uma decisão organizacional, não um bug por si só. |
| Meta e fieldtypes | Parcial | Tipos e propriedades principais existem; expressões de somente leitura não são impostas no servidor; `listSettings` não consta da meta Go. |
| Migrações | Parcial | Criação, alteração de tipo, prune, histórico, patches e instalação existem; idempotência passou no app atual, mas há falhas nos índices únicos. |
| Document e lifecycle | Parcial | CRUD, naming, submit/cancel/amend, fetchFrom, filhos e versões existem. Falham concorrência, isolamento de filhos e proteção do histórico. |
| Permissões e autenticação | Incompleta | Argon2id, sessão, API key, roles e hooks existem. Há caminhos HTTP que ignoram autorização. |
| API | Parcial | Principais endpoints presentes. Não há ETag na meta; count com busca é inconsistente; SSE e endpoints de workspace precisam de autorização. |
| Desk | Parcial | Rotas, list/form, grid, dialogs, scripts e sidebar explícita existem. Faltam filtro avançado e gráfico de linha; há erros de tipos e defeitos de ciclo de vida/datas. |
| Reports e workspace | Parcial | Três relatórios e workspace presentes; há erros de posição histórica e recebimento mensal. |
| Jobs/scheduler | Parcial | Fila Postgres, SKIP LOCKED, retry e cron presentes; timeout e recuperação de jobs interrompidos ausentes. |
| CLI/MCP/docs | Parcial | Grande parte dos comandos/tools/resources existe. `ddcore demo` não existe; flags documentadas após posicionais não são interpretadas corretamente; eval não transpila TS. |
| Testes portados | Parcial | Sete grupos de domínio foram portados, mas faltam equivalentes de instalação e tradução. |

Dos cinco critérios finais de pronto: **(1)** mecanismo de migração validado parcialmente, com problema no roteiro literal de `init`; **(2)** 57 testes passam, mas a cobertura portável ainda está incompleta; **(3)** fluxo visual não certificado; **(4)** tools presentes e chamada MCP exercitada, mas cadeia completa não certificada e autenticação reprovada; **(5)** mandatory no servidor, sidebar explícita e scheduler no boot estão presentes, com ressalvas de idioma e testes de interface descritas abaixo.

## Achados

### B01 — P0 — MCP HTTP permite executar ferramentas administrativas sem autenticação

**Reproduzido.** `cmd/ddcore/main.go:204` monta `/mcp` diretamente no router. O middleware `internal/api/api.go:158` transforma ausência de credenciais em `Guest`, sem bloquear a rota. As tools executam por `internal/mcp/mcp.go:489`, que sempre usa `Administrator`.

Uma requisição HTTP sem cookie e sem Authorization, com o handler montado como em `ddcore dev`, executou `tools/call` → `sql_query` e retornou HTTP 200 com o usuário do banco. Payload de reprodução não destrutivo:

```json
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sql_query","arguments":{"query":"SELECT current_user AS db_user"}}}
```

O mesmo servidor registra tools de escrita, eval e scaffold. O alcance é qualquer cliente que consiga acessar a porta do servidor de desenvolvimento, que escuta em `:porta`. Isso contradiz explicitamente o MCP HTTP “com API key” do plano.

**Correção sugerida:** exigir API key válida e autorização administrativa antes do handler MCP. Uma chave de usuário comum não deve resultar automaticamente em autoridade de Administrator. Testar ausência de credenciais, sessão comum, chave inválida e chave sem papel adequado.

### B02 — P0 — Guest consegue ler e alterar documentos filhos diretamente

**Reproduzido.** `internal/engine/perm.go:57` retorna `true` incondicionalmente para `isChild`. A API permite GET/PUT/DELETE pelo nome do DocType filho, e `Save` não exige autorização no documento pai.

Usando o identificador conhecido de uma linha `Has Role` do Administrator, GET e PUT em `/api/resource/Has%20Role/<id>` sem autenticação retornaram HTTP 200. O PUT mudou `role` e persistiu `modified_by: "Guest"`.

Isso permite manipular linhas protegidas sem passar pelo lifecycle do pai, inclusive papéis de usuários e dados de baixas. A reprodução utilizou um ID conhecido; não se deve confundir esse resultado com uma prova de enumeração irrestrita pela listagem, que tem filtros adicionais.

**Correção sugerida:** impedir CRUD avulso de filhos ou resolver e validar o pai, sua permissão, seu docstatus e o campo Table correspondente. Alterações de papéis precisam também passar pelas validações e invalidação de cache do User.

### B03 — P1 — Salvar um pai pode transferir silenciosamente um filho de outro documento

**Reproduzido.** `internal/engine/doc.go:1057` faz upsert de filhos por `name`, atualizando `parent`, `parenttype` e `parentfield`, sem conferir a associação anterior.

Criei usuários A e B, copiei para B uma linha de `roles` de A mantendo o `name` e salvei B. Resultado: **A ficou com zero linhas e B com uma**. A gravação não recusou o ID estrangeiro nem criou uma cópia.

**Correção sugerida:** validar a propriedade de cada ID antes do upsert. Linhas novas devem receber IDs novos; linhas existentes só podem pertencer ao mesmo pai/campo. Testar isso também com baixas e anexos.

### B04 — P1 — Histórico vaza documentos restritos e hashes de senha

**Reproduzido para Version; inspeção para Comment.** `core/doctypes/version/version.doctype.ts` concede leitura a `All`. `internal/api/api.go:556` consulta versões sem verificar leitura do documento referenciado. `internal/engine/doc.go:1112` grava o diff de todos os campos, inclusive `User.password_hash`.

Após trocar uma senha no schema de teste, um contexto de usuário sem papel administrativo recebeu PermissionError ao ler `User/Administrator`, mas conseguiu listar sua Version, cujo JSON continha `password_hash`.

Comentários têm problema semelhante: a meta permite leitura/escrita/exclusão a `All`, sem restrição por autor ou permissão do documento associado, e o endpoint de comentários também não verifica o pai.

**Correção sugerida:** vincular a autorização de Version/Comment ao documento referenciado, inclusive via API genérica; excluir segredos do versionamento e das respostas; restringir alteração/exclusão de comentários conforme autoria/papel. Avaliar saneamento dos históricos já gerados.

### B05 — P1 — Gráfico financeiro do workspace é público

**Reproduzido.** `internal/api/api.go:634` chama `rt.Chart` sem conferir as roles do workspace. O gráfico em `apps/alugueis/workspaces/alugueis.workspace.ts` usa SQL direto sobre `tab_lancamento`, sem filtro de permissão.

GET anônimo em `/api/workspace/Alugueis/chart/receita_mensal` retornou HTTP 200 com a série financeira. O schema de teste estava sem movimentos; o caminho executado é o mesmo que agrega os valores quando há dados. O filtro aplicado ao boot/sidebar não protege esse endpoint. `numberCard` também precisa validar acesso ao workspace antes de executar métodos.

**Correção sugerida:** autorizar workspace e dados consultados no servidor, não apenas ocultar links no boot.

### B06 — P1 — SQL “somente leitura” aceita escrita

**Reproduzido.** `internal/engine/query.go:275` verifica apenas se o texto começa com `select` ou `with`. Foi aceita, executada e revertida pela transação de teste esta consulta:

```sql
WITH changed AS (
  UPDATE tab_role SET modified_by = 'audit'
  WHERE name = 'System Manager' RETURNING name
)
SELECT * FROM changed;
```

O retorno continha a linha modificada. A tool `sql_query` usa essa mesma função dentro de uma transação que normalmente faz commit; o contrato readonly está quebrado independentemente do problema de autenticação do MCP.

**Correção sugerida:** impor somente leitura no banco/conexão de execução dessas consultas. Um teste por prefixo não assegura essa propriedade. Preservar explicitamente a semântica transacional necessária para reports que consultam dados da operação atual.

### B07 — P1 — Concorrência otimista permite perda de atualização

**Reproduzido com duas transações.** `internal/engine/doc.go:304` compara `modified` após um SELECT, mas `writeUpdate`, em `:1034`, usa somente `WHERE name = ...`. Não há compare-and-swap nem lock antes da comparação.

A primeira transação mudou `full_name` de um User para `first` e manteve o UPDATE sem commit. A segunda leu a versão anterior e tentou salvar apenas uma mudança em `language`; confirmei que aguardava um lock no Postgres. Após liberar a primeira, ambas foram aceitas e o resultado final foi **`full_name=original`, `language=en`**: a primeira edição se perdeu.

Além disso, `sameTime`, em `:411`, aceita timestamps inválidos como equivalentes e reduz a precisão a milissegundos.

**Correção sugerida:** comparar e gravar atomicamente pelo timestamp/version token, ou bloquear e reler antes de validar. Timestamp inválido deve ser recusado. Cobrir gravações simultâneas, submit e registro de pagamentos.

### B08 — P1 — Hot-reload pode devolver runtime antigo ao pool novo

**Inspeção.** `internal/engine/engine.go:118` substitui `e.Pool`, meta e snapshot. Um contexto que adquiriu runtime antes do reload o devolve em `release`, `:304`, usando **o pool atual**, e não o pool de origem. Assim, o pool novo pode receber uma VM com o bundle antigo.

O mutex usado para substituir os campos não é adquirido pelos leitores habituais de `Meta`, `Snap`, `Pool` e `whitelisted`; há também risco de data races entre requisições e reload. O scheduler é criado uma vez em `cmdServe`, de modo que recarregar a meta não reinstala suas entradas.

**Correção sugerida:** usar snapshots consistentes por requisição, guardar a geração/pool que forneceu a VM e sincronizar a troca; reconstruir o scheduler quando sua definição mudar. Validar com requisições durante reload e detector de races.

### B09 — P1 — Apuração histórica incorpora pagamentos posteriores à referência

**Reproduzido.** `apps/alugueis/services/cobranca.ts:138` percorre todas as baixas antes de aplicar `opts.ate`; não filtra as que ocorreram depois da data consultada.

Lançamento de R$ 100, vencimento 01/01/2026, pagamento de R$ 50 em 01/02/2026, consulta em **15/01/2026**: retornou recebido R$ 50, saldo R$ 50 e `apurado_em=2026-02-01`. Para aquela referência, deveria indicar recebido zero e principal aberto R$ 100, sem encargos neste exemplo.

O relatório `recebimentos_em_atraso.report.ts` agrava o problema ao selecionar pelo status atual: um título pago hoje pode desaparecer da posição histórica de quando ainda estava aberto.

**Correção sugerida:** limitar o replay à referência e selecionar candidatos ao relatório pela situação apurada nessa data. Testar baixas futuras, quitação posterior e abatimento posterior.

### B10 — P1 — “Recebido no Mês” soma valores recebidos em outros meses

**Reproduzido.** `apps/alugueis/workspaces/alugueis.workspace.ts:40` seleciona lançamentos pela data da última baixa e soma seu `valor_pago` acumulado.

Um lançamento com R$ 50 pagos no mês anterior e R$ 50 no mês atual produziu **R$ 100 no card**, quando o movimento do mês é R$ 50.

**Correção sugerida:** agregar as linhas de pagamento pela data da própria baixa, com os filtros de receita e autorização do pai. Não somar abatimentos como recebimento.

### B11 — P1 — Mesmo reajuste pode ser aplicado novamente

**Reproduzido.** `apps/alugueis/services/reajuste.ts:62` sempre acrescenta uma linha e aplica o percentual ao valor atual, sem bloquear uma data/janela já processada.

Com aluguel de R$ 2.000 e índice de 10%, aplicar a mesma data, recarregar e aplicar novamente resultou em **R$ 2.420 e duas linhas de reajuste**, em vez de rejeitar a segunda operação. A função também não exige explicitamente contrato enviado antes de iniciar a operação.

**Correção sugerida:** validar estado, cronologia e unicidade do período de reajuste, inclusive sob concorrência; repetição deve ser idempotente ou retornar erro claro.

### B12 — P1 — Idempotência do faturamento não é garantida sob concorrência

**Inspeção.** `apps/alugueis/services/faturamento.ts:48` faz “consulta se existe → insert”. Não há restrição composta no banco nem lock por contrato/competência. Duas chamadas podem passar pela consulta antes de qualquer uma inserir; o contador de nomes gera nomes distintos, mas não protege a chave de negócio.

**Correção sugerida:** serializar a operação por chave de negócio ou criar unicidade para `(contrato, categoria, competencia)` nos lançamentos relevantes, tratando cancelamento e repetição. Acrescentar teste de duas gerações simultâneas.

### B13 — P2 — Renovação automática não mantém o faturamento após o término

**Reproduzido para faturamento; inconsistência também visível no scheduler.** `services/contratos.ts:12` pula contratos com renovação automática porque deveriam permanecer vigentes, mas `services/faturamento.ts:32` e a seleção em `gerarCompetenciaTodos` continuam limitadas a `data_final`.

Um contrato com renovação automática e término anterior à competência gerou **zero lançamentos**. Ao submeter um contrato já vencido, o controller ainda o marca como Encerrado, mesmo com essa opção.

**Correção sugerida:** definir a regra de prorrogação e aplicá-la de forma consistente ao estado, seleção para faturamento, conflitos de ocupação e relatórios.

### B14 — P1 — Índices únicos não funcionam corretamente para tipos e alterações de meta

**Reproduzido para Currency; inspeção para mudança de índice.** `internal/db/schema.go:94` usa `campo <> ''` para qualquer campo único. Marcar `Imovel.valor_aluguel` como unique e migrar falhou com **SQLSTATE 22P02: invalid input syntax for type numeric: ""**.

O diff consulta apenas nomes de índices (`Plan`, `:105`), sem comparar definições. Um campo que já tinha índice de busca/Link e passa a unique conserva o índice não único, pois ambos recebem o mesmo nome. O inverso também mantém restrição antiga.

**Correção sugerida:** gerar predicados compatíveis com o tipo e comparar definição/unicidade dos índices existentes. Testar numeric/date/check e transições searchIndex ↔ unique.

### B15 — P1 — Jobs não têm timeout nem recuperação de execução interrompida

**Inspeção.** `internal/engine/jobs.go:76` confirma `status='running'` antes de executar o método. Se o processo cair depois disso, o job permanece running: a seleção só considera queued. Também não existe deadline por job nem interrupção da VM para código JS que não retorna; `internal/js/runtime.go` não conecta cancelamento à execução goja.

**Correção sugerida:** implementar timeout real de execução, lease/heartbeat ou recuperação de jobs abandonados, e política explícita para reexecução. Cobrir queda após claim, timeout de JS e erro ao persistir o resultado.

### B16 — P1 — Typecheck do Desk e do app está reprovado e fora do fluxo de testes

**Reproduzido.** `desk/package.json` possui `check`, mas `Makefile` não o executa em build/test. O build transpila com sucesso apesar dos erros.

No Desk: dois `onMount(async () => ... cleanup)` incompatíveis, destructuring de `unknown` em `DocSidebar.svelte:56`, acesso a `frm` possivelmente nulo e seis erros de parâmetros opcionais das rotas. No app, além de `ignoreDeprecations`, há três incompatibilidades de `BaixaLancamento.data` anulável com `Baixa.data`, uma asserção de status incompatível e um documento sem tipo de Contrato em `services/demo.ts:43`.

**Correção sugerida:** alinhar a versão de TypeScript, corrigir os contratos de tipos e tornar `svelte-check` e `tsc` gates da validação. Não considerar apenas o build como aprovação da tipagem prometida pelo plano.

### B17 — P2 — Listas e formulários deixam assinaturas SSE ativas após desmontar

**Inspeção, também apontada pelo typecheck.** `desk/src/lib/components/FormView.svelte:26` e `ListView.svelte:63` retornam cleanup de dentro de callback async de `onMount`. O retorno imediato é uma Promise; o cleanup não é registrado como função de desmontagem.

Navegações acumulam handlers que retêm formulários/listas antigos, podendo causar consultas duplicadas e crescimento de memória.

**Correção sugerida:** registrar cleanup sincronamente e executar a inicialização assíncrona separadamente, tratando desmontagem durante o carregamento e cancelando timers pendentes.

### B18 — P2 — Datas e horas divergem entre cliente e servidor

**Reproduzido para conversão e addMonths.** `desk/src/lib/controls/Control.svelte:34` exibe UTC em `datetime-local`, mas ao gravar interpreta esse texto como horário local. No fuso America/Cuiaba, um valor de 12h local é mostrado como 16h e pode voltar como 20h UTC, deslocando quatro horas.

`desk/src/lib/desk-sdk.ts` calcula `today()` em UTC e usa `setUTCMonth` sem limitar o dia ao fim do mês: **31/01/2026 + 1 mês = 03/03/2026**, enquanto o runtime do servidor/teste Go usa 28/02. O calendário nativo de `input type=date` também depende do ambiente do navegador, não só do catálogo de idioma do site.

**Correção sugerida:** definir a política de timezone do site e compartilhar a semântica de datas civis; usar conversão local correta no control e limitar dias em addMonths. Testar virada de dia/mês e usuários com locale diferente.

### B19 — P2 — Busca e filtros em filhos geram contagens/paginação incorretas

**Inspeção.** `internal/api/api.go:348` calcula count usando somente `filters`, descartando `or_filters` da busca enviada por `ListView.svelte:57`. A lista filtrada pode mostrar o total global e oferecer páginas vazias.

`internal/engine/query.go:50` usa JOIN para filtros em filhos, sem deduplicação automática do pai. Se duas linhas filhas correspondem ao filtro, o mesmo documento aparece duas vezes e `count(*)` conta linhas do join.

**Correção sugerida:** compartilhar todas as condições entre listagem e contagem; usar EXISTS ou deduplicação adequada para filtros de filhos; validar vínculo do DocType filho ao pai e ao campo Table.

### B20 — P2 — Eventos não respeitam integralmente transações nem permissões

**Inspeção.** `DBSet` (`internal/engine/doc.go:498`), Delete e Rename publicam antes do commit; uma transação revertida pode gerar evento de alteração que não aconteceu. `Ctx.Run` (`internal/engine/engine.go:268`) também executa callbacks `afterCommit` quando o caminho escolhido foi rollback explícito e este retornou sem erro.

`/api/events` aceita Guest, e `internal/engine/hub.go` transmite eventos sem destinatário a todos, sem autorização por documento. Assim, identificadores e atividade de documentos privados podem ser divulgados.

**Correção sugerida:** publicar exclusivamente após commit real, descartar callbacks em rollback/savepoints e filtrar assinantes pela permissão do evento/documento. Atualizações via DBSet também precisam levar os dados esperados pelo consumidor e notificar listas quando aplicável.

### B21 — P2 — dbSet devolve documento com timestamp desatualizado

**Observado durante a reprodução de reajuste e confirmado por inspeção.** `internal/js/prelude.js:160` atualiza apenas os valores passados a `dbSet`, enquanto o bridge Go altera também `modified`. Depois de reajustar, um `save()` na mesma instância pode falhar com TimestampMismatch sem outra pessoa ter editado o documento.

No Desk, `frm.call` carrega `res.doc` (`desk/src/lib/form.svelte.ts`), em vez de obrigatoriamente buscar a versão persistida. O resultado de `aplicarReajuste` pode, portanto, deixar o formulário com esse timestamp antigo. Recarregar o documento antes da segunda aplicação eliminou o falso conflito e permitiu reproduzir B11.

**Correção sugerida:** sincronizar timestamp/estado retornado pelo dbSet ou devolver o documento persistido ao fim dos métodos que gravam.

### B22 — P2 — CLI diverge dos exemplos e eval não executa TypeScript tipado

**Inspeção das flags; eval reproduzido.** `cmd/ddcore/main.go:349`, `:373`, `:449` e `:141` usam `flag.FlagSet.Parse`, que encerra a leitura de flags no primeiro argumento posicional. Exemplos como `ddcore exec caminho --args ...` não configuram `args`; `user add email nome --password ... --role ...` incorpora opções ao nome e não aplica senha/papéis. Em eval, `--commit` após o código acaba incluído no texto avaliado.

Além disso, `Runtime.Eval` usa eval JavaScript direto (`internal/js/prelude.js:447`), sem esbuild. **`const x: number = 1; x` falhou com SyntaxError**, apesar da interface anunciada como TS.

**Correção sugerida:** suportar flags intercaladas ou corrigir todos os exemplos e rejeitar sobras, para evitar execução silenciosa com argumentos errados. Transpilar snippets TS antes de avaliar. Criar testes do parsing real de cada comando.

### B23 — P2 — Suíte e bootstrap não correspondem integralmente à definição de pronto

**Inspeção e execução dos testes disponíveis.** O POC contém `tests/test_install.py` e `tests/test_traducao.py`, seis testes em cada, sem equivalentes neste port. Os sete grupos de domínio do POC totalizam 55 casos; o port tem 57, mas esse aumento não substitui os dois grupos ausentes. Nem todos os testes específicos de Frappe precisam ser copiados literalmente; precisam de equivalentes para os comportamentos ainda exigidos, especialmente rota inicial e precedência/consistência de tradução.

`internal/engine/engine_test.go:75` ignora `DDCORE_TEST_DSN`, usa endereço fixo e recria `ddcore_test`. Se o Postgres estiver indisponível, pula o único teste de lifecycle. `make test` chama `./bin/ddcore test` sem depender da compilação: pode faltar binário em checkout limpo ou executar binário antigo.

O checkout já contém `ddcore.json`, e `ddcore init` falha se ele existir; portanto, o roteiro literal `init && migrate` do plano não funciona ali. O comando `ddcore demo` não existe, embora haja `make demo` chamando um serviço. Também não foram encontrados a cópia do design e os planos por fase em `docs/superpowers/` previstos no documento.

**Correção sugerida:** criar um comando de verificação reproduzível que compile o código atual, use DSN de teste configurável e falhe claramente quando a integração obrigatória não puder rodar; completar os testes de aceite e ajustar os comandos/documentos de bootstrap.

## Outras lacunas e melhorias

| Item | Evidência e melhoria proposta |
|---|---|
| Expressões no servidor | `mandatoryDependsOn` é validado, inclusive em filhos. `readOnlyDependsOn` e `dependsOn` são carregados na meta, mas não avaliados pela validação Go como previsto. Definir a semântica esperada e implementá-la; não tratar ocultação visual como autorização. |
| Campos após submit | `checkAllowOnSubmit` roda antes de validate/beforeSave. Hooks podem alterar campos protegidos depois da checagem; incluir validação final, preservando a exceção deliberada de dbSet. |
| Filtro avançado / listSettings | ListView oferece busca, filtros padrão e docstatus, sem construtor de condições avançadas. A meta Go não inclui listSettings; há registro de defineListView no SDK do Desk que a listagem não consome. |
| Reports / gráficos | O renderer é `BarChart.svelte`; não há implementação de linha prevista no plano. Reports exibem summaries específicos, sem rodapé genérico de totais. A permissão report do refDoctype não é verificada pelo endpoint, que usa somente roles próprias do relatório. |
| ETag | `getMeta` não gera ETag nem trata If-None-Match. Implementar por versão/conteúdo da meta. |
| Dependências de apps | `requires` é carregado, mas não há validação/ordenação por dependências no Load. Evitar instalação em ordem incoerente ou com app requerido ausente. |
| Pool de runtimes | `Acquire` cria VM sempre que não há uma livre; `size` limita apenas a lista free. A lista all retém cada VM criada, inclusive as não reutilizadas. Limitar concorrência e liberar referências excedentes. |
| Credenciais de usuário desativado | `User.onUpdate` remove sessões, mas `UserFromAPIKey` verifica enabled da chave, sem consultar enabled do usuário. Revogar/bloquear chaves ao desativar o usuário. |
| Atualização diária de atrasos | `marcarAtrasados` só altera status. O card Total em Atraso soma `saldo_devedor` persistido, que não é reapurado diariamente; pode divergir do relatório que calcula encargos na consulta. |
| Export CSV | ListView e ReportView usam JSON.stringify para células: aspas são escapadas como JSON, não como CSV. Usar escape CSV correto, liberar Object URLs e explicitar que a exportação da lista contém só a página carregada. |
| Uploads | ParseMultipartForm limita memória, não tamanho total. Não há limite explícito do corpo, remoção do arquivo após rollback da inserção ou garantia forte contra colisão de nomes; o sufixo usa só a fração de segundo. Usar limites, nomes aleatórios e limpeza coordenada. |
| Observabilidade/testes | HTTP, MCP, jobs, migrações evolutivas e UI carecem de testes dedicados. Priorizar cenários negativos e concorrentes identificados aqui, em vez de apenas aumentar casos do caminho feliz. |

## Ordem recomendada de correção e revalidação

1. Fechar MCP e acesso avulso a filhos; proteger histórico, gráficos e eventos; garantir SQL readonly no banco.
2. Corrigir atomicidade das gravações, associação dos filhos, índices e geração concorrente; validar com duas transações reais.
3. Corrigir apuração histórica, recebimento mensal, reajustes repetidos e renovação; adicionar exemplos financeiros datados e verificáveis.
4. Corrigir geração de runtimes/reload, timeout e recuperação de jobs; testar interrupção e reexecução.
5. Reprovar CI quando typechecks falharem, corrigir Desk/CLI e completar os testes de instalação/tradução.
6. Reexecutar os cinco critérios finais do plano, incluindo navegador e a cadeia MCP completa em ambiente isolado, registrando as evidências.

As correções acima são recomendações para uma próxima implementação. Este relatório não declara corrigido nenhum dos problemas encontrados.
