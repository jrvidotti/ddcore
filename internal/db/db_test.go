package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

// SetMaintenanceFlag is the one writer of the flag. Switching it on while it is
// already on keeps the moment the pause began; switching it off blanks the
// reason and the moment.
func TestSetMaintenanceFlagKeepsSinceWhileOn(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.WithDatabase(testdb.BaseDSN(), testdb.Database(testdb.BaseDSN())+"_maint")
	if err := testdb.Empty(ctx, dsn); err != nil {
		if errors.Is(err, testdb.ErrUnavailable) {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.DropOwn(testdb.BaseDSN()) })
	d, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	read := func() (on bool, reason string, since *time.Time) {
		t.Helper()
		if err := d.Pool.QueryRow(ctx, `SELECT enabled, reason, since FROM ddcore_maintenance WHERE id = 1`).Scan(&on, &reason, &since); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := SetMaintenanceFlag(ctx, d, true, "first", "cli"); err != nil {
		t.Fatal(err)
	}
	_, _, first := read()
	if first == nil {
		t.Fatal("since must be set while the flag is on")
	}
	if err := SetMaintenanceFlag(ctx, d, true, "second", "cli"); err != nil {
		t.Fatal(err)
	}
	on, reason, again := read()
	if !on || reason != "second" || again == nil || !again.Equal(*first) {
		t.Fatalf("on=%v reason=%q since=%v, want on, 'second' and since %v unchanged", on, reason, again, first)
	}
	if err := SetMaintenanceFlag(ctx, d, false, "ignored", "cli"); err != nil {
		t.Fatal(err)
	}
	if on, reason, since := read(); on || reason != "" || since != nil {
		t.Fatalf("off must blank the reason and since, got on=%v reason=%q since=%v", on, reason, since)
	}
}
