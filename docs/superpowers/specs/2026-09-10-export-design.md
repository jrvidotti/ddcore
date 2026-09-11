# Design: exportação completa e reconciliável (DAT-02)

Registro do desenho executado em 2026-09-10. O contrato para quem escreve app está em
[`docs/agent/export.md`](../../agent/export.md); este documento guarda **por que** cada
peça ficou como ficou.

## Ponto de partida

O Desk exportava CSV da página carregada — o `ListView` dizia isso no próprio toast
(`Exported {0} of {1} rows (the current page)`). Não havia endpoint, comando de CLI, nem
travessia de filhos ou anexos. E a permissão `export`, declarada no `PermDef` e na meta
desde a v1, **não era verificada em lugar nenhum do servidor**: era só um flag que o Desk
usava para esconder um botão.

O que o [inventário](../../inventario-migracao-frappe.md) pede em DAT-02 é o oposto de uma
tela: percorrer todo o conjunto, filhos e anexos, respeitando filtros e autorização, para
extrair e **reconciliar** o legado.

## Decisões

**1. A varredura passa pelo `GetList`, não por SQL próprio.** É o que faz `ifOwner` e o
`permissionQuery` do controller do app valerem para uma exportação de graça. Escrever a
consulta aqui criaria um segundo lugar onde a autorização pode estar errada — e o critério
de aceite do inventário nomeia exportação junto com relatório, histórico e arquivos.

**2. Paginação por keyset (`name > último`), nunca por OFFSET.** Um OFFSET sobre um
conjunto que encolhe durante a varredura pula linhas: apagado um documento já exportado, a
página seguinte começa uma linha adiante e um documento nunca sai. Isso está coberto por
`TestExportDoesNotSkipWhenTheSetShrinks`, que foi verificado reprovando contra uma
implementação por OFFSET antes de ser aceito.

**3. Um `ExportSink`, e o chamador é dono dos bytes.** O motor empurra documentos; virarem
NDJSON num socket ou CSV num diretório não é assunto dele. É o que permite a mesma
travessia servir o HTTP e a CLI com memória constante nos dois.

**4. Tudo que pode recusar acontece antes do primeiro byte.** Foi o defeito encontrado pelo
teste de aceitação: a checagem de permissão morava dentro de `Export`, que só roda depois
de os cabeçalhos e o `200` terem ido para o fio — uma exportação negada chegava ao usuário
como um download vazio e bem-sucedido. Daí `Ctx.CanExport`: o mesmo pré-voo, compartilhado
com a varredura em vez de copiado.

**5. Depois do `200` não há código de status, então sobra conferência.**
`X-DDCore-Export-Count` diz quantas linhas o arquivo deveria ter, e o NDJSON fecha com
`{"_manifest": …}`. A ausência do manifesto denuncia um stream cortado. Sem isso, um
arquivo truncado é indistinguível de um completo.

**6. `REPEATABLE READ` como primeira instrução da transação.** Cada página da varredura
precisa ver o mesmo instante do banco, ou a contagem não fecha. O Postgres recusa a troca
de isolamento depois da primeira consulta — por isso `SnapshotIsolation()` é chamado antes
de papéis, meta ou qualquer outra coisa.

**7. CSV com filhos pelo HTTP é recusado, não achatado.** Um arquivo não comporta um pai e
suas tabelas filhas sem repetir o pai por linha ou perder os filhos em silêncio. Recusar
com uma mensagem que aponta `format=ndjson` ou a CLI é honesto; entregar um CSV que perdeu
os filhos não seria.

**8. Teto no HTTP, nenhum na CLI.** O teto existe porque um download segura conexão e
worker; a CLI não segura nada. Um `limit` explícito é o chamador aceitando uma amostra, e é
permitido — mas o manifesto vem com `"truncated": true`, para que ninguém reconcilie uma
amostra como se fosse o conjunto.

**9. Paridade de valores entre CSV e NDJSON.** Um `Check` sai `true`, nunca `1`. As duas
saídas são comparadas lado a lado numa conferência; divergirem por convenção de formato
seria ruído puro. O CSV usa `encoding/csv` da stdlib (RFC 4180 de graça) com BOM e CRLF —
as mesmas regras que `desk/src/lib/csv.ts` já documentava para o Excel.

**10. Segredo nenhum viaja.** Todo campo `Password` sai da lista de colunas, e as colunas de
credencial do core (`User.password_hash`, `API Key.secret_hash`) entram numa lista explícita
de redação. Pedir uma delas pelo nome é erro de validação, não omissão silenciosa. Um hash
não é a senha, mas identifica a credencial, e um export é um arquivo que viaja.

**11. Anexo ausente é achado, não exceção.** Uma linha de `File` cuja bytes sumiram do disco
vem marcada `"missing": true`. Abortar a exportação inteira esconderia justamente o que a
reconciliação precisa ver.

**12. Anexos são lidos ignorando a permissão do `File`.** `File` é legível pelo dono, então
uma consulta comum esconderia o anexo de um colega num documento que o usuário pode ler. É a
mesma pergunta que `/private/files` responde, e responde pela permissão do documento
anexado — que aqueles nomes já passaram.

**13. `--user` na CLI.** Exportar como alguém exercita o modelo de permissões em vez de
presumi-lo; com `--all`, um DocType que aquele usuário não pode exportar vira linha em
`skipped` no manifesto, não fim de execução.

## O que ficou de fora

- Exportação assíncrona por job gerando um `File` (é OPS-07 no inventário).
- Zip dos anexos pelo HTTP: o manifesto vai, os bytes ficam para a CLI.
- Importação (DAT-01), que é a contraparte deste trabalho.
- Rótulos traduzidos e títulos de Link nas células: a exportação carrega valor cru, que é o
  que a reconciliação compara. O CSV da página no Desk continua resolvendo títulos.
