# Design: evolução de schema e dados (DAT-04)

Registro do desenho executado em 2026-09-10, a partir de
[`inventario-migracao-frappe.md:119`](../../inventario-migracao-frappe.md). O contrato para
quem escreve app está em [`docs/agent/migrations.md`](../../agent/migrations.md); este
documento guarda **por que** cada peça ficou como ficou.

## Ponto de partida

O framework já tinha DDL automático, patches, `--dry-run` e `--prune`. O que faltava era
**intenção**: `db.Plan` comparava a meta com o catálogo campo a campo, e nesse diff um
renome é indistinguível de "apagar um e criar outro". O resultado era coluna nova vazia,
coluna velha órfã — e sob `--prune`, a velha derrubada **na mesma transação em que a nova
foi criada e antes de qualquer patch**. Não havia como um patch se meter no meio.

Três coisas a mais estavam faltando e só apareceram ao tentar escrever o roteiro:
`ddcore.db.sql` é somente leitura por construção (`SET LOCAL transaction_read_only = on`),
então o passo "preenchimento" não tinha primitiva nenhuma; `--prune` não tinha um único
teste; e `ALTER COLUMN … USING col::tipo` era um cast cego que, em metade das combinações,
o Postgres nem sabe fazer.

## Decisões

**1. O renome é declarado, nunca inferido.** `renamedFrom` no campo e no DocType. A
alternativa — heurística por tipo e posição — erra silenciosamente exatamente quando mais
custa, e um acerto de 95% num migrador de dados vale zero. Aceita `string | string[]`
porque um banco que pulou um release está duas renomeações atrás, e uma cadeia
`["a", "b"]` é o que o faz chegar ao fim.

**2. `convert` nomeia o fieldtype de origem, e não carrega SQL.** Um `convert: "cast"`
booleano seria mais curto e silenciosamente autorizaria, dois releases depois, uma
conversão diferente da que o autor examinou. Um `using: "<sql>"` seria mais poderoso e
colocaria SQL dentro da meta, que é declarativa e é lida pelo Desk e pelo typegen —
conversão que um cast não expressa é justamente o que patch existe para fazer.

**3. Só é automático o que cabe em toda linha sem consultar o dado.** `safeWidening` é uma
tabela explícita, não uma regra computada: são nove tipos de coluna, as respostas não se
derivam de nada, e uma entrada errada perde dado calado. `Int → Float` entra porque os
números de um app são float64 do goja de qualquer jeito; `Currency → Float` fica de fora
porque perde exatidão e DAT-06 existe; `Date ↔ Datetime` fica de fora porque o resultado
depende do fuso da conexão, e a mesma migração produziria dados diferentes em duas máquinas.

**4. Recusar é o produto, não o efeito colateral.** `text → numeric` aborta na primeira
linha que o Postgres não consegue ler, e `numeric → boolean` nem cast tem: hoje isso vira
um `ERROR: cannot cast type` cru sem saída. A recusa troca um erro inacionável por um que
nomeia DocType, campo, tipo atual, tipo desejado e os dois caminhos adiante. E recusa **o
plano inteiro**: meia migração aplicada é o estado que este trabalho existe para não criar.

**5. Fases de patch, nomeadas pelo que o framework garante.** `beforeSchema` e
`afterSchema`, não `expand` e `contract`. A fase diz **quando o patch roda em relação ao
DDL**, que é o que o core sabe; o que o autor pretende com isso é do autor. E reaproveita o
vocabulário de `beforeSave`/`afterInsert` que o framework já fala.

**6. O plano é calculado depois dos patches `beforeSchema`.** Custa uma linha e compra a
propriedade que mais importa: **o planejador nunca é um beco sem saída**. O que ele recusar,
o autor ainda faz à mão num patch, e o plano seguinte já não tem o que recusar. Um plano
calculado antes ficaria obsoleto no instante em que o patch mexesse no schema.

**7. `ctx.sql` escreve, e só existe dentro de um patch.** É a peça que faltava para o
preenchimento existir. Chega pelo argumento de `execute` e é tipado em `PatchContext`, de
modo que **nenhum controller, serviço ou report o alcança por construção** — um global em
`ddcore` estaria ao alcance de todo mundo. Aceita DDL de propósito: é o que sustenta a
decisão 6. A alternativa, laço documento a documento pelo ciclo de vida, reescreve
`modified` de cada linha e grava uma Version por linha: uma trilha de auditoria dizendo que
uma pessoa editou o que uma migração moveu.

**8. `definePatch` em vez de um `export const phase` solto.** Decide sozinho pela decisão 7:
um export solto não tem como tipar o argumento de `execute`. Além disso toda a superfície do
SDK é `define*`, e não havia tipo nenhum de patch antes — é o único momento livre para
introduzir um. O `export function execute` nu continua funcionando, porque é o que
`conventions.md` documentava, e sem fase é `afterSchema`, o comportamento de hoje bit a bit.

**9. As retiradas rodam por último, e recusam derrubar o que tem dado.** Duas mudanças, uma
consequência. Depois dos patches, um preenchimento ainda lê a coluna que a mesma migração
remove. E recusando o que não está vazio, apagar uma declaração `renamedFrom` cedo demais
vira uma migração que para e nomeia a coluna, em vez de esvaziá-la. Sem isso, "pode apagar a
declaração?" seria conselho; com isso, é política.

O preço: a retirada legítima — o dado já foi copiado e a coluna velha é para sumir — deixa
de ser `--prune` e passa a ser um `ALTER TABLE … DROP COLUMN` escrito no patch de validação.
Isso é o desenho, não o desvio: destruir dado fica sendo um ato deliberado, com o nome do
autor em `ddcore_patch`, e não o efeito de uma flag.

**10. `ddcore_rename` + `doctor` respondem a única pergunta que a declaração levanta.**
Nenhum checkout sabe dos outros ambientes, então o honesto é o `doctor` de cada banco dizer
"aqui já foi aplicada", e a regra escrita ser "apague quando todo site disser o mesmo".

**11. Instalação nova grava os patches sem rodar.** Um patch descreve mudança em dado que um
banco novo não tem; com `beforeSchema` ele passaria a rodar contra tabelas que ainda não
existem. Semear é trabalho de `afterInstall` e de fixtures.

**12. Continua uma transação só.** O Postgres tem DDL transacional — abrir mão disso seria
regressão, não recurso. É também o que faz o passo "validação" do roteiro sair de graça: um
patch `beforeSchema` que dá `throw` desfaz a retirada junto com todo o resto. O custo é o
lock segurado pela duração; fica escrito no contrato, com a regra de que patch faz trabalho
limitado e um preenchimento grande vai para a fila.

**13. Fora do escopo, deliberadamente:** depreciação por objeto (um `deprecated: true` com
período de carência) — a ordem das fases mais a recusa de derrubar dado já entregam o que o
roteiro precisa; fixtures que atualizam, que é a terceira cláusula da linha DAT-04 e ficou
registrada como residual; varrer `ddcore_job.args` e `tab_version.data` num renome de
DocType; e transformar `File`/`Comment`/`Version` em `Dynamic Link`, que seria a correção de
princípio para as três colunas fixas em `coreRefs` mas mexe na meta do core e no Desk.

## Achados que barataram a execução

- **`Ctx.Rename` já era o molde**: percorre o registry reescrevendo Link, Dynamic Link,
  `parent` dos filhos e as tabelas do core. A varredura do renome de DocType é a mesma
  forma, um nível acima.
- **O catálogo já estava todo em memória** quando o diff começa, então detectar renome não
  custou nenhuma consulta nova. A única consulta que este trabalho acrescenta é a de "ainda
  tem dado?", e só sob `--prune`, onde os candidatos são poucos.
- **O `sameIndex` já ignorava nome e tabela** na comparação, então deu para reusá-lo
  inteiro para comparar um índice renomeado contra a própria definição antiga.
- **`c.Flags` já era o jeito da casa** de marcar escopo (`ignorePermissions`,
  `captureLogs`), então `inPatch` não inventou mecanismo.

## O que o trabalho encontrou

Quatro defeitos que não eram de migração, achados porque o desenho passou por cima deles:

- `Ctx.Rename` atualizava `tab_version` e `tab_comment` mas não `tab_file`: renomear um
  documento órfãva os anexos — e com eles a checagem de permissão de arquivo privado, que
  lê `File.attached_to_name` para decidir quem baixa.
- `Ctx.Delete` tinha a mesma omissão, deixando linhas de File para trás. As três tabelas
  viraram uma lista só (`coreRefs`), usada pelas três operações, porque manter três cópias
  foi como a omissão nasceu.
- As fixtures eram percorridas com `range` sobre mapa Go: a ordem entre DocTypes mudava a
  cada execução, e uma fixture cujo Link aponta para outro DocType de fixture falhava de
  forma intermitente.
- `cmd/ddcore/main.go` descartava o erro de `e.Plan` no boot do `dev` (`plan, _ :=`), então
  uma recusa apareceria como "0 statements pendentes".

E um conceitual: `titleField`, `sortField` e `searchFields` nomeiam um campo por string e
nada os validava — um renome os deixava apontando para um nome que não existe mais, calado.
Agora a meta se recusa a carregar, que é a metade do renome que a declaração não faz sozinha.

## Portão

`internal/engine/migrate_test.go` (renomes, conversões, fases, prune), `internal/db/schema_test.go`
(a tabela de alargamento e o relatório), `internal/meta/meta_test.go` (as recusas de
validação) e `TestEvolucaoDeSchema` em `internal/acceptance`, que roda o caminho inteiro
contra um Postgres real. O caso que prova DAT-04 é `TestExpandBackfillValidateContract`:
expande, preenche, deixa uma linha por converter, tenta contrair e **falha com rollback**,
conserta a linha e só então a retirada acontece.
