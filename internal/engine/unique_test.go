package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// DAT-05 — a compound business key, from the declaration an app writes down to
// the two transactions that must not both create it.

// setupWithKey replaces the fixture's Pessoa with one carrying a compound key,
// so the declaration travels the whole pipeline — TypeScript, goja, the meta,
// the planner — the way an app's would, rather than being poked into the
// registry afterwards.
func setupWithKey(t *testing.T) *Engine {
	t.Helper()
	return setupWith(t, map[string]string{
		"doctypes/pessoa/pessoa.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pessoa", idGeneration: { hash: true }, allowRename: true,
  uniqueKeys: [{ name: "tipo_codigo", fields: ["tipo", "codigo"] }],
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "tipo", fieldtype: "Select", label: "Tipo", options: ["PF", "PJ"], default: "PF" },
    { fieldname: "codigo", fieldtype: "Data", label: "Código" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
	})
}

// novaPessoa inserts one Pessoa and returns whatever the save said.
func novaPessoa(t *testing.T, e *Engine, tipo, codigo string) error {
	t.Helper()
	return e.Run(context.Background(), "Admin", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		doc, err := c.NewDoc("Pessoa", Doc{"nome": "X", "tipo": tipo, "codigo": codigo})
		if err != nil {
			return err
		}
		_, err = c.Insert(doc, SaveOpts{IgnorePermissions: true})
		return err
	})
}

func TestACompoundKeyRefusesADuplicate(t *testing.T) {
	e := setupWithKey(t)
	if err := novaPessoa(t, e, "PF", "A-1"); err != nil {
		t.Fatalf("the first document must save: %v", err)
	}
	err := novaPessoa(t, e, "PF", "A-1")
	if err == nil {
		t.Fatal("the business key was created twice")
	}
	if got := cerr.From(err).Type; got != "DuplicateEntryError" {
		t.Fatalf("got %q (%v), want DuplicateEntryError", got, err)
	}
}

// Changing any one component is a different key. Without this the pre-check
// would be reading the key as a set rather than as a tuple.
func TestACompoundKeyIsTheWholeTuple(t *testing.T) {
	e := setupWithKey(t)
	if err := novaPessoa(t, e, "PF", "A-1"); err != nil {
		t.Fatal(err)
	}
	if err := novaPessoa(t, e, "PJ", "A-1"); err != nil {
		t.Fatalf("a different tipo is a different key: %v", err)
	}
	if err := novaPessoa(t, e, "PF", "A-2"); err != nil {
		t.Fatalf("a different codigo is a different key: %v", err)
	}
}

// Decision: a row with any component empty falls outside the key, exactly as a
// `unique` field with no value does. The pre-check and the partial index have
// to agree about that, or one of them would refuse what the other allows.
func TestACompoundKeyIgnoresAnEmptyComponent(t *testing.T) {
	e := setupWithKey(t)
	if err := novaPessoa(t, e, "PF", ""); err != nil {
		t.Fatalf("an incomplete key is an unfinished document, not a duplicate: %v", err)
	}
	if err := novaPessoa(t, e, "PF", ""); err != nil {
		t.Fatalf("the second incomplete document must save too: %v", err)
	}
}

// The pre-check excludes the row being written, or no document could ever be
// saved a second time.
func TestSavingADocumentDoesNotCollideWithItself(t *testing.T) {
	e := setupWithKey(t)
	if err := novaPessoa(t, e, "PF", "A-1"); err != nil {
		t.Fatal(err)
	}
	rows := sqlRows(t, e, `SELECT id FROM tab_pessoa LIMIT 1`)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		doc, err := c.GetDoc("Pessoa", db.Str(rows[0]["id"]))
		if err != nil {
			return err
		}
		doc["nome"] = "Y"
		_, err = c.Save(doc, SaveOpts{IgnorePermissions: true})
		return err
	})
	if err != nil {
		t.Fatalf("re-saving a document must not collide with its own key: %v", err)
	}
}

// The acceptance DAT-05 names. A SELECT before the write cannot see a row a
// concurrent transaction has not committed, so both writers pass validation —
// the partial unique index is the only thing that refuses the second, and the
// error it produces has to be the framework's, not Postgres's.
func TestTwoTransactionsCannotCreateTheSameBusinessKey(t *testing.T) {
	e := setupWithKey(t)
	// One connection per writer, and one to watch them block.
	if max := e.DB.Pool.Config().MaxConns; max < 3 {
		t.Skipf("needs three connections, the pool has %d", max)
	}
	ctx := context.Background()
	inserted, release := make(chan struct{}), make(chan struct{})
	errA, errB := make(chan error, 1), make(chan error, 1)

	go func() {
		errA <- e.Run(ctx, "Admin", func(c *Ctx) error {
			c.Flags["ignorePermissions"] = true
			doc, err := c.NewDoc("Pessoa", Doc{"nome": "A", "tipo": "PF", "codigo": "SHARED"})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); err != nil {
				return err
			}
			close(inserted)
			<-release // hold the transaction open, written but uncommitted
			return nil
		})
	}()

	select {
	case <-inserted:
	case err := <-errA:
		t.Fatalf("the first writer failed before inserting: %v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("the first writer never inserted")
	}

	go func() {
		errB <- novaPessoa(t, e, "PF", "SHARED")
	}()

	// The barrier is Postgres's own: the second writer cannot be waiting on a
	// lock until it has passed its pre-check and reached the index. Waiting for
	// that — rather than for a duration — is what makes this deterministic.
	waitForABlockedWriter(t, e)
	close(release)

	if err := <-errA; err != nil {
		t.Fatalf("the first writer must commit: %v", err)
	}
	err := <-errB
	if err == nil {
		t.Fatal("both transactions created the same business key")
	}
	if got := cerr.From(err).Type; got != "DuplicateEntryError" {
		t.Fatalf("the loser was told %q (%v); a race must not surface as raw Postgres text", got, err)
	}
	// The two paths word it differently on purpose, and that is what proves
	// which one ran: the pre-check can name the document already holding the
	// key, the index cannot — by then the transaction is aborted and there is
	// no query left to ask with. Seeing the pre-check's key here would mean
	// the race never happened and this test proves nothing.
	if got := cerr.From(err).Key; got != "{0} already exists" {
		t.Fatalf("the loser was refused by %q, not by the index", got)
	}
	rows := sqlRows(t, e, `SELECT count(*) AS n FROM tab_pessoa WHERE tipo = $1 AND codigo = $2`, "PF", "SHARED")
	if n := rows[0]["n"]; n != int64(1) {
		t.Fatalf("the key ended up on %v rows, want 1", n)
	}
}

// waitForABlockedWriter polls until some backend on this database is waiting on
// a lock. Scoped by datname because the other packages' suites run against
// their own databases in the same cluster at the same time.
func waitForABlockedWriter(t *testing.T, e *Engine) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		rows := sqlRows(t, e, `SELECT 1 AS waiting FROM pg_locks l
			JOIN pg_stat_activity a ON a.pid = l.pid
			WHERE NOT l.granted AND a.datname = current_database() LIMIT 1`)
		if len(rows) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the second writer never blocked on the index — is the unique index there?")
}

// Reading the constraint name is what lets one 23505 become four different
// sentences. The race test covers the compound branch; these are the others,
// exercised directly because provoking each one for real would mean three more
// concurrent transactions to say nothing more.
func TestADuplicateIsReportedByWhichConstraintItHit(t *testing.T) {
	e := setupWithKey(t)
	pgErr := func(constraint string) error {
		return &pgconn.PgError{Code: "23505", ConstraintName: constraint,
			Message: `duplicate key value violates unique constraint "` + constraint + `"`}
	}
	cases := []struct {
		name, constraint, wantKey, wantTitle string
	}{
		{"compound key", "tab_pessoa_uk_tipo_codigo", "{0} already exists", "Duplicate value"},
		{"unique field", "tab_pessoa_nome", `{0} "{1}" already exists`, "Duplicate value"},
		{"primary key", "tab_pessoa_pkey", "{0} {1} already exists", "Duplicate ID"},
		// Not ours to read: another table's constraint, or a hand-made index.
		{"someone else's", "tab_pedido_cliente", "Duplicate value in {0}", "Duplicate value"},
		{"a key since removed", "tab_pessoa_uk_gone", "Duplicate value in {0}", "Duplicate value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
				d, _ := c.St.DocType("Pessoa")
				doc := Doc{"id": "PES-1", "nome": "A", "tipo": "PF", "codigo": "K"}
				got := cerr.From(c.duplicateErr(d, doc, pgErr(tc.constraint)))
				if got.Type != "DuplicateEntryError" {
					t.Fatalf("type = %q, want DuplicateEntryError", got.Type)
				}
				if got.Key != tc.wantKey {
					t.Fatalf("key = %q, want %q", got.Key, tc.wantKey)
				}
				if got.TitleKey != tc.wantTitle {
					t.Fatalf("title = %q, want %q", got.TitleKey, tc.wantTitle)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// An error that is not a unique violation has to come back untouched, or a
// connection failure would be reported to the reader as a duplicate.
func TestAnUnrelatedErrorIsNotCalledADuplicate(t *testing.T) {
	e := setupWithKey(t)
	boom := errors.New("connection reset")
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.St.DocType("Pessoa")
		if got := c.duplicateErr(d, Doc{"id": "PES-1"}, boom); got != boom {
			t.Fatalf("got %v, want the original error back", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
