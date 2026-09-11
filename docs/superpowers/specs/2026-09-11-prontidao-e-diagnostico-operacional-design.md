# Design: prontidão e diagnóstico operacional (PRD-03)

Registro do desenho executado em 2026-09-11. O contrato para quem opera está em
[`docs/agent/ops.md`](../../agent/ops.md); este documento guarda **por que** cada peça
ficou como ficou.

## Ponto de partida

`/api/health` era um literal: `writeJSON(w, 200, map[string]any{"ok": true})`. Nunca tocava
o banco. Um site com o Postgres fora do ar respondia `ok` e seguia recebendo tráfego.

O `doctor` imprimia `database:   ok` **escrito à mão** — chegava naquela linha só porque o
`load()` anterior já tinha aberto o pool. Com o banco fora do ar, `load()` falhava, o comando
devolvia erro e o `main` imprimia `error: …`: **nenhum relatório saía**, exatamente na
situação em que alguém corre um doctor. E ele sempre terminava com código 0.

Não havia request id em lugar nenhum — nem Go, nem TS, nem banco. Não havia log de acesso.
E o `writeErr`, a única fronteira erro→HTTP, não registrava nada: um 500 entregue ao usuário
não deixava rastro no servidor. O `Recoverer` do chi engolia pânico num 500 pelado, fora do
envelope JSON e fora do `Error Log`.

Nenhuma função Go consultava `ddcore_job`.

## Decisões

**1. A sonda do banco não pode depender do engine.** O modo de falha que o `doctor` existe
para relatar é justamente *o engine não existe*: `engine.New` chama `db.Open`, que pinga e
devolve o erro em vez de um engine. Uma sonda que fosse método de `*Engine` estaria
inalcançável precisamente quando importa. Daí `db.Probe(ctx, dsn, timeout)`, que abre o
próprio pool a partir de um DSN pelado. `TestPRD03_DoctorReportsAnUnreachableDatabase`.

**2. O readiness não roda o agregado da fila.** `ddcore_job` cresce sem retenção — retenção
é PRD-04 — e um endpoint anônimo que dispara um scan é um amplificador de graça. Readiness é
um `Ping` com deadline. Os números vivem em `/api/health/report`, autenticado.

**3. Limiar nunca vira 503.** Se fila cheia reprovasse o `/readyz`, o orquestrador
reiniciaria exatamente os workers que estão drenando a fila. Só `down` — o banco não
responder — muda código de status; limiar produz `warn`.

**4. O corpo do readiness é um booleano.** Latência é canal de realimentação para quem mede
o próprio efeito no site; profundidade de fila vaza volume de negócio e diz quando o site
está sob pressão; a versão diz quais avisos de segurança se aplicam; e o texto de erro do pg
carrega host, porta, usuário e nome do banco. Um orquestrador lê o código de status e mais
nada. `TestPRD03_ReadinessLeaksNothingAboutTheDatabase`.

**5. A sonda é memoizada por um segundo.** Sem isso, cada acesso de cada réplica é uma ida ao
banco que qualquer um dispara à vontade. O custo é que um banco recém-caído é relatado pronto
por até um segundo, e isso está escrito no contrato. É o que substitui um rate limit.
`TestPRD03_ReadyIsMemoisedForASecond`.

**6. As sondas escapam do `s.auth`, por igualdade exata.** Hoje um `Authorization: token`
inválido responde 401 *antes* do handler, em qualquer rota — então um agente de monitoração
com chave vencida relataria processo morto. A lista é de caminhos exatos e nunca de prefixo:
um `HasPrefix("/api/health")` entregaria o `/api/health/report`, que é justamente o que tem
profundidade de fila e contagem de erro. `TestPRD03_ProbesIgnoreABadToken` afirma as duas
metades, inclusive a negativa.

**7. Id próprio, não o `middleware.RequestID` do chi.** O do chi copia o header de entrada
verbatim para o log, embute o hostname do contêiner e um contador global no que gera, e não
devolve header nenhum. São trinta linhas escrever um que sanitiza, ecoa e não conta nada
sobre a máquina.

**8. Sanitizar, não escapar.** O valor de entrada chega a uma linha de log, a um header de
resposta e a uma coluna `text`. Um `\n` forja entrada de log; um megabyte inunda a coluna. A
regra é conjunto de caracteres conservador e teto duro — que ainda admite UUID, ULID e
`traceparent` do W3C, então adotar trace distribuído depois é aditivo. Os casos de injeção
são testados na unidade e não por HTTP, porque o cliente do próprio Go recusa mandar controle
em header: isso é uma segunda linha de defesa, não esta, e não é uma na qual o servidor possa
se apoiar. `TestPRD03_HostileRequestIDIsReplaced`.

**9. O `writeErr` carimba numa cópia.** `cerr.From` devolve o **mesmo ponteiro** quando o
erro já é `*Error`, e `Translate` faz o mesmo quando não há chave. Carimbar no original
escreveria num valor que outra requisição pode estar segurando — e vazaria o id de um
chamador na resposta de outro. É uma linha de código e três de comentário.

**10. 500 escreve no `Error Log`; 4xx não.** O 500 é o erro sobre o qual quem chamou não pode
fazer nada e que o operador vai ter que achar depois, então ganha linha com o mesmo id que o
usuário viu. Um 4xx é obra de quem chamou e já está no log de acesso — mandá-lo para o
`Error Log` transformaria validação em ruído. `TestPRD03_ClientErrorsDoNotFillTheErrorLog`.

**11. O pânico ganha o envelope comum.** O `Recoverer` do chi escreve um 500 sem corpo: o
desk monta o toast com um pedaço de texto solto, nada chega ao `Error Log`, e o id que o
chamador recebeu não leva a lugar nenhum. Agora o texto do pânico e a pilha ficam no
servidor, o cliente recebe o id, e é o id que junta os dois. O `LogError` de dentro do
tratador vai embrulhado no próprio `recover` — pânico dentro de tratador de pânico derruba o
processo — e nada é escrito se algum byte já foi para o fio.
`TestPRD03_PanicBecomesAJSONErrorAndAnErrorLogRow`.

**12. `run_after` fora da idade da fila.** Um job enfileirado para amanhã não é backlog; sem
o filtro ele fixaria o alarme de idade desde o instante em que foi criado, e o alarme nunca
mais significaria nada. Daí `runnable` ao lado de `queued`.
`TestPRD03_OldestQueuedIgnoresAScheduledJob`.

**13. `stalled` reusa o predicado do `requeueStale`.** O número relatado é exatamente o que
um worker vivo devolveria para a fila — é assim que ele vira o sinal de "um worker morreu
segurando trabalho" em vez de mais um contador. `TestPRD03_StalledJobIsReported`.

**14. O `LogError` se solta do contexto cancelado.** O contexto de uma requisição que falhou
muito frequentemente já está cancelado — o cliente desligou, ou foi um timeout que a fez
falhar — e a linha que explica o porquê era justamente a que se perdia. Solta com
`WithoutCancel`, mas com teto de 5 s: isso roda num caminho de erro e não pode segurar
conexão. Correção de arrasto, com teste próprio.
`TestPRD03_LogErrorSurvivesACancelledContext`.

**15. A chave do id mora no engine, não na api.** `LogError` é método do engine e o engine
não pode importar a api. Pondo `WithRequestID`/`RequestIDFrom` no engine, a assinatura do
`LogError` não muda e todo log do lado do engine correlaciona de graça. O `NewCtx` lê o id do
contexto em vez de recebê-lo por parâmetro — assim nenhum ponto de entrada pode esquecer de
correlacionar.

**16. O log de acesso é filtrado por nível, não desligado.** Uma abertura do desk são ~30
requisições na mesma cadeia: o fallback do SPA, cada asset, cada arquivo. Tudo em `info`
soterra as poucas linhas que dizem algo. Asset em 200 é `debug`; `/api/…` é `info`; 4xx é
`warn`; 5xx é `error`; e qualquer coisa acima de `ops.slowRequestMs` é `warn` venha de onde
vier. Só o `path`, nunca a query: filtro de lista carrega dado pessoal e link de recuperação
carrega token.

**17. `DDCORE_LOG_FORMAT` no `.env`, limiares no `ddcore.json`.** A forma do log é
propriedade de onde o processo roda — terminal lê texto, plataforma que manda stdout para
coletor precisa de objeto indexável. Já os limiares são decisão do site: staging tem que
chamar backlog de backlog igual à produção, ou o ensaio não é ensaio. É a mesma divisão que o
comentário do pacote `config` já explicava, e a mesma que o `auth` segue.

**18. Bloco parcial de `ops` é completado, valor impossível é recusado.** Quem escreve só
`{"queueBacklog": 500}` quer aquele número e o resto como vem de fábrica — deixar os outros
em zero seria alarme disparando no primeiro job. Já um zero escrito de propósito é recusado
no boot, a mesma regra que o modo de arredondamento e a política de acesso já seguem.

**19. O `doctor` degrada e sai com código.** O probe roda antes do engine e independente
dele; se o engine não sobe, sai o relatório dizendo isso mais tudo que não depende de banco.
O `os.Exit` vem **depois** de imprimir, nunca `return err`, porque o `main` escreveria
`error: …` por cima do relatório. Saída 1 só para crítico: DDL pendente e backlog são coisas
para ler, e transformar os "3 pending statements" de hoje em falha de shell seria regressão
para todo mundo que roda isso à mão. `--strict` é para o script.
`TestPRD03_DoctorWarningsDoNotFailUnlessStrict`.

**20. Nada de DSN no relatório.** O erro de conexão do pgx sai como ``failed to connect to
`user=… database=…` `` e um site mal configurado põe a URL inteira na mensagem. Tudo passa por
um funil de redação — o mesmo raciocínio que já estava escrito sobre a seção de segredos:
este relatório é colado em issue e janela de chat. `TestPRD03_DoctorNeverPrintsTheDSN`.

**21. Sem `HEALTHCHECK` no Dockerfile.** O `ENTRYPOINT` da imagem é o próprio binário, e a
mesma imagem roda `ddcore migrate` e `ddcore doctor` como contêiner de uma tacada só — um
healthcheck assado marcaria todos eles como unhealthy. O contrato documenta os trechos de
compose e de k8s em vez disso.

**22. O scheduler relata declarações, não vida.** `SchedulerHealth` conta
`ScheduledMethods()` e não `e.sched`, porque o processo que pergunta costuma ser o `doctor`,
que nunca iniciou cron nenhum. Nada persiste última execução hoje, então "o scheduler está
vivo" não é afirmação que dê para fazer honestamente — e a peça que afirma menos é a que não
mente.

## Achado que barateou a execução

`Ctx.Request` já cruzava para o TS como `ddcore.session.request`, e o `host.go` devolve o mapa
inteiro. Expor o id ao código de app custou uma chave no mapa, não um caminho novo — e
`isJob()`, que detecta contexto de job pela **ausência** de `request`, continuou valendo
porque o campo novo é irmão dele e não parte dele.

## O que ficou de fora

- **Alertas de backup**: o roadmap adia explicitamente para "when recovery automation
  exists". PRD-01/02 são Stage 4 e nada produz evento de backup ainda — alerta sem produtor é
  galho morto.
- **Administração de jobs** (retry, cancelamento, retenção): é PRD-04. Esta fatia relata a
  saúde da fila e não escreve em linha nenhuma de job.
- **`request_id` em `ddcore_job`**, para propagar o trace requisição→job: é mudança de schema
  na tabela que o PRD-04 governa. A metade barata — o handle `job:<id>` na linha de
  `Error Log` — entrou.
- **Prometheus/OpenTelemetry**: endpoint de métricas sem coletor implantado é produto que
  ninguém usa. O relatório autenticado e o `doctor --json` já são raspáveis por cron hoje; é
  a segunda fatia natural.
- **Histórico de última execução do scheduler** e histogramas de latência por rota. O
  `RestartScheduler` (`jobs.go`) segue sem chamador, o que é assunto do B08 e não deste item.
