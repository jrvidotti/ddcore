package engine

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// #14 — a migration that changes a table's columns must not leave the pool
// holding statements planned against the old row type: Postgres refuses them
// with "cached plan must not change result type".

const selectPessoa = `SELECT * FROM tab_pessoa WHERE id = $1`

// onEveryConn runs fn on n connections held at once, so a statement prepared
// by fn is prepared on n distinct connections of the pool.
func onEveryConn(t *testing.T, e *Engine, n int, fn func(*pgxpool.Conn) error) []error {
	t.Helper()
	ctx := context.Background()
	conns := make([]*pgxpool.Conn, n)
	for i := range conns {
		c, err := e.DB.Pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns[i] = c
	}
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i, c := range conns {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = fn(c) }()
	}
	wg.Wait()
	for _, c := range conns {
		c.Release()
	}
	return errs
}

func readPessoa(id string) func(*pgxpool.Conn) error {
	return func(c *pgxpool.Conn) error {
		_, err := db.Select(context.Background(), c, selectPessoa, id)
		return err
	}
}

func TestMigrateResetsPreparedStatements(t *testing.T) {
	e := setup(t)
	id := insertPessoa(t, e, Doc{"nome": "Ana"})
	for _, err := range onEveryConn(t, e, 3, readPessoa(id)) {
		if err != nil {
			t.Fatalf("warm up: %v", err)
		}
	}

	pessoa, _ := e.Meta.Get("Pessoa")
	pessoa.Fields = append(pessoa.Fields, &meta.Field{Fieldname: "photo", Fieldtype: "Attach Image", Label: "Photo"})
	pessoa.ResetFieldIndex()
	migrar(t, e, false, "add photo")

	// straight at the pool, with no retry to hide a stale statement
	for _, err := range onEveryConn(t, e, 3, readPessoa(id)) {
		if err != nil {
			t.Fatalf("read after the migration: %v", err)
		}
	}
}

func TestRunRetriesAfterAnotherProcessAltersTheTable(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := insertPessoa(t, e, Doc{"nome": "Ana"})
	for _, err := range onEveryConn(t, e, 3, readPessoa(id)) {
		if err != nil {
			t.Fatalf("warm up: %v", err)
		}
	}

	// another process: this pool never hears of it
	other, err := pgx.Connect(ctx, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close(ctx)
	if _, err := other.Exec(ctx, `ALTER TABLE tab_pessoa ADD COLUMN photo text`); err != nil {
		t.Fatal(err)
	}

	// Each run lands on some warmed connection; every one of them has to work.
	for i := 0; i < 5; i++ {
		err := e.Run(ctx, "Admin", func(c *Ctx) error {
			_, err := db.Select(ctx, c.Q(), selectPessoa, id)
			return err
		})
		if err != nil {
			t.Fatalf("run %d after the other process's ALTER: %v", i, err)
		}
	}
}
