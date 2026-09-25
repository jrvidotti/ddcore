# Virtual DocTypes Design Specification

**Date:** 2026-09-24  
**Status:** Implemented  
**Topic:** Virtual DocTypes as a union of local DocTypes — DAT-07 (issue #17; trees delivered earlier, external sources out of scope)  

---

## 1. Overview & Objectives

Some relations accept either of two DocTypes that are kept apart on purpose. A supplier is a
natural person or a company; a payee is an employee or a vendor. Before this work an app had three
choices, and each re-implemented something the core already owns for real DocTypes:

- a Select plus a Dynamic Link, with copied fields;
- an index DocType kept in sync by hooks, with its permissions mirrored by hand;
- a report, which cannot be linked to or searched.

Objectives:

1. Declare a union (`virtual.sources`, with a field mapping) and get a list, search and Link target.
2. Authorise every row exactly as its source document is authorised.
3. Link to the union with one picker and one value, validated and kept intact by deletes and renames.
4. Refuse every write, because there is no table to write to.

## 2. Identity

A row's id is `"<Source>:<source id>"`. A Link to the union stores the same string. Code splits it
at the first `:`, so a source's name cannot contain one; its id may.

The alternative was a pair: the Link column plus a hidden type column, like a typed Dynamic Link.
It would have touched the schema planner, forms, import and export for every Link to a union. The
single string keeps a Link a Link.

## 3. Metadata contract

- **Declaration.** `DocType.Virtual{Sources []{DocType, Fields map[virtual]source}}`.
- **Injected field.** `Registry.ApplyVirtual` runs next to `ApplyTrees` and adds
  `source_doctype` (Data). Its `_doctype` suffix is what makes the desk render it as a DocType
  select.
- **Load-time refusals (`validateVirtual`).**
  - Flags: child, Single, tree, submittable, `trackChanges`, `allowRename`, `idGeneration`,
    `renamedFrom`, `uniqueKeys`.
  - Fields: Table, Table MultiSelect, Dynamic Link, Password and Vault fields; any field above
    level 0, `unique`, `fetchFrom` or `convert`.
  - Permissions: rows granting anything beyond read, select, report and export.
  - Sources: a missing, virtual, child or Single source, or a duplicated one.
  - Mapping: a key that is not a field of the union, or a mapped field that does not exist or has
    no readable column.
  - Types: differing column types, or a Link mapped to a Link with a different target.
- **Extensions.** `virtual` is not in `DoctypeProps`, so an extension cannot introduce it.
- **Global search** leaves the union out unless it sets `globalSearch: true`, since its sources
  already appear.

## 4. Reads

`GetList` keeps its shape. For a virtual DocType, the `FROM` becomes `virtualRelation`, a
parenthesised `UNION ALL` with one branch per source. Each branch aliases its table as `"t"`, so the
ordinary column resolver and `filterSQL` render that source's permission filters inside the branch.

- **Who may list the union at all** is decided by the union's own role rows.
- **Which rows appear** is decided per branch, by the source's `permissionFilters`: roles,
  `ifOwner`, `permissionQuery`, scopes, shares and portal rules. A source the user cannot read is
  dropped. Under `IgnorePermissions` only the scopes apply, and under the ctx flag nothing does,
  both as for a table.
- **Field levels.** A mapped column the user cannot read on its source becomes `CAST(NULL AS …)`.
  The caller's filters run over the outer relation, so filtering on a masked field sees the null
  and cannot act as an oracle for its value.
- **Everything else is shared.** Filters, order, grouping and paging apply outside the union, and
  Postgres pushes them down into each branch. `Count`, `LinkSearch`, `GetValue` and export keyset
  paging need no change. `getDoc` reads one row through `GetList`.
- **Engine-internal reads.** `idExists`, `LinkTitles` and `ResolveLinkTitles` resolve through the
  source named in the id. The title-matching `EXISTS` of a `like` on a Link, and the import's
  dangling-link check, read an unauthorised union of ids, as they read a table.

## 5. Links, deletes, renames

- **Validation.** `checkLinks` needs no change: `idExists` on a union checks the source prefix
  against the declaration and then the source's row. This is existence only, like every Link.
- **Delete.** `checkLinksBeforeDelete` also looks for `<DocType>:<id>` in Links to unions fed by
  the DocType.
- **Rename of a document.** `moveID` rewrites the value in the same Links.
- **Rename of a source DocType.** `sweepDocTypeRename` rewrites the `<Old>:` prefix in those Links.
- **Loops over every DocType** skip unions: delete, rename, the legacy admin rename and the
  rename-reference sweep.

## 6. Writes and other subsystems

`refuseVirtual` runs after `checkWritable` in `Insert`, `DBSet`, `Delete`, `Rename`, `ImportDoc` and
data-import planning. Save, Submit, Cancel and fixtures reach one of those. The HTTP `childGuard`
refuses at the border too.

The following refuse a virtual DocType:

- sharing;
- notification rules;
- webhooks;
- portal pages and portal identities;
- the import plan;
- a User Permission whose `allow` names one.

The schema planner leaves unions out entirely. A table left behind by a DocType that became
virtual is an orphan, handled by `--prune` like any other.

## 7. Desk

The boot payload carries `virtuals` (union → source names) for every union, readable or not.
`FormView` sends a union's record URL to the source form in the source's workspace, and `new` back
to the list. That one redirect covers every list view's row link, Link cells, a Link control's ↗
and search hits, so no view had to learn about unions.

The Link picker prefixes each option's subtitle with the source's label and shows the source's own
id. New, Delete, Import, calendar "+" and Kanban drag were already gated on permissions the union
never grants.

## 8. Testing strategy

- **`internal/meta`:** the injected field, every refusal, a standard-column mapping, and the
  separator in a source name.
- **`internal/engine`:**
  - the union with filters, order, paging, grouping, `like` on a Link, count and Link search;
  - per-source roles, `permissionQuery`, shares, scopes and field masking;
  - `getDoc` errors;
  - Link validation, `fetchFrom` and titles;
  - delete blocked, rename followed;
  - every write refused;
  - export paging.
- **`internal/acceptance`:** `TestVirtualDocTypes` over the `Work Item` fixture (Project ∪ Task):
  meta and boot, list and filter, count, Link search, titles and get, POST, PUT and DELETE
  refused, and a contributor's masked `budget`.
- **Desk vitest:** `virtual.ts`, which covers splitting ids, the redirect and the picker subtitle.
