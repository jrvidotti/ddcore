package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
)

func TestGeneratePasswordHasEveryClass(t *testing.T) {
	for range 200 {
		pw, err := generatePassword(adminPasswordLength)
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != adminPasswordLength {
			t.Fatalf("%q is not %d characters", pw, adminPasswordLength)
		}
		for _, set := range []string{pwLower, pwUpper, pwDigits, pwSymbols} {
			if !strings.ContainsAny(pw, set) {
				t.Fatalf("%q has nothing from %q", pw, set)
			}
		}
	}
}

// A first install leaves Admin without a password; migrate gives it one, says
// what it is, and never replaces it afterwards.
func TestMigrateGeneratesAdminPassword(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.Cfg.Test = false

	res, err := e.Migrate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.AdminPassword) != adminPasswordLength {
		t.Fatalf("no password generated: %q", res.AdminPassword)
	}
	rows := sqlRows(t, e, `SELECT password_hash FROM tab_user WHERE id = 'Admin'`)
	if !CheckPassword(db.Str(rows[0]["password_hash"]), res.AdminPassword) {
		t.Fatal("the stored hash does not match the password shown")
	}

	again, err := e.Migrate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if again.AdminPassword != "" {
		t.Fatal("a second migrate replaced a password Admin already had")
	}
}

// A container has no console to read a generated password from: the
// deployment's DDCORE_ADMIN_PASSWORD is used instead, and never printed.
func TestMigrateUsesAdminPasswordFromEnvironment(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.Cfg.Test = false

	e.Cfg.AdminPassword = "x"
	if _, err := e.Migrate(ctx, false); err == nil || !strings.Contains(err.Error(), "DDCORE_ADMIN_PASSWORD") {
		t.Fatalf("a password the policy refuses should fail the migration by name, got %v", err)
	}

	e.Cfg.AdminPassword = "Deploy-Secret-42"
	res, err := e.Migrate(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.AdminPassword != "" {
		t.Fatalf("a password from the environment must not be shown: %q", res.AdminPassword)
	}
	rows := sqlRows(t, e, `SELECT password_hash FROM tab_user WHERE id = 'Admin'`)
	if !CheckPassword(db.Str(rows[0]["password_hash"]), "Deploy-Secret-42") {
		t.Fatal("Admin did not get DDCORE_ADMIN_PASSWORD")
	}

	// It only fills a gap: a password Admin has is never replaced.
	e.Cfg.AdminPassword = "Another-Secret-42"
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	rows = sqlRows(t, e, `SELECT password_hash FROM tab_user WHERE id = 'Admin'`)
	if !CheckPassword(db.Str(rows[0]["password_hash"]), "Deploy-Secret-42") {
		t.Fatal("a later DDCORE_ADMIN_PASSWORD replaced Admin's password")
	}
}
