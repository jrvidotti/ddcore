# Tree DocTypes Design Specification

**Date:** 2026-09-22  
**Status:** Implemented  
**Topic:** Hierarchical DocTypes — DAT-07 (trees; virtual DocTypes out of scope)  

---

## 1. Overview & Objectives

DAT-07 covers two unrelated capabilities: hierarchies and virtual DocTypes. This
specification takes the first only; virtual DocTypes stay in the demand-driven backlog.

Before this work an app could declare a Link pointing at its own DocType, and nothing
else followed: no rule kept a document from becoming its own ancestor, no query walked the
branch, and no view showed the shape. Worse, `descendants of` was already accepted as a
filter operator and compiled into `=` (`internal/db/query.go`), so a hierarchy question
returned a wrong answer with no error at all.

Objectives:

1. Declare a hierarchy (`isTree`) and get the two fields that hold it.
2. Keep it a tree on every write path, including the ones that skip validation.
3. Query a branch, composing with every access rule the framework already applies.
4. Make a User Permission over a hierarchy mean what a reader expects: the branch.
5. Show it, one level at a time, without loading the whole tree.

## 2. Storage model

**Adjacency list only**: the parent column plus `is_group`. Subtree and ancestor questions
are answered by `WITH RECURSIVE`, not by stored bounds.

The alternative was Frappe's nested set (`lft`/`rgt`). It answers a subtree in one range
scan, but every insert or move rewrites half the table under a global lock, and it needs a
rebuild command for when the bounds drift. Adjacency cannot drift: the parent column *is*
the hierarchy, so there is no second representation to keep true. The cost is that a
branch query walks the branch, which is the trade this framework prefers — correctness that
needs no maintenance command. This is also why no `lft`/`rgt` compatibility exists for
importing Frappe tree data: such an import maps the parent and `is_group` instead.

## 3. Metadata contract

`isTree: true`, with optional `parentField` (default `parent_<snake(name)>`, Frappe's
convention). `Registry.ApplyTrees` runs before `ApplyExtensions` and appends the parent
Link and the `is_group` Check when the DocType does not declare them, and writes the
resolved `parentField` back so every consumer reads it by name. Because the fields are
ordinary fields by the time anything else looks, typings, the i18n extractor, the schema
planner and the desk meta needed no changes at all.

`validateTree` refuses, at load: a tree that is also a child table, a Single or
submittable; `parentField` without `isTree`; a parent field named `id`, `parent`,
`parenttype`, `parentfield`, `is_group` or a standard column; a parent field that is not a
Link to its own DocType, or is `reqd`, `unique`, `fetchFrom` or above permission level 0;
an `is_group` that is not a Check or is above level 0. The two columns are read by every
recursive query, so a permission level on either would make the hierarchy unreadable
rather than confidential.

`isTree` and `parentField` are not in `DoctypeProps`: like `isSingle`, they decide what the
document is, so an extension cannot introduce them into another app's DocType.

## 4. Write integrity

`checkTree` runs after `beforeSave` on insert and save (so a parent set by a hook is
checked like one the caller sent), and from `DBSet` when the write touches either column —
the one place where "no hooks, no validation" would corrupt the structure rather than a
value. `checkTreeDelete` runs after `onTrash` and before the generic link check, so the
message names the children; it ignores `force`, because forcing would orphan them.

**Concurrency.** A cycle needs two writers, each checking against a hierarchy the other is
about to change; a row lock cannot see that, because the two documents are different rows.
So the unit of exclusion is the hierarchy: `pg_advisory_xact_lock` on the DocType name,
taken by `Insert`, `Save`, `Delete`, `Rename` and the structural `DBSet` **before** any row
lock, which also fixes the lock order and rules out a deadlock against `Save`'s
`FOR UPDATE`.

The ancestor walk uses `UNION`, not `UNION ALL`, so data that is already cyclic (raw SQL,
or a hierarchy built before `isTree`) terminates the query instead of spinning.

## 5. Query operators

`db.Filter` gains `Tree *TreeRef{Table, ParentCol}`, never parsed from a request:
`filterSQL` resolves it from the meta, for the DocType's own `id` and for a Link whose
target is a tree, on the DocType's own filters and on child-table filters. A tree operator
without it is refused by name. `Builder.treeSet` renders the recursive subquery; positive
operators become `col IN (…)` and negative ones `(col IS NULL OR col = '' OR col NOT IN
(…))`, which is Frappe's ifnull reading — a row with no value is outside the branch.

Two rebuild sites in `filterSQL` were dropping unknown `db.Filter` fields (the `byChild`
collection and the `IfField` branch) and now copy the filter; the child EXISTS resolves the
parent key through `col("id")` rather than hard-coding `"t".id`.

## 6. Scopes (SEC-01)

`strictScopeFilters` emits `descendants of (inclusive)` in place of `in` when the rule's
`allow` DocType is a tree, which covers the tree itself, every Link into it and a Dynamic
Link whose selector names it, and therefore lists, counts, link search and export.
`checkUserPermissionsFor` gains `withinScope`: an allowed value, or a value one of whose
ancestors is allowed, memoised per request. For the tree's own document the check also
accepts the in-memory parent, which is what lets a scoped user create a child under an
allowed node (the document has no row yet) while still refusing a move out of the branch.

## 7. The level endpoint and the Desk

`Ctx.TreeChildren` reads through `GetList` twice — the level, then a grouped count — so
permissions, scopes, shares and field levels apply to the nodes *and* to the child counts:
a user never learns how many children they cannot see. For the roots, a readable node whose
parent is unreadable is promoted to a root, so a branch granted in the middle has a top;
that scan is bounded (`treeRootCandidates`) and skipped entirely when the user has no
permission filters at all. `GET /api/tree/{doctype}` is a thin wrapper.

In the desk, `"tree"` joins `DeskViewMode` in both SDK copies, is allowed only for a tree
DocType and comes first for one. `tree-state.ts` holds the whole model (loaded levels, open
set, depth-first flattening, the refresh set, the parent-picker filters) so it is tested
without a DOM; `TreeView.svelte` renders it and fetches one level per branch. The parent
picker applies `is_group = 1`, `id != self` and `not descendants of self` unless the app
set its own query.

## 8. Testing strategy

- `internal/meta`: injection, every refusal, `uniqueKeys` allowed, extension refusal.
- `internal/db`: the SQL and arguments of each operator, empty values, no-`Tree` refusal.
- `internal/engine`: integrity (non-group parent, self-parent, cycle, leaf conversion,
  delete with children under `force`, `dbSet`, rename), the operators over `id`, a Link and
  a child table, a query over deliberately cyclic rows, two concurrent opposite moves, and
  the SEC-01 branch behaviour including promoted roots.
- Desk vitest: `tree-state` and the view resolution.
- `internal/acceptance`: `TestTrees` over real HTTP — meta, `/api/tree` levels and counts,
  the list filter, and the two refusals.
