# Virtual DocTypes

A virtual DocType has no table of its own. Its rows are the union of other DocTypes, its
**sources**. It exists for the "either one of these" relation: a supplier that is a person or a
company, a payee that is an employee or a vendor. The sources stay separate DocTypes, each with
its own fields and rules, and the virtual one gives them one list, one search and one Link
target.

`virtual` declares one:

```ts
export default defineDoctype({
  name: "Party",
  label: "Party",
  titleField: "party_name",
  searchFields: ["party_name", "tax_id"],
  linkSubtitle: ["tax_id"],
  virtual: {
    sources: [
      { doctype: "Person", fields: { party_name: "person_name", tax_id: "cpf", city: "city" } },
      { doctype: "Organization", fields: { party_name: "organization_name", tax_id: "cnpj", city: "city" } },
    ],
  },
  fields: [
    { fieldname: "party_name", fieldtype: "Data", label: "Name", inListView: true },
    { fieldname: "tax_id", fieldtype: "Data", label: "Tax ID", inListView: true },
    { fieldname: "city", fieldtype: "Link", options: "City", label: "City", inStandardFilter: true },
  ],
  permissions: [{ role: "Sales User", read: true, report: true, export: true }],
});
```

Any relation that accepts either source links to it like any other DocType:

```ts
{ fieldname: "party", fieldtype: "Link", options: "Party", label: "Supplier", reqd: true }
```

## Ids

A row's id is `"<Source DocType>:<source id>"`, for example `Person:12345678900` or
`Organization:acme`. Two sources may use the same id and the rows still stay apart. A Link to a
virtual DocType stores the same string. The engine splits it at the first `:`, so a source's
name cannot contain one.

## The mapping

Each source maps a virtual fieldname to one of its own fieldnames:

- A virtual field that a source does not map reads as `null` on that source's rows.
- The column types must match (`Data` ↔ `Select` is fine, since both are text; `Data` ↔ `Date`
  is not).
- A Link must map to a Link to the same DocType.
- A standard column (`creation`, `owner`…) may be mapped when the types match.

A `source_doctype` field (Data, in the list view and the standard filters) is added unless you
declare it. It holds the source's name, so `{ source_doctype: "Organization" }` filters by
kind.

All of the following are refused at load:

- **In the declaration:**
  - `isChild`, `isSingle`, `isTree`, `submittable`, `trackChanges`, `allowRename`,
    `idGeneration`, `uniqueKeys` or `renamedFrom`;
  - a field that is a Table, Table MultiSelect, Dynamic Link, Password or Vault;
  - a field above permission level 0, or one that is `unique` or `fetchFrom`;
  - a permission row granting anything other than `read`, `select`, `report` or `export`.
- **In the sources:**
  - a source that is missing, virtual, a child table or a Single;
  - a mapping that names a field that does not exist, or whose type does not match.

## Reads and permissions

`getList`, `getCount`, `getDoc`, Link search, reports and export all read a virtual DocType
through the ordinary list path. Its `FROM` is a `UNION ALL` with one branch per source:

- **The virtual DocType's own `permissions`** decide who may open it at all, as for any
  DocType.
- **Each branch is authorised as a list of its source would be:** role rows and `ifOwner`,
  `permissionQuery`, User Permission scopes, shares and portal rules. A source the user cannot
  read contributes no rows. A user therefore sees a row exactly when they can read the source
  document.
- **Field levels are the source's.** A mapped field above the user's level on its source reads
  as `null`. Filtering or sorting on it cannot reveal the value, because the filter runs over the
  `null`.
- **Filters, sorting, `groupBy` and paging** run over the union's columns like columns of a
  table. A `like` on a Link column also matches the target's title, as usual.

A controller's `hasPermission` on the virtual DocType applies to single documents only, as it
does everywhere else. Controller hooks have no effect, because nothing is ever written.

## Links to a virtual DocType

On save, a Link value must name one of the sources, and that source must hold the document.
This is the same existence check every Link gets; whether the user can read the target is not
checked. `fetchFrom` works through the Link, for example `fetchFrom: "party.party_name"`.

- **Deleting** a source document that is still referenced is refused with the usual "is linked
  from" error.
- **Renaming** a source document rewrites `Person:old` to `Person:new` in every Link to a
  virtual DocType fed by `Person`.
- **Renaming a source DocType** (`renamedFrom`) rewrites the prefix the same way.

## Writes

Insert, save, `dbSet`, delete, rename, spreadsheet import and `import` are all refused with a
validation error, including through the REST API. To change a row, edit its source document.

A virtual DocType cannot be:

- shared;
- watched by a notification or a webhook;
- used by a portal page;
- named as the `allow` of a User Permission. Restrict its sources instead.

## The Desk

- The list, Cards and the other list views show the union. The Source column and filter pick a
  source DocType. New, Delete and Import are hidden, because nothing grants create, delete or
  import.
- Opening a row, a Link cell, a Link control's ↗ or a search hit sends you to the **source
  document**: `/app/<ws>/Party/Person:123` redirects to the Person form, in the workspace that
  owns Person. `…/Party/new` returns to the list. A virtual DocType has no form of its own.
- A Link control pointing at a virtual DocType is a single picker across the sources. Each
  option's subtitle starts with its source's label, then the source's own id.
- The boot payload lists every virtual DocType with its sources (`virtuals`), whether or not the
  reader can open it. That lets a Link to one open the source even for someone who only reads the
  source.

## Search

A virtual DocType stays out of global search unless it sets `globalSearch: true`. Its rows are
its sources' documents, which the search already finds. Link search works as usual, over `id`,
`titleField` and `searchFields`.

## Migrations

A virtual DocType creates no table. If a regular DocType becomes virtual, its old table is
treated as an orphan: `migrate --prune` drops it when it is empty and refuses when it still
holds rows. Move the data first.

## Limits

- Sources are local DocTypes only. External sources are not supported.
- There are no aggregates or joins across sources beyond what the list path does over the union.
- Filters and sorting on a field that only some sources map treat the other sources' rows as
  `null`.
- A User Permission over a source does not narrow a Link to the virtual DocType that points at
  that source. It narrows only the rows of the virtual list itself.
