package engine

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// One person, several tenants (#167): a User row per tenant under one id,
// roles per tenant, one password, and a tenant's administrator who learns
// nothing of the others.

const (
	adminA = "chefe@alfa.test" // alfa's System Manager
	adminB = "chefe@beta.test" // beta's
)

// setupMembership is setupTenancy with a System Manager in each tenant, and
// userA also working in beta, as a Gestor there too.
func setupMembership(t *testing.T) *Engine {
	t.Helper()
	e := setupTenancy(t)
	for tenant, admin := range map[string]string{tenantA: adminA, tenantB: adminB} {
		inTenant(t, e, tenant, func(c *Ctx) error {
			return insertDoc(c, "User", Doc{"email": admin, "full_name": admin,
				"roles": []any{map[string]any{"role": "System Manager"}}})
		})
	}
	return e
}

// saveUser saves a User through the document path, as the desk does.
func saveUser(c *Ctx, id string, values Doc) error {
	doc, err := c.GetDoc("User", id)
	if err != nil {
		return err
	}
	for k, v := range values {
		doc[k] = v
	}
	_, err = c.Save(doc, SaveOpts{})
	return err
}

func joinBeta(t *testing.T, e *Engine, roles ...string) {
	t.Helper()
	rs := []any{}
	for _, r := range roles {
		rs = append(rs, map[string]any{"role": r})
	}
	inTenant(t, e, tenantB, func(c *Ctx) error {
		return insertDoc(c, "User", Doc{"email": userA, "full_name": "Ana in beta", "roles": rs})
	})
}

func TestMembershipTwoTenantsOnePerson(t *testing.T) {
	e := setupMembership(t)
	joinBeta(t, e, "System Manager")
	ms, err := e.Memberships(context.Background(), userA)
	if err != nil || len(ms) != 2 || ms[0].Tenant != tenantA || ms[1].Tenant != tenantB {
		t.Fatalf("memberships: %+v, %v", ms, err)
	}
	// the roles are each tenant's
	rolesIn := func(tenant string) []string {
		var roles []string
		err := e.Run(WithTenant(context.Background(), tenant), userA, func(c *Ctx) (err error) {
			roles, err = c.Roles()
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return roles
	}
	if a := rolesIn(tenantA); !slices.Contains(a, "Gestor") || slices.Contains(a, "System Manager") {
		t.Fatalf("roles in alfa: %v", a)
	}
	if b := rolesIn(tenantB); !slices.Contains(b, "System Manager") || slices.Contains(b, "Gestor") {
		t.Fatalf("roles in beta: %v", b)
	}
	// from the platform space a person in two tenants names one
	wantStatus(t, e.Run(context.Background(), userA, func(*Ctx) error { return nil }), 417)
	// and asking about someone from a tenant they are not in tells nothing
	runAs(t, e, userB, func(c *Ctx) error {
		if r, err := c.RolesOf(adminA); err != nil || len(r) != 1 {
			t.Fatalf("beta learnt the roles of alfa's System Manager: %v, %v", r, err)
		}
		return nil
	})
}

func TestMembershipJoinsThePersonIgnoringCase(t *testing.T) {
	e := setupMembership(t)
	inTenant(t, e, tenantB, func(c *Ctx) error {
		return insertDoc(c, "User", Doc{"email": strings.ToUpper(userA), "full_name": "Ana"})
	})
	ms, _ := e.Memberships(context.Background(), userA)
	if len(ms) != 2 {
		t.Fatalf("an address in other letters is another person: %+v", ms)
	}
	// an address that is somebody else's is not taken by a change of e-mail
	inTenant(t, e, tenantB, func(c *Ctx) error {
		err := c.SetValue("User", adminB, Doc{"email": userA})
		mustErr(t, "taking another person's address", err)
		return nil
	})
	// and Admin is nobody's tenant row
	inTenant(t, e, tenantB, func(c *Ctx) error {
		mustErr(t, "a tenant row for Admin", insertDoc(c, "User", Doc{"email": "admin", "full_name": "Admin"}))
		return nil
	})
}

func TestMembershipPasswordGuard(t *testing.T) {
	e := setupMembership(t)
	ctx := context.Background()
	// alone in alfa, ana's password is alfa's administrator's to set
	runAs(t, e, adminA, func(c *Ctx) error {
		err := saveUser(c, userA, Doc{"new_password": "Alfa-Senha-123"})
		return err
	})
	joinBeta(t, e, "Gestor")
	// now she works in beta too: neither administrator may
	for tenant, admin := range map[string]string{tenantA: adminA, tenantB: adminB} {
		err := e.Run(WithTenant(ctx, tenant), admin, func(c *Ctx) error {
			err := saveUser(c, userA, Doc{"new_password": "Tomada-Senha-123"})
			return err
		})
		wantStatus(t, err, 403)
		if msg := err.Error(); strings.Contains(strings.ToLower(msg), tenantA) || strings.Contains(strings.ToLower(msg), tenantB) {
			t.Fatalf("the refusal names a tenant: %s", msg)
		}
	}
	// the password alfa's administrator chose for her went when she joined
	// beta: alfa knows it, and would sign in as her there
	if _, err := e.Login(ctx, userA, "Alfa-Senha-123", LoginFrom{}); err == nil {
		t.Fatal("a password alfa chose for her still signs her in to beta")
	}
	// she may, and so may the operator
	if err := e.Run(WithTenant(ctx, tenantB), userA, func(c *Ctx) error {
		return saveUser(c, userA, Doc{"new_password": "Dela-Senha-123"})
	}); err != nil && cerr.From(err).Status != 403 {
		// a Gestor may not save Users at all; what matters is the guard
		t.Fatal(err)
	}
	if err := e.Run(WithTenant(ctx, tenantB), userA, func(c *Ctx) error { return c.mayManageCredential(userA) }); err != nil {
		t.Fatalf("she may not change her own password: %v", err)
	}
	// but a key or a provider's session of hers is a tenant's to hold
	err := e.Run(WithBoundCredential(WithTenant(ctx, tenantB)), userA, func(c *Ctx) error { return c.mayManageCredential(userA) })
	wantStatus(t, err, 403)
	inTenant(t, e, tenantB, func(c *Ctx) error {
		err := saveUser(c, userA, Doc{"new_password": "Operador-Senha-123"})
		return err
	})
	if _, err := e.Login(ctx, userA, "Operador-Senha-123", LoginFrom{}); err != nil {
		t.Fatalf("the operator's password does not sign her in: %v", err)
	}
	// a reset link from her administrator is refused too
	err = e.Run(WithTenant(ctx, tenantA), adminA, func(c *Ctx) error {
		return c.mayManageCredential(userA)
	})
	wantStatus(t, err, 403)
	// and the confined role cannot read the hashes at all
	runAs(t, e, adminA, func(c *Ctx) error {
		if _, err := c.SQL(`select password_hash from ddcore_person`, nil); err == nil {
			t.Fatal("tenant SQL read ddcore_person")
		}
		return nil
	})
}

func TestMembershipLinkIssuedBeforeSecondMembershipIsRefused(t *testing.T) {
	e := setupMembership(t)
	ctx := context.Background()
	var token string
	err := e.Run(WithTenant(ctx, tenantA), adminA, func(c *Ctx) (err error) {
		token, _, err = e.IssueToken(WithTenant(ctx, tenantA), userA, TokenReset, time.Hour, adminA, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	joinBeta(t, e, "Gestor")
	if _, err := e.CompleteRecovery(ctx, token, TokenReset, "Roubada-Senha-123", ""); err == nil {
		t.Fatal("a link alfa's administrator kept set the password of a person who now works in beta")
	}
}

func TestMembershipDisableAndDeleteAreLocal(t *testing.T) {
	e := setupMembership(t)
	ctx := context.Background()
	joinBeta(t, e, "Gestor")
	if err := e.SetPassword(ctx, userA, "Ana-Senha-123"); err != nil {
		t.Fatal(err)
	}
	link := func() {
		if _, err := e.DB.Sys.Exec(ctx, `INSERT INTO ddcore_user_identity (provider, subject, "user") VALUES ('kc', 'sub-ana', $1)
			ON CONFLICT DO NOTHING`, userA); err != nil {
			t.Fatal(err)
		}
	}
	link()
	sidA, err := e.Login(ctx, userA, "Ana-Senha-123", LoginFrom{})
	if err != nil {
		t.Fatal(err)
	}
	if sp := e.SessionTenant(ctx, sidA); sp.Tenant != tenantA {
		t.Fatalf("landed in %q", sp.Tenant)
	}
	if err := e.moveSession(ctx, sidA, userA, tenantA); err != nil {
		t.Fatal(err)
	}
	sidB, _ := e.Login(ctx, userA, "Ana-Senha-123", LoginFrom{})
	if err := e.moveSession(ctx, sidB, userA, tenantB); err != nil {
		t.Fatal(err)
	}
	// alfa disables her: alfa's session ends, beta's goes on
	runAs(t, e, adminA, func(c *Ctx) error {
		err := saveUser(c, userA, Doc{"enabled": false})
		return err
	})
	e.Cache.Clear()
	if u, _ := e.UserFromSession(ctx, sidA); u != "" {
		t.Fatal("the session in alfa survived her being disabled there")
	}
	if u, _ := e.UserFromSession(ctx, sidB); u != userA {
		t.Fatal("disabling her in alfa ended her session in beta")
	}
	// alfa deletes her: her sign-in stays, for beta
	runAs(t, e, adminA, func(c *Ctx) error { return c.Delete("User", userA, false, false) })
	if h := personHashT(t, e, userA); h == "" {
		t.Fatal("deleting one row took her password")
	}
	var n int
	e.DB.Sys.QueryRow(ctx, `SELECT count(*) FROM ddcore_user_identity WHERE "user" = $1`, userA).Scan(&n)
	if n != 1 {
		t.Fatal("deleting one row took her identity")
	}
	// beta deletes her last row: everything goes
	runAs(t, e, adminB, func(c *Ctx) error { return c.Delete("User", userA, false, false) })
	if h := personHashT(t, e, userA); h != "" {
		t.Fatal("her password outlived her last row")
	}
	e.DB.Sys.QueryRow(ctx, `SELECT count(*) FROM ddcore_user_identity WHERE "user" = $1`, userA).Scan(&n)
	if n != 0 {
		t.Fatal("her identity outlived her last row")
	}
}

func TestMembershipAPIKeyBoundToItsRow(t *testing.T) {
	e := setupMembership(t)
	ctx := context.Background()
	joinBeta(t, e, "Gestor")
	// from the platform space, a person in two tenants needs one named
	if _, err := e.CreateAPIKey(ctx, userA, "ci"); err == nil {
		t.Fatal("a key was issued without saying which tenant")
	}
	token, err := e.CreateAPIKey(WithTenant(ctx, tenantB), userA, "ci")
	if err != nil {
		t.Fatal(err)
	}
	u, space, err := e.APIKeyOwner(ctx, token)
	if err != nil || u != userA || space != tenantB {
		t.Fatalf("the key is %q in %q, %v", u, space, err)
	}
	// it works in beta whatever the request names
	rctx, err := e.RequestSpace(ctx, u, SpaceReq{Fixed: &space, Header: tenantA})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := TenantFrom(rctx); got != tenantB {
		t.Fatalf("the key works in %q", got)
	}
	// and stops when beta disables her
	runAs(t, e, adminB, func(c *Ctx) error {
		err := saveUser(c, userA, Doc{"enabled": false})
		return err
	})
	e.Cache.Clear()
	if u, _, _ := e.APIKeyOwner(ctx, token); u != "" {
		t.Fatal("the key outlived her row in beta")
	}
}

func TestMembershipRunAsFromThePlatformNeedsATenant(t *testing.T) {
	e := setupMembership(t)
	joinBeta(t, e, "Gestor")
	runAs(t, e, "Admin", func(c *Ctx) error {
		_, err := c.userCtx(userA)
		mustErr(t, "acting as a person in two tenants from the platform", err)
		return c.InTenant(tenantB, func(b *Ctx) error {
			as, err := b.userCtx(userA)
			if err != nil {
				return err
			}
			if as.Tenant != tenantB {
				t.Fatalf("acting as her inside beta works in %q", as.Tenant)
			}
			return nil
		})
	})
}

func TestMembershipInviteExistingPersonGrantsAccess(t *testing.T) {
	e := setupMembership(t)
	var res map[string]any
	err := e.Run(WithTenant(context.Background(), tenantB), adminB, func(c *Ctx) (err error) {
		res, err = e.InviteUser(c, Invitation{Email: userA, FullName: "Ana", Roles: []string{"Gestor"}})
		return err
	})
	if err != nil || res["access"] != true || res["link"] != nil {
		t.Fatalf("invite: %v, %v", res, err)
	}
	var tokens, mails int
	e.DB.Sys.QueryRow(context.Background(), `SELECT count(*) FROM ddcore_auth_token WHERE "user" = $1`, userA).Scan(&tokens)
	if tokens != 0 {
		t.Fatalf("an invitation to a person who signs in elsewhere minted %d token(s)", tokens)
	}
	e.DB.Sys.QueryRow(context.Background(), `SELECT count(*) FROM tab_email_delivery WHERE template = $1 AND tenant = $2`,
		MailTemplateAccess, tenantB).Scan(&mails)
	if mails != 1 {
		t.Fatalf("%d access mail(s) queued in beta", mails)
	}
}

func TestMembershipLanding(t *testing.T) {
	e := setupMembership(t)
	ctx := context.Background()
	joinBeta(t, e, "Gestor")
	// titles ALFA < BETA: the first by title, until she works somewhere
	if sp, err := e.LandingSpace(ctx, userA); err != nil || sp != tenantA {
		t.Fatalf("first landing %q, %v", sp, err)
	}
	if err := e.System(ctx, func(q db.Querier) error { return rememberSpace(ctx, q, userA, tenantB) }); err != nil {
		t.Fatal(err)
	}
	e.Cache.Clear()
	if sp, _ := e.LandingSpace(ctx, userA); sp != tenantB {
		t.Fatalf("landed in %q, not where she last worked", sp)
	}
	// a session that names no tenant, or one she does not work in, lands
	for _, named := range []string{"", "nowhere"} {
		rctx, err := e.RequestSpace(ctx, userA, SpaceReq{Session: named, SessionSet: named != ""})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := TenantFrom(rctx); got != tenantB {
			t.Fatalf("session naming %q works in %q", named, got)
		}
	}
	// a disabled row is somewhere she cannot go
	runAs(t, e, adminB, func(c *Ctx) error {
		err := saveUser(c, userA, Doc{"enabled": false})
		return err
	})
	e.Cache.Clear()
	if sp, _ := e.LandingSpace(ctx, userA); sp != tenantA {
		t.Fatalf("landed in %q, where she is disabled", sp)
	}
}

func TestPasswordAbsorbedIntoPersonTable(t *testing.T) {
	e := setupTenancy(t)
	ctx := context.Background()
	hash := HashPassword("Antiga-Senha-123")
	if _, err := e.DB.Sys.Exec(ctx, `ALTER TABLE tab_user ADD COLUMN IF NOT EXISTS password_hash text`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Sys.Exec(ctx, `DELETE FROM ddcore_person WHERE id = $1`, userA); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Sys.Exec(ctx, `UPDATE tab_user SET password_hash = $2 WHERE id = $1`, userA, hash); err != nil {
		t.Fatal(err)
	}
	if err := absorbUserPasswords(ctx, e.DB.Sys); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Login(ctx, userA, "Antiga-Senha-123", LoginFrom{}); err != nil {
		t.Fatalf("the absorbed password does not sign her in: %v", err)
	}
	var left int
	e.DB.Sys.QueryRow(ctx, `SELECT count(*) FROM tab_user WHERE password_hash IS NOT NULL`).Scan(&left)
	if left != 0 {
		t.Fatalf("%d row(s) still hold a hash", left)
	}
}

// A tenant that gives an operator's address a row of its own gains no say
// over the operator's sign-in: no key minted for that row, no password set
// through it, and no operator power for whoever holds it.
func TestMembershipOperatorRowInATenantCarriesNoSitePower(t *testing.T) {
	e := setupMembership(t)
	ctx := context.Background()
	const op = "op@platform.test"
	runAs(t, e, "Admin", func(c *Ctx) error {
		return insertDoc(c, "User", Doc{"email": op, "full_name": "Op",
			"roles": []any{map[string]any{"role": "System Manager"}}})
	})
	if err := e.SetPassword(ctx, op, "Operador-Senha-123"); err != nil {
		t.Fatal(err)
	}
	// beta adds the operator's address
	runAs(t, e, adminB, func(c *Ctx) error {
		return insertDoc(c, "User", Doc{"email": op, "full_name": "Op in beta",
			"roles": []any{map[string]any{"role": "System Manager"}}})
	})
	err := e.Run(WithTenant(ctx, tenantB), adminB, func(c *Ctx) error {
		_, err := e.CreateUserAPIKey(c, op, "x", 0, "")
		return err
	})
	wantStatus(t, err, 403)
	// working through beta's row, the operator is beta's member, not the site's
	err = e.Run(WithTenant(ctx, tenantB), op, func(c *Ctx) error {
		if ok, err := c.isOperator(); err != nil || ok {
			t.Fatalf("an operator's tenant row acts as the operator: %v, %v", ok, err)
		}
		return c.mayManageCredential(op)
	})
	wantStatus(t, err, 403)
	if _, err := e.Login(ctx, op, "Operador-Senha-123", LoginFrom{}); err != nil {
		t.Fatalf("the operator's password changed: %v", err)
	}
}

// A password the operator set, or the person, stays when they join another
// tenant; only one a tenant chose goes.
func TestMembershipJoinKeepsAPasswordNoTenantChose(t *testing.T) {
	e := setupMembership(t)
	ctx := context.Background()
	if err := e.SetPassword(ctx, userA, "Dela-Senha-123"); err != nil {
		t.Fatal(err)
	}
	joinBeta(t, e, "Gestor")
	if _, err := e.Login(ctx, userA, "Dela-Senha-123", LoginFrom{}); err != nil {
		t.Fatalf("joining beta took a password she had: %v", err)
	}
}
