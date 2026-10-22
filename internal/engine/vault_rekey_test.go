package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

const (
	oldMasterKey = "the-old-master-key"
	newMasterKey = "the-new-master-key"
)

// rotateTo is step 1 of a rotation: the new key current, the old one previous.
func rotateTo(t *testing.T) {
	t.Setenv(MasterKeyEnvName, newMasterKey)
	t.Setenv(PreviousKeyEnvName, oldMasterKey)
}

func vaultKeyIDs(t *testing.T, e *Engine) map[string]string {
	t.Helper()
	rows, err := e.DB.Sys.Query(context.Background(), `SELECT name, coalesce(key_id, '') FROM ddcore_vault`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var n, id string
		if err := rows.Scan(&n, &id); err != nil {
			t.Fatal(err)
		}
		out[n] = id
	}
	return out
}

func vaultMust(t *testing.T, e *Engine, name, want string) {
	t.Helper()
	runAs(t, e, "Admin", func(c *Ctx) error {
		if v, ok, err := e.VaultGet(c, name); err != nil || !ok || v != want {
			t.Fatalf("%s: %q %v %v", name, v, ok, err)
		}
		return nil
	})
}

func TestVaultKeyFingerprint(t *testing.T) {
	a, b := deriveVaultKey(oldMasterKey), deriveVaultKey(newMasterKey)
	if a.id != deriveVaultKey(oldMasterKey).id {
		t.Fatal("the fingerprint is not stable")
	}
	if a.id == b.id || len(a.id) != 16 {
		t.Fatalf("fingerprints %q and %q", a.id, b.id)
	}
	if strings.Contains(a.id, oldMasterKey) || strings.Contains(string(a.key), a.id) {
		t.Fatal("the fingerprint shows the key")
	}

	t.Setenv(MasterKeyEnvName, newMasterKey)
	t.Setenv(PreviousKeyEnvName, newMasterKey)
	if kr, err := VaultKeys(); err != nil || kr.previous != nil {
		t.Fatalf("a previous key equal to the current one is no previous key: %v %v", kr.previous, err)
	}
	t.Setenv(MasterKeyEnvName, "")
	t.Setenv(PreviousKeyEnvName, oldMasterKey)
	if _, err := VaultKeys(); err == nil {
		t.Fatal("a previous key alone is not a configured vault")
	}
}

// With the new key current and the old one previous, a secret written before
// still reads, and a write uses the new key.
func TestVaultReadsWithThePreviousKey(t *testing.T) {
	e := setup(t)
	t.Setenv(MasterKeyEnvName, oldMasterKey)
	runAs(t, e, "Admin", func(c *Ctx) error { return e.VaultSet(c, "card:token:1", "tok-1") })
	if got := vaultKeyIDs(t, e)["card:token:1"]; got != deriveVaultKey(oldMasterKey).id {
		t.Fatalf("written under %q", got)
	}

	t.Setenv(MasterKeyEnvName, newMasterKey)
	runAs(t, e, "Admin", func(c *Ctx) error {
		if _, _, err := e.VaultGet(c, "card:token:1"); err == nil {
			t.Fatal("the new key alone opened a secret of the old one")
		}
		return nil
	})

	rotateTo(t)
	vaultMust(t, e, "card:token:1", "tok-1")
	runAs(t, e, "Admin", func(c *Ctx) error { return e.VaultSet(c, "card:token:2", "tok-2") })
	if got := vaultKeyIDs(t, e)["card:token:2"]; got != deriveVaultKey(newMasterKey).id {
		t.Fatalf("a write during the rotation went under %q", got)
	}

	// a row from before fingerprints opens under either key
	if _, err := e.DB.Sys.Exec(context.Background(), `UPDATE ddcore_vault SET key_id = NULL`); err != nil {
		t.Fatal(err)
	}
	vaultMust(t, e, "card:token:1", "tok-1")
	vaultMust(t, e, "card:token:2", "tok-2")
}

func TestVaultRekeyMovesEverySecret(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	t.Setenv(MasterKeyEnvName, oldMasterKey)
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, n := range []string{"a", "b", "c"} {
			if err := e.VaultSet(c, "card:token:"+n, "tok-"+n); err != nil {
				return err
			}
		}
		return nil
	})
	rotateTo(t)
	runAs(t, e, "Admin", func(c *Ctx) error { return e.VaultSet(c, "card:token:d", "tok-d") })

	dry, err := e.VaultRekey(ctx, VaultRekeyOpts{DryRun: true, Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Moved != 0 || dry.Census.Current != 1 || dry.Census.Previous != 3 || len(dry.Census.Unreadable) != 0 || !dry.Census.PreviousConfigured {
		t.Fatalf("dry run: %+v", dry)
	}
	if ids := vaultKeyIDs(t, e); ids["card:token:a"] != deriveVaultKey(oldMasterKey).id {
		t.Fatal("the dry run moved a secret")
	}

	res, err := e.VaultRekey(ctx, VaultRekeyOpts{BatchSize: 2, Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 3 || res.Census.Current != 4 || res.Census.Previous != 0 {
		t.Fatalf("rekey: %+v", res)
	}
	for n, id := range vaultKeyIDs(t, e) {
		if id != deriveVaultKey(newMasterKey).id {
			t.Fatalf("%s still under %q", n, id)
		}
	}

	// the previous key can go
	t.Setenv(PreviousKeyEnvName, "")
	for _, n := range []string{"a", "b", "c", "d"} {
		vaultMust(t, e, "card:token:"+n, "tok-"+n)
	}

	// one event for the run, none for the dry run: counts, no names, no values
	var count int
	var detail string
	err = e.DB.Sys.QueryRow(ctx, `SELECT count(*) OVER (), coalesce(detail::text, '') FROM tab_audit_event WHERE action = 'vault.rekey'`).Scan(&count, &detail)
	if err != nil || count != 1 {
		t.Fatalf("%d vault.rekey events (%v)", count, err)
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(detail), &d); err != nil {
		t.Fatal(err)
	}
	if d["moved"] != float64(3) || d["current"] != float64(4) || d["previous"] != float64(0) {
		t.Fatalf("detail %s", detail)
	}
	if strings.Contains(detail, "card:token") || strings.Contains(detail, "tok-") {
		t.Fatalf("the audit detail names a secret: %s", detail)
	}
}

// A secret that opens under neither key stops the re-key and is named; the
// batches before it stay moved, and its value is nowhere in the error.
func TestVaultRekeyStopsAtAnUnreadableSecret(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	t.Setenv(MasterKeyEnvName, oldMasterKey)
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, n := range []string{"a", "b"} {
			if err := e.VaultSet(c, "card:token:"+n, "tok-"+n); err != nil {
				return err
			}
		}
		return nil
	})
	t.Setenv(MasterKeyEnvName, "some-third-key")
	runAs(t, e, "Admin", func(c *Ctx) error { return e.VaultSet(c, "card:token:c", "tok-lost") })
	if _, err := e.DB.Sys.Exec(ctx, `UPDATE ddcore_vault SET key_id = NULL WHERE name = 'card:token:c'`); err != nil {
		t.Fatal(err)
	}
	rotateTo(t)

	dry, err := e.VaultRekey(ctx, VaultRekeyOpts{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Census.Unreadable) != 1 || dry.Census.Unreadable[0] != "card:token:c" || dry.Census.Previous != 2 {
		t.Fatalf("dry run: %+v", dry)
	}

	res, err := e.VaultRekey(ctx, VaultRekeyOpts{BatchSize: 1, Actor: "test"})
	if err == nil || !strings.Contains(err.Error(), "card:token:c") {
		t.Fatalf("expected the unreadable secret to be named: %v", err)
	}
	if strings.Contains(err.Error(), "tok-lost") {
		t.Fatalf("the error shows a value: %v", err)
	}
	if res.Moved != 2 || res.Census.Current != 2 {
		t.Fatalf("the batches before it: %+v", res)
	}
}

// With tenancy the re-key crosses every space, the platform's included.
func TestVaultRekeyCrossesTenants(t *testing.T) {
	e := setupSharedVault(t)
	ctx := context.Background()
	t.Setenv(MasterKeyEnvName, oldMasterKey)
	for _, tenant := range []string{tenantA, tenantB} {
		inTenant(t, e, tenant, func(c *Ctx) error { return e.VaultSet(c, "card:token:1", "of "+tenant) })
	}
	runAs(t, e, "Admin", func(c *Ctx) error { return e.VaultSet(c, "card:token:1", "of the platform") })
	// the secrets setupSharedVault wrote under its own key are out of this test
	if _, err := e.DB.Sys.Exec(ctx, `DELETE FROM ddcore_vault WHERE name <> 'card:token:1'`); err != nil {
		t.Fatal(err)
	}
	rotateTo(t)

	res, err := e.VaultRekey(ctx, VaultRekeyOpts{Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 3 || res.Census.Current != 3 {
		t.Fatalf("rekey: %+v", res)
	}
	t.Setenv(PreviousKeyEnvName, "")
	for _, tenant := range []string{tenantA, tenantB} {
		inTenant(t, e, tenant, func(c *Ctx) error {
			if v, ok, err := e.VaultGet(c, "card:token:1"); err != nil || !ok || v != "of "+tenant {
				t.Fatalf("%s: %q %v %v", tenant, v, ok, err)
			}
			return nil
		})
	}
	vaultMust(t, e, "card:token:1", "of the platform")
}

// A webhook keeps signing through a rotation: its secret is read with the
// previous key until the re-key moves it.
func TestWebhookSecretSurvivesRotation(t *testing.T) {
	t.Setenv(MasterKeyEnvName, oldMasterKey)
	e := setup(t)
	name := addWebhook(t, e, "http://127.0.0.1:1/hook", nil)
	rotateTo(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		if s, err := e.webhookSecret(c, name); err != nil || s != hookSecret {
			t.Fatalf("during the rotation: %q %v", s, err)
		}
		return nil
	})
	if _, err := e.VaultRekey(context.Background(), VaultRekeyOpts{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PreviousKeyEnvName, "")
	runAs(t, e, "Admin", func(c *Ctx) error {
		if s, err := e.webhookSecret(c, name); err != nil || s != hookSecret {
			t.Fatalf("after the re-key: %q %v", s, err)
		}
		return nil
	})
}

// #135 — a binary newer than the database's last migration: the vault says to
// migrate instead of passing Postgres's error on, the plan shows the column it
// is missing, and the migration that adds it counts it.
func TestVaultBeforeMigrateSaysToMigrate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	t.Setenv(MasterKeyEnvName, newMasterKey)
	runAs(t, e, "Admin", func(c *Ctx) error { return e.VaultSet(c, "card:token:1", "tok-1") })
	if _, err := e.DB.Sys.Exec(ctx, `ALTER TABLE ddcore_vault DROP COLUMN key_id`); err != nil {
		t.Fatal(err)
	}
	e.DB.Pool.Reset()
	e.DB.Sys.Reset()

	notMigrated := func(what string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "run ddcore migrate") {
			t.Fatalf("%s: %v", what, err)
		}
	}
	_, err := e.VaultRekey(ctx, VaultRekeyOpts{DryRun: true})
	notMigrated("rekey --dry-run", err)
	_, err = e.VaultRekey(ctx, VaultRekeyOpts{Actor: "test"})
	notMigrated("rekey", err)
	_, err = e.VaultCensus(ctx)
	notMigrated("census", err)
	_, _, err = e.VaultGet(nil, "card:token:1")
	notMigrated("get", err)
	var events int
	if err := e.DB.Sys.QueryRow(ctx, `SELECT count(*) FROM tab_audit_event WHERE action = 'vault.rekey'`).Scan(&events); err != nil || events != 0 {
		t.Fatalf("a re-key that could not start recorded %d event(s) (%v)", events, err)
	}

	plan, err := e.Plan(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || !plan[0].Internal || plan[0].Kind != db.KindAddColumn || plan[0].Table != "ddcore_vault" || plan[0].Column != "key_id" {
		t.Fatalf("plan: %v", db.SQL(plan))
	}
	if out := db.Report(plan); !strings.Contains(out, "ddcore_vault.key_id") {
		t.Fatalf("report:\n%s", out)
	}

	res, err := e.Migrate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.String(), "1 DDL") {
		t.Fatalf("migrate: %s", res)
	}
	var recorded int
	if err := e.DB.Sys.QueryRow(ctx, `SELECT count(*) FROM ddcore_migration WHERE ddl LIKE '%ddcore_vault ADD COLUMN IF NOT EXISTS key_id%'`).Scan(&recorded); err != nil || recorded != 1 {
		t.Fatalf("recorded %d time(s) (%v)", recorded, err)
	}
	if plan, err := e.Plan(ctx, false); err != nil || len(plan) != 0 {
		t.Fatalf("after migrate: %v %v", db.SQL(plan), err)
	}
	dry, err := e.VaultRekey(ctx, VaultRekeyOpts{DryRun: true})
	if err != nil || dry.Census.Current != 1 {
		t.Fatalf("after migrate: %+v %v", dry, err)
	}
}
