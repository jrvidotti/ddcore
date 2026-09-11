# Design: precisão decimal e semântica de datas (DAT-06)

Registro do trabalho executado em 2026-09-10 para o item DAT-06 do
[inventário](../../inventario-migracao-frappe.md). O contrato para quem escreve app está em
[`docs/agent/fieldtypes.md`](../../agent/fieldtypes.md) e
[`docs/agent/controller-api.md`](../../agent/controller-api.md); este documento guarda **por
que** cada peça ficou como ficou.

## Ponto de partida

O inventário descrevia DAT-06 como "parcial para equivalência". A inspeção mostrou algo mais
simples e pior: **não havia contrato nenhum**.

`Currency` era `numeric(21,9)` no Postgres e um float sem arredondamento em todo o resto da
pilha. Digitar 10.005 num formulário gravava 10.005 e exibia 10.01 — o número na tela e o
número que entra num `sum()` eram números diferentes, e nada no framework tinha opinião sobre
qual estava certo. `FieldDef.precision` existia e era ignorado pelo DDL, pelo `castValue` e
pelo próprio editor, que fixava 2 casas. Não havia configuração de precisão nem de
arredondamento em lugar algum, então "definir precisão/arredondamento" não tinha onde ser
definido.

Do lado das datas o contrato já estava escrito e o desk já o cumpria. O servidor não.

## Decisões

**1. O arredondamento é definido sobre a representação decimal mais curta, não sobre o
double.** Quem digita 1.005 grava 1.00499999999999989; arredondar o valor binário dá 1.00, que
não é o número que a pessoa escreveu. Formatar o double de volta para sua representação mais
curta recupera "1.005", e arredondar essa string dá 1.01. É a diferença entre uma regra que um
contador confere e uma que só um artigo sobre ponto flutuante explica.

**2. Uma regra, três implementações, uma tabela.** A mesma regra roda em Go (`internal/num`),
no runtime da app (`prelude.js`) e no desk (`round.ts`), porque um total calculado num script
de formulário precisa bater com o total que o servidor grava. `String(v)` em JS emite
exatamente os dígitos de `strconv.FormatFloat(v, 'f', -1, 64)` em Go, então um algoritmo
baseado em string é idêntico nas três por construção — o que não acontece com nada construído
sobre potências de dez. As três leem `internal/num/testdata/rounding.json`: duas cópias de uma
tabela divergem, e o dia em que divergirem é o dia em que o formulário e o banco discordam de
um centavo. A implementação em Go é a autoritativa, porque é a que arredonda em toda escrita.

**3. `currencyPrecision`, não `precision`.** Uma chave chamada `precision` no `ddcore.json`
seria lida como "precisão dos números", e `precision` de campo já significa outra coisa e vai
continuar significando. O padrão é a unidade menor ISO da moeda — 2 para USD, 0 para JPY, 3
para KWD — lida do mesmo CLDR que o `Intl` do navegador, então servidor e desk concordam sem
combinar. `rounding` é `"commercial"` (metade para longe do zero) ou `"bankers"`; um valor
desconhecido **falha na carga**, porque dinheiro não é lugar de adivinhar.

Ficaram fora de propósito `"half-up"`/`"half-even"`: "half up" é ambíguo — em boa parte da
literatura significa "para o infinito positivo", que é outra regra para negativos.

**4. `castValue` é o ponto único, e vira método.** Tudo passa por lá: REST, MCP, código de
app, `dbSet`, defaults e fixtures. Mais importante, ele é chamado **nos dois lados** das
comparações de `readOnly` e `allowOnSubmit` — é isso que impede uma linha gravada antes deste
trabalho de parecer editada. As opções (fuso, precisão, regra) são fatos do site, não da
requisição, e por isso são resolvidas uma vez no `Engine`: `castAll` roda por campo por linha
por save, e uma importação sentiria a diferença.

**5. Só `Currency` é arredondado.** `Percent` é taxa — 33.333333 é um terço legítimo — e
`Float` é medida, cuja `precision` a documentação promete ser só de exibição. `Int` mantém a
regra própria (metade para longe do zero) e é deliberadamente imune a `rounding`: uma
contagem que mudasse porque alguém configurou arredondamento bancário para faturas seria uma
surpresa de verdade.

**6. `splitAmount`, não uma API decimal.** A linha do inventário diz "criar API decimal se os
casos exigirem". Os casos não exigiram: um double é exato até 2^53, o que cobre dinheiro com
folga, e um tipo decimal contaminaria tipos gerados, filtros, relatórios e o fio inteiro. O
que faltava não era precisão de representação, era **onde o centavo sobra**: três terços de
100,00 arredondados dão 33,33 cada e deixam a fatura um centavo curta. `splitAmount` decide
isso — resíduo nas primeiras parcelas — em vez de deixar emergir.

**7. Datas: um parser, não três.** Havia dois padrões diferentes para uma string sem offset —
`castValue` usava o fuso do processo, `parseTime` usava UTC — e `ddcore.utils.now()` devolve
justamente uma string sem offset, no fuso **do site**. Gravá-la de volta a reinterpretava.
Invisível enquanto tudo é UTC, e um erro de horas no instante em que não é. Os dois passaram a
receber a localização do site: dois parsers que discordam sobre qual relógio escreveu uma
string sem offset são um bug, não vários.

O `cron.New()` sem localização era o mesmo erro num grão mais grosso: `daily` disparava à
meia-noite do processo, não do dia que o negócio está tendo. E `Time` era o único fieldtype
que não validava nada — qualquer string ia ao Postgres, que respondia com um erro de driver
sem nome de campo nem valor.

## O que o trabalho encontrou

Três defeitos que não eram de precisão, achados porque os testes novos os alcançaram:

- `NaN` vindo de aritmética JS era gravado como `null`, silenciosamente; `±Inf` de uma string
  como `"1e400"` idem; e um valor além dos 12 dígitos inteiros de `numeric(21,9)` chegava como
  erro cru de driver. Os três agora falham com o nome do campo.
- `Ctx.Now()` devolvia a hora do processo — e não tinha nenhum chamador, o que explica por que
  ninguém tinha notado.
- Os calendários de `DateControl` e `MonthControl` perguntavam ao navegador que dia é hoje,
  podendo destacar um "hoje" com que o servidor não concordava.

## Sem migração

`ColumnType` não mudou, então `migrate` não emite nada e nenhuma linha existente é tocada. Uma
tabela vai legitimamente conter valores arredondados e valores antigos até que os antigos
sejam salvos de novo — que é o padrão seguro. Reescrever o livro-razão de alguém em silêncio é
pior que a inconsistência. Para quem quiser converter, o padrão é um patch explícito:

```sql
UPDATE tab_fatura SET total = round(total::numeric, 2);
```

## Mudança de comportamento anunciada

`roundTo` e `flt(v, p)` mudam de resposta para meios exatos negativos: `roundTo(-1.005, 2)`
era `-1.00` e passa a ser `-1.01`. É correção de bug — a assimetria fazia um crédito e um
débito de mesmo tamanho não se cancelarem — mas um app que dependia do número antigo verá a
diferença.

## Deixado para depois

- **`SET TIME ZONE` na conexão.** Hoje o offset impresso no fio é o do processo; o instante
  está certo, mas um export lê de forma confusa. Mudar isso altera também o que
  `date_trunc('day', ts)` considera um dia em SQL cru de app, e nada na suíte cobre isso.
  Merece mudança própria, com teste de que o `date_trunc` passa a agrupar pelo dia do site.
- **Totais de relatório e CSV.** `ReportView` soma em doubles no cliente e `csv.ts` exporta
  números com ponto decimal mesmo onde o separador de colunas foi escolhido pelo locale. Os
  dois arquivos estão sendo reescritos por DAT-02; pegá-los agora compraria conflito de merge
  por ganho cosmético.

## Portão

`make test` — incluindo `internal/num` (25 vetores × 2 regras, mais 20.000 valores aleatórios
conferidos contra aritmética racional exata), `internal/js` (os mesmos vetores dentro da VM
real) e `desk` (os mesmos vetores de novo). Mais `apps/demo/doctypes/invoice/invoice.test.ts`,
que prova ponta a ponta o que os três garantem em separado.
