# Fieldtypes e propriedades

| Fieldtype | Coluna Postgres | Observações |
|---|---|---|
| Data | text | `length` limita o input |
| Email | text | um endereço de e-mail; espaços externos são removidos e o formato é validado no desk e no servidor |
| Small Text / Text / Text Editor | text | textarea (2 / 5 linhas) |
| Int | bigint | |
| Float | double precision | `precision` só afeta exibição |
| Currency | numeric(21,9) | exibido como R$ (moeda do cerne.json) |
| Percent | numeric(21,9) | exibido com % |
| Check | boolean | default `false` |
| Date | date | valor "YYYY-MM-DD" |
| Month | date | valor "YYYY-MM-01", exibido e editado como "mm/aaaa" |
| Datetime | timestamptz | valor ISO |
| Time | time | "HH:MM:SS" |
| Select | text | `options: ["A", "B"]`; validado no servidor |
| Link | text | `options: "DocType"`; existência validada; índice automático |
| Dynamic Link | text | `options: "<campo que guarda o DocType>"` |
| Table | (tabela filha) | `options: "DocType filho"` com `isChild: true`; `gridEditMode: "dialog"` desativa edição inline |
| Attach | text | URL do arquivo (`/files/..` ou `/private/files/..`) |
| JSON | jsonb | |
| Password | text | não é hasheado automaticamente |
| Section Break / Column Break / Tab Break | — | layout; `label`, `collapsible`, `dependsOn` em Section |
| HTML | — | `options` é o HTML renderizado |

## Propriedades de campo

`fieldname, fieldtype, label, options, reqd, unique, default, readOnly, hidden, fetchFrom, dependsOn,
readOnlyDependsOn, mandatoryDependsOn, allowOnSubmit, inListView, inStandardFilter, searchIndex,
length, precision, description, columns (largura no grid 1–12), gridEditMode (`"inline"` padrão ou `"dialog"`), collapsible, bold`

- `default`: valor literal, ou `"Today"` para Date/Datetime, `"__user"` para o usuário atual.
- `fetchFrom: "imovel.proprietario"`: copia do documento vinculado ao salvar. Se `readOnly`, sempre sobrescreve; senão só preenche quando vazio.
- `dependsOn`, `readOnlyDependsOn`, `mandatoryDependsOn`: expressão JS sobre `doc` (`"doc.tipo == 'PJ'"`) ou nome de campo (truthy). Avaliadas no desk **e** no servidor.

## Propriedades do DocType

```ts
defineDoctype({
  name: "Contrato", module: "Comercial", label: "Contrato",
  naming: { series: "CTR-.YYYY.-.####" } | { field: "sigla" } | { format: "{indice}-{competencia}" } | { hash: true } | { prompt: true },
  submittable: true, isChild: false, trackChanges: true, allowRename: true,
  titleField: "nome", searchFields: ["nome", "cpf"], sortField: "modified", sortOrder: "desc", icon: "building-2",
  fields: [...],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true, report: true, export: true, ifOwner: false }],
});
```

Séries: `.YYYY.`, `.YY.`, `.MM.`, `.DD.`, `.####.` (contador com zeros), `.{campo}.`. Um campo `naming_series` (Select) permite o usuário escolher a série.
