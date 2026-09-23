# Tree DocTypes

A tree DocType is a hierarchy: categories inside categories, territories inside regions,
an account plan. `isTree: true` declares one, and the framework gives it a
self-referencing Link for the parent, a `is_group` flag deciding which documents may
have children, query operators over the branch, scopes that follow it, and a Desk view
that opens one level at a time.

A self-referencing Link on its own is *not* a tree: nothing would stop a document from
becoming its own ancestor, and `descendants of` would have no hierarchy to walk.

## Declaring one

```ts
export default defineDoctype({
  name: "Territory",
  isTree: true,
  idGeneration: { field: "title" },
  titleField: "title",
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Sales Manager", read: true, create: true, write: true, delete: true }],
});
```

Two fields are added unless the DocType declares them itself:

| Field | Type | Meaning |
| --- | --- | --- |
| `parent_<snake(name)>` | Link to the DocType itself | The parent; empty on a root. `parentField` names it otherwise |
| `is_group` | Check | Whether this document may have children |

Declare either one yourself to place it in the layout, relabel it, or carry a
`renamedFrom`. What a declaration may not do is refused at load: the parent field must be
a `Link` to its own DocType, may not be `reqd` (a root has no parent), `unique`, fetched
from another field, or above permission level 0 — the tree queries read both columns, so
neither may be a restricted field (see `field-permissions`). `is_group` must be a `Check`.

A tree cannot be a child table, a Single or submittable. `uniqueKeys` work as usual —
`["parent_territory", "title"]` makes sibling titles unique, though roots are not
constrained by it, because a key skips rows where a component is empty.

An `extendDoctype` cannot turn another app's DocType into a tree, as with `isSingle`:
`isTree` and `parentField` decide what the document *is*.

Turning `isTree` on for an existing DocType is an ordinary migration: `migrate` adds the
two columns and indexes the parent, and every existing document becomes a root leaf.

## What the engine enforces

On every write, whichever path it comes through — the API, a controller, the Desk,
`dbSet`:

- the parent must exist and be a group; a leaf never takes children;
- a document is never its own parent, nor moved under one of its own descendants;
- a group with children stays a group (clear `is_group` only on an empty one);
- a document with children is not deleted — the message says so, and `force` does not
  waive it, because forcing would leave the children pointing at nothing.

The checks run after `beforeSave`, so a parent a hook sets is checked like one the caller
sent. Renaming a document moves its children with it: the parent column is a Link, and
rename rewrites every Link to the renamed document.

Structural writes on one tree DocType are **serialised** by an advisory lock held to the
end of the transaction. Two transactions moving A under B and B under A at the same time
would each check against a hierarchy the other is about to change, and a row lock cannot
see that — so the whole hierarchy is the unit of exclusion. Write-heavy hierarchies will
feel it; master data will not.

Roots are not limited: any number of documents may have no parent. An app that wants a
single "All Territories" root enforces it in `validate`.

## Querying a branch

Five operators walk the hierarchy. They apply to a tree's own `id` and to any `Link`
pointing at a tree, in `getList`, in REST filters, in reports and in the Desk:

| Operator | Matches |
| --- | --- |
| `descendants of` | everything below the value, excluding it |
| `descendants of (inclusive)` | the value and everything below it |
| `not descendants of` | the rest, including rows with no value |
| `ancestors of` | everything above the value, excluding it |
| `not ancestors of` | the rest, including rows with no value |

```ts
// every territory inside Brazil
ddcore.db.getList("Territory", { filters: [["id", "descendants of", "Brazil"]] });
// every customer in Brazil or in any territory under it
ddcore.db.getList("Customer", { filters: [["territory", "descendants of (inclusive)", "Brazil"]] });
```

The value is one id, or a list of them; a list matches the union of their branches, and an
empty list matches nothing. A single string is one id and is never split on commas.
Naming one of these operators on a field that is neither a tree's `id` nor a Link to a
tree is refused.

They compile to one recursive query and compose with everything else a list applies —
roles, `permissionQuery`, scopes, shares and field permissions. Two notes on cost and
reach: nothing is materialised, so a filter over a very large or very deep branch is a
real walk of it; and the walk itself does not check row permissions, so a user can learn
that a branch *is deep* through nodes they cannot read. Rows already cyclic (written by
raw SQL, or built before `isTree` was declared) end the walk instead of spinning, but
saving such a document fails until the cycle is repaired.

## Scopes follow the branch

A `User Permission` whose `allow` is a tree DocType covers the value it names **and
everything below it** (see `scopes`). "Territory: Brazil" lets the user read Brazil, South
and PR, and every document linked to any of them — on lists, counts, link search, direct
reads, and on write, where creating a document under an allowed node is allowed and moving
one out of the branch is refused.

## The tree view and `/api/tree`

A tree DocType's list page gets a **Tree** view, and opens on it. It expands one branch at
a time, links each node to its form, and offers *Add child* on a group. Filters and search
belong to the list view; switch views to use them.

```
GET /api/tree/{doctype}?parent=&limit=&fields=["acronym"]&order_by=title asc
→ { "nodes": [{ "id", "title", "parent", "is_group", "children", "values": { "acronym" } }], "hasMore": false }
```

A level lists groups first, then by the title field. `order_by` replaces that order outright
(`id` stays the tiebreak), and `fields` returns those fields under each node's `values`, which
exists only when fields were asked for. Both go through the list path's validation: an unknown
field is refused.

The view's label and order come from the `tree` option of `defineListView` (see `form-api`):

```ts
defineListView<TaskCategory>("Task Category", {
  tree: { title: "{acronym} - {title}", orderBy: "title asc" },
  // or: title: (r) => (r.acronym ? `${r.title} (${r.acronym})` : r.title), fields: ["acronym"]
});
```

A template's placeholders are fetched for it. An empty one renders as nothing and takes the
empty brackets and end separators it leaves with it, so the template above reads `Delivery` for
a node without an acronym. A function sees `id`, the title field and `fields`. A label that comes
out blank falls back to the title.

An empty `parent` asks for the roots. Every read goes through the ordinary list path, so
permissions, scopes and shares apply — including `children`, which counts only what the
user may read. A user who may read a node but not its parent sees that node as a root,
so a branch granted in the middle still has a top.

On a form, the parent field's picker offers groups only, and never the document itself or
one of its descendants. A `frm.setQuery` on the field replaces that.

## Limitations

- No drag-and-drop: a document is moved by editing its parent on the form.
- No filters or search inside the tree view, and a level is capped at 500 nodes.
- No `lft`/`rgt` (nested set) columns, and no import path for Frappe tree data carrying
  them; map `parent_<x>` and `is_group` instead.
- No depth-ordered ancestor helper or breadcrumb API: `ancestors of` returns a set.
- A tree cannot be submittable.
- `ddcore.db.sql` ignores the hierarchy, as it ignores scopes and shares.
- Nothing repairs rows made cyclic outside the engine.
