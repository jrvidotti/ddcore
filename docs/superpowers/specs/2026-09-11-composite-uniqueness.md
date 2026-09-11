# Composite uniqueness — DAT-05

Written 2026-09-11. Delivers ROADMAP Stage 1 item 2.

## The problem

`unique` is a field property, so it can only ever describe one column. A business key that
spans several — `(customer, invoice_no)`, `(contract, category, period)` — had nowhere to be
declared, which left every app to re-implement it as a `validate` hook doing a SELECT. That is
not a guard: a SELECT cannot see a row another transaction has written and not committed, so
two concurrent requests both pass it and both write. The framework was asking apps to solve a
problem only the database can solve.

## The declaration

```ts
defineDoctype({
  name: "Invoice",
  uniqueKeys: [{ name: "customer_invoice_no", fields: ["customer", "invoice_no"] }],
  fields: [...],
});
```

Each key becomes one partial unique index:

```sql
CREATE UNIQUE INDEX "tab_invoice_uk_customer_invoice_no"
    ON "tab_invoice" ("customer", "invoice_no")
 WHERE "customer" IS NOT NULL AND "customer" <> ''
   AND "invoice_no" IS NOT NULL AND "invoice_no" <> '';
```

### Why the key is named

The planner tells indexes apart **by name**. Deriving a name from the field list would mean
that reordering the list, or renaming a component, renames the index — and an index that has
to be renamed is an index the diff first has to recognise, which is the machinery
`renamedFrom` exists to avoid. A name the author writes is stable through both, and it gives
a duplicate error something to be about that is not a column list.

`uk_` is the infix, and it is load-bearing: it is a namespace the framework owns, which is
what makes it safe to drop every index in it that no declaration claims.

### Null and empty

A row is constrained only when **every** component has a value. This is not a new rule — it is
`uniquePredicate`, the same rule per-field `unique` has followed since B14, applied per
component and ANDed. An incomplete business key describes an unfinished document, not a
duplicate one, and a framework with two answers to "what counts as absent" would be worse than
one with a debatable answer.

The runtime pre-check mirrors the predicate exactly rather than reusing `isEmpty`, because
`isEmpty` calls `false` empty while a boolean column's predicate is only `IS NOT NULL`. Reusing
it would let the pre-check wave through a row the index then refuses.

## Enforcement, in two layers

**`checkUniqueKeys`** runs in `validate` and is the readable half: it can name the document
already holding the key, because it runs before anything has failed.

**The index** is the half that holds. When two writers race, both pre-checks pass and the
second write raises `23505`. That path now reads Postgres's `ConstraintName` — through
`db.UniqueViolation`, an `errors.As` on `*pgconn.PgError` — instead of matching `"duplicate
key"` as a substring, so it can map the violated index back to the declaration and say the
same thing the pre-check would have.

It says slightly less: it cannot name the other document. A `23505` aborts the transaction, so
there is no query left to ask with. Hence two message keys rather than one — `{0} already
exists on {1} {2}` before the write, `{0} already exists` after it.

Reading the constraint name also improved the messages that were already there. `writeInsert`
used to answer every violation as if it were the primary key, and `writeUpdate` answered all
of them with `Duplicate value in {0}`; both now name the field or the key that was actually
violated, and `Duplicate value in {0}` survives as the answer for an index this DocType does
not own the name of.

## Three planner defects fixed on the way

1. **Orphan indexes were never dropped.** The index diff walks only the names the meta *wants*,
   and `--prune` covers columns and tables. A removed declaration would have left its unique
   index enforcing a constraint nobody could read a declaration for. The sweep drops every
   index in the `<table>_uk_` namespace that is not wanted — scoped by `idxRow.table`, not by a
   name prefix, so `tab_pedido_` can never reach into `tab_pedido_item_`. It needs no `--prune`
   and is not `Destructive`, because dropping an index loses no row.

2. **A renamed component would have rebuilt the index.** Postgres carries an index across a
   `RENAME COLUMN` on its own, but the in-memory catalog still holds the pre-rename text, so
   the comparison would have seen a difference and emitted DROP + CREATE — the expensive way to
   change nothing, and on a unique index a window with no constraint. `planRenames` now also
   reports, per table, what each renamed column used to be called, and the compound index is
   rendered against those names before being compared. `renamedIdx` does the same job for
   single-field indexes; it could not speak for an index no single field owns.

3. **A behaviour change worth naming:** a field literally called `uk_*` that loses `unique: true`
   now has its orphaned index dropped, where before it leaked. That is a fix, but it is outside
   this feature's stated scope.

## What is deliberately not here

- **Child tables.** A key scoped by `parent` is a different question — whether `parent` joins
  the key implicitly — and nothing needs it yet.
- **`extendDoctype`.** `DoctypeProps` is a closed list that excludes everything with a schema
  effect. One app silently adding a business key to another app's DocType is exactly what that
  list exists to prevent.
- **Pre-migration duplicate detection.** Creating a key over a table that already holds
  duplicates fails with Postgres's own error and applies nothing. `migrations.md` gives the
  query to find them. A refusal with a remedy would be better; it is not free, and the failure
  is already safe and already legible.
- **A desk-side check.** It would duplicate the skip rule and still not be the guard.

## Known residual

Removing `unique: true` from a field still leaves `tab_<table>_<field>` alive forever. This
delivery fixes the leak only inside the `uk_` namespace; sweeping per-field indexes means
deciding what to do about hand-made indexes and about `tab_x_pkey`, which is a wider blast
radius than DAT-05 should take. It belongs in a follow-up.

Idempotence rests on `normalizeSQL` turning our predicate and Postgres's rendering of it into
the same string — true because Postgres flattens nested `AND`s, keeps conjunct order and spells
a text literal `''::text`, but guaranteed by nothing. `TestSameIndexAcceptsPostgresRenderingOfACompositePredicate`
pins it without a database, because the failure mode otherwise is a migration that never
settles.

## Acceptance

`TestTwoTransactionsCannotCreateTheSameBusinessKey` is the one the roadmap asks for. Writer A
inserts and holds its transaction open; writer B then inserts the same key, passes its
pre-check — A has not committed, so B's snapshot is clean — and blocks on the index. The test
waits for that lock in `pg_locks` rather than for a duration, so the race is forced rather than
hoped for, then releases A. B must come back with a `DuplicateEntryError`, and the assertion is
on the message **key**: seeing the pre-check's wording there would mean the race never happened
and the test proved nothing.
