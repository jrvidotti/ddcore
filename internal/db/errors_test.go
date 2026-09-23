package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestStaleCachedPlan(t *testing.T) {
	stale := &pgconn.PgError{Code: "0A000", Message: "cached plan must not change result type"}
	if !StaleCachedPlan(fmt.Errorf("wrapped: %w", stale)) {
		t.Fatal("a wrapped stale-plan error was not recognized")
	}
	if StaleCachedPlan(&pgconn.PgError{Code: "0A000", Message: "some other unsupported feature"}) {
		t.Fatal("any 0A000 was taken for a stale plan")
	}
	if StaleCachedPlan(errors.New("cached plan must not change result type")) {
		t.Fatal("a non-Postgres error was taken for a stale plan")
	}
}
