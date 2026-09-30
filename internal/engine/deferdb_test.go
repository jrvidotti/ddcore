package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/jrvidotti/ddcore/internal/testdb"
)

// Port 1 refuses at once, so the attempt fails fast.
const unreachableDSN = "postgres://ddcore:ddcore@127.0.0.1:1/ddcore?connect_timeout=2"

func TestDeferDBBootsWithoutTheDatabaseAndConnectsLater(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, Config{DSN: unreachableDSN, LogOut: io.Discard}); err == nil {
		t.Fatal("without DeferDB an unreachable database must stop New")
	}
	e, err := New(ctx, Config{DSN: unreachableDSN, DeferDB: true, LogOut: io.Discard})
	if err != nil {
		t.Fatalf("DeferDB: %v", err)
	}
	if e.DB != nil {
		t.Fatal("DB set without a database")
	}
	if err := e.Connect(ctx); err == nil || e.DB != nil {
		t.Fatalf("Connect to an unreachable database: err=%v DB=%v", err, e.DB)
	}

	// Postgres comes up: the same engine connects without being rebuilt.
	dsn := testdb.WithDatabase(testDSN, testdb.Database(testDSN)+"_defer")
	err = testdb.Empty(ctx, dsn)
	if errors.Is(err, testdb.ErrUnavailable) && os.Getenv("DDCORE_TEST_DSN") == "" {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	e.Cfg.DSN = dsn
	if err := e.Connect(ctx); err != nil {
		t.Fatalf("Connect once the database answers: %v", err)
	}
	t.Cleanup(func() { e.DB.Close() })
	first := e.DB
	if err := e.Connect(ctx); err != nil || e.DB != first {
		t.Fatalf("a second Connect must keep the pool it has: err=%v", err)
	}
}
