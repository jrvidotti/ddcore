package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/testdb"
)

// poolMaxConns sizes the pool requests run on; the system pool of a site
// with tenancy carries single statements and keeps its own small size (#67).
func TestOpenConfinedSizesThePool(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.WithDatabase(testdb.BaseDSN(), testdb.Database(testdb.BaseDSN())+"_pool")
	if err := testdb.Empty(ctx, dsn); err != nil {
		if errors.Is(err, testdb.ErrUnavailable) {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.DropOwn(testdb.BaseDSN()) })

	single, err := OpenConfined(ctx, dsn, Options{MaxConns: 17})
	if err != nil {
		t.Fatal(err)
	}
	defer single.Close()
	if got := single.Pool.Config().MaxConns; got != 17 {
		t.Errorf("without tenancy: MaxConns %d, want 17", got)
	}

	confined, err := OpenConfined(ctx, dsn, Options{TenantRole: DefaultTenantRole, MaxConns: 17})
	if err != nil {
		t.Fatal(err)
	}
	defer confined.Close()
	if got := confined.Pool.Config().MaxConns; got != 17 {
		t.Errorf("confined pool: MaxConns %d, want 17", got)
	}
	if got := confined.Sys.Config().MaxConns; got != 4 {
		t.Errorf("system pool: MaxConns %d, want 4", got)
	}

	// unset leaves the DSN's own pool_max_conns in charge
	fromDSN, err := OpenConfined(ctx, withParam(dsn, "pool_max_conns=9"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer fromDSN.Close()
	if got := fromDSN.Pool.Config().MaxConns; got != 9 {
		t.Errorf("from the DSN: MaxConns %d, want 9", got)
	}
}

// withParam appends a query parameter to a URL DSN, whether or not it has
// parameters already.
func withParam(dsn, kv string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&" + kv
	}
	return dsn + "?" + kv
}
