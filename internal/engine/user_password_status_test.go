package engine

import (
	"context"
	"strings"
	"testing"
)

// ddcore.users.passwordStatus (#175): app code learns whether a person has a
// password and whether they also work in another space, and nothing more.

func statusOf(t *testing.T, e *Engine, tenant, caller, user string) (map[string]any, error) {
	t.Helper()
	var out map[string]any
	err := e.Run(WithTenant(context.Background(), tenant), caller, func(c *Ctx) (err error) {
		out, err = e.PasswordStatus(c, user)
		return err
	})
	return out, err
}

func wantPasswordStatus(t *testing.T, what string, out map[string]any, err error, hasPassword, elsewhere bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if len(out) != 2 || out["hasPassword"] != hasPassword || out["elsewhere"] != elsewhere {
		t.Fatalf("%s: got %v, want hasPassword=%v elsewhere=%v", what, out, hasPassword, elsewhere)
	}
}

func TestPasswordStatusInTenants(t *testing.T) {
	e := setupMembership(t)

	out, err := statusOf(t, e, tenantA, userA, userA)
	wantPasswordStatus(t, "yourself, no password", out, err, false, false)

	runAs(t, e, adminA, func(c *Ctx) error {
		return saveUser(c, userA, Doc{"new_password": "Alfa-Senha-123"})
	})
	out, err = statusOf(t, e, tenantA, userA, userA)
	wantPasswordStatus(t, "yourself, with a password", out, err, true, false)
	out, err = statusOf(t, e, tenantA, adminA, userA)
	wantPasswordStatus(t, "a System Manager about a user here", out, err, true, false)

	// an address with no row in beta, but one in alfa: beta's administrator
	// learns that it is taken, not the password nor where
	out, err = statusOf(t, e, tenantB, adminB, userA)
	wantPasswordStatus(t, "an address of another space", out, err, false, true)
	out, err = statusOf(t, e, tenantB, adminB, "  "+strings.ToUpper(userA)+" ")
	wantPasswordStatus(t, "the same address in other letters", out, err, false, true)
	out, err = statusOf(t, e, tenantB, adminB, "nobody@x.test")
	wantPasswordStatus(t, "an unknown address", out, err, false, false)

	// once she works in beta too, both spaces see it
	joinBeta(t, e, "Gestor")
	// joining forgot the password alfa's administrator chose; she sets her own
	out, err = statusOf(t, e, tenantA, adminA, userA)
	wantPasswordStatus(t, "after joining beta", out, err, false, true)
	if err := e.setPasswordBy(context.Background(), userA, "Minha-Senha-456", "", nil); err != nil {
		t.Fatal(err)
	}
	out, err = statusOf(t, e, tenantA, adminA, userA)
	wantPasswordStatus(t, "alfa about a shared person", out, err, true, true)
	out, err = statusOf(t, e, tenantB, adminB, userA)
	wantPasswordStatus(t, "beta about a shared person", out, err, true, true)
	out, err = statusOf(t, e, tenantB, userA, userA)
	wantPasswordStatus(t, "the shared person about herself", out, err, true, true)

	// a Gestor reads no User: neither another one here nor an address
	_, err = statusOf(t, e, tenantA, userA, adminA)
	wantStatus(t, err, 403)
	_, err = statusOf(t, e, tenantA, userA, "nobody@x.test")
	wantStatus(t, err, 403)
	_, err = statusOf(t, e, tenantA, adminA, "")
	wantStatus(t, err, 417)

	// and through the bridge
	err = e.Run(WithTenant(context.Background(), tenantB), adminB, func(c *Ctx) error {
		got, err := evalIn(t, c, `(() => { const s = ddcore.users.passwordStatus("`+userA+`"); return [s.hasPassword, s.elsewhere].join(); })()`)
		if err != nil {
			return err
		}
		if got != `"true,true"` {
			t.Fatalf("ddcore.users.passwordStatus: %s", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPasswordStatusWithoutTenancy(t *testing.T) {
	e := setup(t)
	const plain = "plain@x.com"
	runAs(t, e, "Admin", func(c *Ctx) error {
		return insertDoc(c, "User", Doc{"email": plain, "full_name": "Plain"})
	})
	out, err := statusOf(t, e, "", "Admin", plain)
	wantPasswordStatus(t, "a user", out, err, false, false)
	out, err = statusOf(t, e, "", "Admin", "nobody@x.com")
	wantPasswordStatus(t, "an unknown address", out, err, false, false)
	out, err = statusOf(t, e, "", plain, plain)
	wantPasswordStatus(t, "yourself", out, err, false, false)
	_, err = statusOf(t, e, "", "Guest", plain)
	wantStatus(t, err, 401)
}
