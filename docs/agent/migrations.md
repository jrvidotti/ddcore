# Migrations: renames, conversions and patches

`ddcore migrate` brings the database to the meta. It is additive by default, it never
guesses at a conversion that could lose data, and it runs everything in **one
transaction** — a failure anywhere leaves the database exactly as it was.

## The order, and why it matters

| # | Step |
|---|---|
| 1 | the framework's own tables (`ddcore_patch`, `ddcore_rename`, …) |
| 2 | `beforeSchema` patches — the old shape is still in place |
| 3 | the plan, computed **after** step 2 |
| 4 | the additive DDL: renames, new tables and columns, declared conversions, indexes |
| 5 | the reference sweep for each DocType renamed in step 4 |
| 6 | installing new apps (roles, `afterInstall`), then fixtures |
| 7 | `afterSchema` patches — where a backfill lives |
| 8 | the drops, under `--prune`, **last** |
| 9 | `afterMigrate` |

Two of those placements are load-bearing. A `beforeSchema` patch can make the data fit
what the DDL is about to do, and the plan is computed after it so a patch that changes
the schema by hand is never overruled by a stale plan — **the planner is never a dead
end**. And the drops run after the patches, so a backfill can still read the column the
same migration removes.

`ddcore migrate --dry-run` prints the plan in that order, with the destructive statements
under their own heading. `ddcore doctor` adds what the meta no longer declares, which
patches are pending, and which rename declarations this database no longer needs.

## Renaming a field

Declare where it came from. The column is renamed, with its index, and the data stays put.

```ts
{ fieldname: "due_date", fieldtype: "Date", label: "Due date", renamedFrom: "vencimento" }
```

Without the declaration this is two unrelated facts to the planner — a new field and a
column nobody claims — so it adds an empty `due_date` and leaves `vencimento` orphaned.

A chain, for a database that skipped a release: `renamedFrom: ["a", "b"]`, oldest first.

Renaming `a` to `b` and giving the freed name to a **new** field in the same release works:
the rename runs before the add.

`renamedFrom` does not follow the places that name a field by string — `titleField`,
`sortField`, `searchFields`, `uniqueKeys`, `fetchFrom`, a form script. Change those yourself;
the meta refuses to load while any of them points at a field that is gone.

`uniqueKeys` is the one on that list where editing it is all you have to do: the rename is
planned as a rename, Postgres carries the key's index across it, and nothing is rebuilt.

## Renaming a DocType

```ts
defineDoctype({ name: "Rental Contract", renamedFrom: "Contrato", … })
```

The table is renamed with every index it owns, including the primary key, and then every
stored reference to the old **name** is repointed: child `parenttype`, every `Dynamic
Link` discriminator, and `File.attached_to_doctype`, `Comment.reference_doctype`,
`Version.ref_doctype`. A `Link`'s target comes from the meta, so it follows on its own —
and the meta refuses to load while a `Link` still points at the old name, which is what
stops a rename from being half done.

**Not swept, deliberately:** a queued job's `args`, which may embed the old name — drain
the queue before migrating; and `tab_version.data`, whose historical diffs embed it,
because rewriting history would be worse than leaving it.

## Changing a business key

A `uniqueKeys` entry is one partial unique index, named after the key rather than after its
fields — which is what decides the cost of each kind of change.

| Change | What migrate does |
|---|---|
| add a key | one `CREATE UNIQUE INDEX` |
| remove a key | the index is dropped, and **`--prune` is not needed** — dropping an index loses no row |
| rename the key | the old index is dropped and a new one created; no data moves |
| edit or reorder `fields` | drop and recreate under the same name |
| rename a *component field* | neither: the index follows the column |

Adding a key to a table that already holds duplicates is the one case with no guard in front
of it. `CREATE UNIQUE INDEX` fails with Postgres's own message — `could not create unique
index … Key (…)=(…) is duplicated` — and, because the whole migration is one transaction,
nothing at all is applied. Find them first:

```sql
SELECT customer, number, count(*)
  FROM tab_invoice
 WHERE customer IS NOT NULL AND customer <> ''
   AND number   IS NOT NULL AND number   <> ''
 GROUP BY customer, number HAVING count(*) > 1;
```

The `WHERE` matters: it is the index's own predicate, so a row with an empty component is not
a duplicate and will not block the index.

## Changing a fieldtype

A change is made without asking only when the new column type holds every value the old
one had, for every row, without consulting the data:

| From → to | Example | |
|---|---|---|
| anything → `text` | Currency → Data | automatic |
| `bigint` → `double precision` | Int → Float | automatic |
| `bigint` → `numeric(21,9)` | Int → Currency | automatic; overflows loudly past 10¹² |
| `double precision` → `numeric(21,9)` | Float → Currency | automatic |
| `text` → anything | Data → Currency | **refused** |
| `numeric(21,9)` → `double precision` | Currency → Float | **refused** — loses exactness |
| `timestamptz` ↔ `date` | Datetime ↔ Date | **refused** — timezone-dependent |
| no cast at all | Currency → Check, Date → Time | **refused** |

A refusal stops the whole migration and applies nothing. It is not pedantry: `text` to
`numeric` aborts on the first row Postgres cannot parse, and an aborted `ALTER` on a table
you cannot lock is an outage with no route forward.

To accept Postgres's own cast on data you have checked:

```ts
{ fieldname: "quantity", fieldtype: "Int", label: "Quantity", convert: { from: "Data" } }
```

`from` names the fieldtype whose column the database still has, so the declaration cannot
quietly authorise a different conversion two releases later. It carries no SQL: a
conversion a plain cast cannot express is what the route below is for.

## Expand → backfill → validate → contract

When the conversion is not a cast — money written as `"R$ 1.250,00"`, a `Select` whose
values changed meaning, a `Link` retargeted — move the data across releases instead.

**Release 1 — expand.** The new field arrives beside the old one. Nothing is removed.

```ts
{ fieldname: "amount_text", fieldtype: "Data",     label: "Amount", hidden: true, readOnly: true },
{ fieldname: "amount",      fieldtype: "Currency", label: "Amount" },
```

**Release 1 — backfill.** `patches/0004_backfill_amount.ts`:

```ts
import { definePatch } from "@ddcore/sdk";

export default definePatch({
  description: "amount_text (Data) → amount (Currency)",
  execute(ctx) {
    ctx.sql(`UPDATE tab_invoice
                SET amount = NULLIF(regexp_replace(amount_text, '[^0-9.-]', '', 'g'), '')::numeric
              WHERE amount_text IS NOT NULL AND amount IS NULL`);
  },
});
```

No `phase`: the default `afterSchema` is right, because the column has to exist first.

**Release 2 — validate and contract.** `amount_text` leaves the meta, and a `beforeSchema`
patch refuses to let the release proceed while any row is unconverted:

```ts
import { definePatch } from "@ddcore/sdk";

export default definePatch({
  phase: "beforeSchema",
  description: "validate the backfill, then retire amount_text",
  execute(ctx) {
    const left = ctx.sql(`SELECT count(*)::int AS n FROM tab_invoice
                           WHERE amount_text IS NOT NULL AND amount IS NULL`)[0].n;
    if (left) ddcore.throw(`${left} invoice(s) still have no amount`);
    ctx.sql(`ALTER TABLE tab_invoice DROP COLUMN amount_text`);
  },
});
```

Because it all runs in one transaction, a failed validation rolls the drop back with
everything else. There is no half-migrated state to recover from — and validation gating
contraction costs no machinery at all.

Note the patch drops the column itself rather than leaving it to `--prune`. That is the
contract step: `--prune` refuses to drop anything that still holds data, so discarding
data a backfill has already copied is a deliberate act, recorded in `ddcore_patch` with
the author's name on it, not a side effect of a flag.

## Patches

`patches/NNNN_name.ts`, discovered by the path and run once, in filename order, recorded in
`ddcore_patch` by `(app, name)`.

```ts
import { definePatch } from "@ddcore/sdk";

export default definePatch({
  phase: "beforeSchema",        // or "afterSchema", the default
  description: "one line, shown by migrate and doctor",
  execute(ctx) { ctx.sql("UPDATE …", ["arg"]); },
});
```

A bare `export function execute(ctx)` still works and is `afterSchema`.

- **`ctx.sql` is the only write-SQL in the framework**, and it exists only inside a patch.
  `ddcore.db.sql` is read-only everywhere else. A backfill must not be a row-by-row walk
  through the lifecycle: that rewrites `modified` on every row and writes a Version per
  row, an audit trail saying a person edited data a migration moved.
- **A patch does bounded work.** It holds the migration's locks for its whole duration. If
  the backfill is large, enqueue it (`ddcore.enqueue`) and let the next release's
  `beforeSchema` validation refuse to contract until it has finished.
- **Nothing at module scope.** A patch module is loaded with every other file; only
  `execute` is deferred to the migration.
- **Never delete a patch file.** It is the history, and `ddcore_patch` is what stops it
  running twice.
- **A first install records patches without running them.** A patch describes a change to
  data a new database does not have; seeding is what `afterInstall` and fixtures are for.

## Retiring a declaration

`renamedFrom` and `convert` go inert once they have been applied — they plan nothing when
the old state is gone. Deleting one **too early** is what breaks a site that has not
migrated yet: the old column becomes undeclared.

`ddcore doctor` answers this for the database in front of it:

```
renames:    2 applied, 2 retirable in this database
              Project.budget renamedFrom "orcamento"  → renamedFrom retirable here; …
```

No checkout can know about other sites, so the rule is: **keep the declaration until every
deployed site reports it retirable**, then delete it. And if you get it wrong, `--prune`
refuses to drop a column that still holds data and names it, rather than emptying it.

## `--prune`

Drops the columns and tables the meta no longer declares — and only the empty ones. A
non-empty one is refused, with the column named and two ways forward: declare the rename,
or drop it yourself in a `beforeSchema` patch. `ddcore doctor` lists the orphans either way.

`ddcore dev --auto-migrate` never prunes.

## Single DocTypes

`isSingle: true` creates a normal table with a fixed `singleton` primary key and a
CHECK constraint on identity and draft status. Field additions, declared field
renames, conversions, child tables and declared DocType renames use the normal
migration machinery. A DocType rename keeps the document identity `singleton`.

Switching an existing table between regular and Single metadata is refused. Plan
an explicit data migration instead; the framework never chooses a settings record
from existing rows or silently discards records. Migration remains idempotent.
