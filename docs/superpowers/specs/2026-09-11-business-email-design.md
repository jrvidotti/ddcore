# Design: e-mail de negócio (OPS-02)

Registro do desenho executado em 2026-09-11. O contrato para quem escreve app está
em [`docs/agent/mail.md`](../../agent/mail.md); este documento guarda **por que**
cada peça ficou como ficou.

## Ponto de partida

O framework mandava exatamente dois corpos de mensagem, montados à mão numa função
Go (`authMail`), sem template, sem histórico, sem anexo e sem API para app nenhum.
O cabeçalho de `internal/mail/mail.go` dizia isso de si mesmo e nomeava o OPS-02
como dono de qualquer coisa mais rica.

A fundação era boa e não se jogou fora: transporte `log|smtp|method`, entrega
durável pelo `ddcore_job`, e o `enqueue` gravando na transação da requisição.

## Decisões

**1. Template é arquivo do app, não linha de tabela.** `mail/<nome>.mail.ts` fica
ao lado de `doctypes/` e `services/`. Custou zero no engine — o bundler pega todo
`.ts` do app por extensão e o extrator de i18n varre o diretório inteiro — e é o
que mantém `make check` honesto: as frases são chaves literais `_("…")` como
qualquer `label:`. Um template editável no desk teria posto o corpo fora do git e
fora do extrator, e `make check` reportaria "nada faltando" enquanto a mensagem
saía em inglês num site traduzido.

**2. O corpo é uma lista de blocos, não HTML.** Se as frases são chaves, o
template não pode devolver markup: a chave viraria tag e o escape viraria problema
do app. `p`, `h`, `button`, `table`, `rule` — o core detém o estilo e o escape, e
as duas partes do multipart saem da mesma lista. `TestRenderEscapesEveryString`,
`TestRenderRefusesAnUnsafeButtonScheme`.

**3. Renderizar em Go, com o template em TS.** O template *é* função JavaScript e
roda em goja, onde `_()` resolve idioma. Transformar blocos em texto e HTML é
função pura e foi para `internal/mail/render.go`, ao lado do `render` de MIME que
já existia — em `core/` não há um único `.test.ts`, então um renderizador em TS
nasceria sem arreio de teste. `TestRenderAlignsTableColumnsInText` mede a largura
em runas porque preencher por bytes é como uma tabela sai torta em português.

**4. Idioma é do leitor, não de quem aperta o botão.** O `_()` do prelude escolhe
o catálogo por `globalThis.__ddcoreLang`, então `renderMail` troca essa variável em
volta da chamada e restaura num `finally` — sem isso uma exceção dentro de um
template deixaria a VM inteira falando outro idioma. O idioma é resolvido na
transação de quem pede (`RecipientLang` lê em `c.Q()`, não no pool), para que
convidar alguém e mandar a mensagem no mesmo request enxergue o idioma recém
gravado. `TestOPS02_MessageIsWrittenInTheRecipientsLanguage`.

**5. O registro nasce na transação de quem manda.** Linha e job gravados em
`c.Q()`: um request que cai não mandou nada e não deixou registro de uma mensagem
que ninguém recebeu. Template inexistente, endereço inválido e anexo não
autorizado falham ali, onde ainda existe alguém para receber o erro, e não sozinhos
num worker meia hora depois. `TestOPS02_RollbackSendsNothing`.

**6. `Email Delivery` é DocType, não tabela interna.** Molde do `Error Log`. Tela,
permissão, REST, export e MCP saíram de graça, e "o cliente recebeu?" deixou de ser
pergunta para o suporte técnico. Sem `create` nas permissões: quem escreve é o
framework.

**7. O corpo montado não é guardado; os argumentos, sim.** A mensagem se
re-renderiza de `template` + `args`. Mantém a tabela pequena e tira o conteúdo
montado de uma tabela que todo System Manager lê.

**8. `sensitive` é a exceção que o item 7 exige.** Um link de recuperação é
credencial viva. Um template pode se declarar sensível: os argumentos não vão para
a linha, viajam no payload do job, e a mensagem não pode ser re-renderizada. Os
dois templates do próprio core são assim. O custo é uma flag cujo esquecimento
grava credencial, então ela está em negrito no `mail.md` e não numa nota de rodapé.
E o que ela **não** resolve está escrito junto: o payload do job também é coluna.
O PRD-04 entrou no main durante este trabalho e trouxe expurgo — 7 dias para job
concluído, 30 para falho — então a janela existe e é limitada também pela própria
validade do token. É bem menos exposição do que antes, quando o corpo inteiro com
o link ia para `ddcore_job.args` e nada nunca era removido, e não é zero.
`TestOPS02_SensitiveTemplateStoresNoArguments` afirma as duas metades: nada na
linha, tudo no job.

**9. Resultado incerto não é retentado.** Só o erro na resposta ao ponto final é
ambíguo — tudo antes dele falha limpo, a mensagem não saiu. `ErrUncertain` marca
esse caso, o job termina com sucesso e a linha fica `Uncertain` para uma pessoa
decidir. Retentar ali é exatamente como se manda a mesma mensagem duas vezes. O
`frappe-port-inventory.md:193` pede que isso seja documentado, não resolvido.
`TestSMTPLostAnswerIsUncertain` usa um servidor SMTP de mentira que corta a conexão
depois de ler o ponto — os primeiros testes do repositório a conversar SMTP de
verdade.

**10. Idempotência é chave de quem chama, não hash de conteúdo.** Hash
desduplicaria dois envios legitimamente iguais, e reenviar um convite é coisa que
se faz de propósito. Quem precisa passa `key`, e o índice único decide sob
concorrência. `TestOPS02_IdempotencyKeyRefusesASecondSend` afirma os dois lados.

**11. Anexo é referência a `File`, autorizada uma vez.** A regra "pode ler este
arquivo" existia embutida na rota de download e reimplementada no export. Virou
`Ctx.CanReadFile` e a rota passou a chamá-la. O `exportFiles` continua perguntando
de outro jeito, de propósito: ele já checou os documentos da página e escopa a
busca em vez de redecidir por arquivo — o comentário lá sempre disse isso. Bytes
crus foram recusados: bytes sem dono não têm política de retenção nem permissão.
`TestOPS02_AttachmentNeedsPermission`.

**12. O teto de anexo é conferido no enfileiramento.** Somando `File.file_size`,
antes de qualquer coisa ser gravada, para que o limite do relay não seja a primeira
notícia. `TestOPS02_AttachmentsOverTheCapAreRefused`.

**13. Apagar o documento não apaga o histórico.** `coreRefs` fazia `DELETE` na
exclusão — foi assim que anexo, comentário e versão passaram a sumir junto do
documento. Ganhou `keepOnDelete`: um pedido pode ser apagado, uma fatura que já
chegou ao cliente não pode ser des-enviada. Renomear continua varrendo tudo, porque
um registro que sobrevive ao documento precisa apontar para o nome que ele atende
agora. `TestOPS02_HistoryOutlivesItsDocument`.

**14. Os e-mails do próprio framework viajam pela estrada dos apps.**
`StartRecovery` chama `core.services.mail.queue` por caminho pontilhado, o mesmo
mecanismo que o worker usa. É a única forma de a estrada continuar honesta — e o
teste de aceitação que raspa o token do log passou sem uma linha alterada, que é o
sinal de que o comportamento observável não mudou.

## Um bug encontrado no caminho

Um campo `JSON` salvo pelo caminho comum saía **codificado duas vezes**: `validate`
chama `castAll` duas vezes — antes e depois do hook `validate`, para capturar o que
o hook escreveu — e o cast fazia `json.Marshal` sem perguntar se o valor já estava
codificado. `Version` escapava por escrever SQL próprio; `Email Delivery` foi o
primeiro campo `JSON` a usar `NewDoc` → `Insert`. O cast agora é idempotente: uma
string que já é JSON válido passa direto. `TestOPS02_JSONFieldIsNotEncodedTwice`
afirma pelo `jsonb_typeof`, não pelo texto, porque o texto "contém" o valor nos
dois casos e foi assim que isso passou despercebido.

## O que ficou de fora

Entrada (IMAP, bounce, webhook), CC/BCC/Reply-To, From por mensagem, reenvio pelo
desk, e anexo por bytes crus. O `apps/testapp` não ganhou template nenhum: a
cobertura ponta a ponta vive em `internal/engine`, onde o worker é alcançável, e o
`CLAUDE.md` diz que a fixture é do tamanho do que `internal/acceptance` afirma e
nada mais.
