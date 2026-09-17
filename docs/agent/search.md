# Global search

The desk opens a search palette with **Mod+K** (⌘K on macOS, Ctrl+K elsewhere), from the
**Search** button at the top of the sidebar, or from the magnifier on the mobile top bar. It
lists two kinds of results:

- **DocTypes** whose label or name contain the text, as "Go to list" entries. They come from
  the boot's DocTypes, which are already only the ones the user may read.
- **Documents** found on the server by `GET /api/search/global`.

Arrow keys move through the results, Enter opens the highlighted one (or the first), and Esc
closes the palette. Each result opens in the workspace that owns its DocType.

## Which DocTypes are searched

A DocType is searched when it declares a `titleField` or `searchFields`. It is never searched
when it is a child table (`isChild`) or a Single (`isSingle`). `globalSearch` overrides the default:

```ts
export default defineDoctype({
  name: "Contract",
  titleField: "contract_title",
  searchFields: ["customer", "contract_no"],
  globalSearch: true,   // the default here, since there is a titleField
  // ...
});
```

- `globalSearch: false` leaves a DocType out even though it has a title. The Core logs do this:
  Audit Event, Document Share, Email Delivery, User Permission and Webhook Delivery.
- `globalSearch: true` searches a DocType that has neither a title nor search fields. The search
  then matches its `name` only.
- An app can change the flag on another app's DocType with `extendDoctype`
  (`doctype: { globalSearch: false }`), as it does any other DocType property.

The matched columns are the same as a Link field's search: `name`, then `titleField`, then each
of `searchFields`. Keep them short, indexed text, such as codes, titles and names. A long text
field in `searchFields` makes every search scan it. A matched column that is a **Link** also
matches the linked document's title and search fields, through an `EXISTS` subquery over the
target table — correct, but a query the target's own indexes have to carry.

## The endpoint

```
GET /api/search/global?txt=<text>&limit=<n>
```

- **Sign-in:** required. A guest gets 401.
- **`txt`:** trimmed, then 2 to 140 characters. Anything else is a `ValidationError` (417).
- **`limit`:** defaults to 20 and is capped at 50. Each DocType contributes at most 5 hits, taken
  **before** the ranking: its first 5 rows in list order (`sortField`/`sortOrder`, else `modified`
  descending). So an exact match is lost when its DocType has more than 5 matches ahead of it.
- **Response:** `data` is a list of `{ doctype, label, name, title }`.
  - `label` is the DocType's translated label.
  - `title` is the title field's value, or the name when there is none.
- **Matching:** a case- and accent-insensitive substring match (`ILIKE` over the same folding as a
  list filter), so `cafe` finds `Café`. `%` and `_` in the text are not escaped, so they reach the
  `ILIKE` as its wildcards.
- **Ranking:** hits whose name or title equal the text come first, then those that start with it,
  then every other match. Within a rank, hits keep DocType order — alphabetical by DocType **name**,
  not by label — and then the list order.

From desk code, `ddcore.search.global(txt, limit?)` calls the same endpoint.

## Authorization

Each DocType is searched through the ordinary list query (`GetList`), so a search returns
exactly what the user's lists would:

- **Roles:** a DocType the user may not read is skipped silently, not reported.
- **Scopes (`User Permission`):** they narrow the results, as they narrow a list (see `scopes`).
- **Shares:** a shared document appears for its recipient even without a role grant (see `sharing`).
- **Field levels:** `titleField` and `searchFields` must be permission level 0 (checked on load), so
  a search never matches a restricted field (see `field-permissions`).
- **`permissionQuery` and `ifOwner` rules:** they apply as they do to a list.

There is no search index. Nothing is kept in sync on save, and nothing can go stale after a
permission change. The cost is one query per searchable DocType the user can read, on every
search, so a site with many large searchable DocTypes should keep their search fields indexed
(`searchIndex: true`) or opt the big logs out.

## Limits

- No relevance scoring beyond exact, prefix and substring; no full-text search of long text,
  child tables or attachments.
- Reports, workspaces and pages are not searched, only DocTypes and documents.
- The palette does not remember recent searches.
