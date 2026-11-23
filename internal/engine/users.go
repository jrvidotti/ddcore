package engine

import (
	"net/url"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Invitation is what ddcore.users.invite asks for.
type Invitation struct {
	Email    string
	FullName string
	Roles    []string
	UserType string
}

// privilegedRoles are never handed out by an invitation that a System Manager
// did not send: they are the desk's keys, not a portal's.
var privilegedRoles = map[string]bool{"admin": true, "system manager": true, "all": true, "guest": true}

// canAdministerUsers reports whether the current user may create any kind of
// account: Admin, or a System Manager outside portal mode.
func (c *Ctx) canAdministerUsers() bool {
	if c.User == "Admin" {
		return true
	}
	return !c.PortalMode() && c.HasRole("System Manager")
}

// InviteUser creates an account with no password and sends the invitation.
//
// A System Manager may invite anyone. Anyone else — which in practice means
// app code a narrower role reached, such as HR inviting employees to their
// portal — may only invite a Website User, and never with a privileged role.
// That is enough to make the call safe to expose: a Website User is confined
// to the portals (OPS-10), so the roles given to one open portals and nothing
// else. Deciding *who* may invite is left to the app's own whitelist.
func (e *Engine) InviteUser(c *Ctx, inv Invitation) (map[string]any, error) {
	if c.User == "Guest" || c.User == "" {
		return nil, cerr.Auth("Sign in to continue")
	}
	email := strings.TrimSpace(inv.Email)
	fullName := strings.TrimSpace(inv.FullName)
	if email == "" {
		return nil, cerr.Validation("Email is required")
	}
	if Undeliverable(email) {
		// a placeholder for someone without a mailbox: the invitation would
		// go nowhere, and the link in it is the whole point (#115)
		return nil, cerr.Validation("{0} has no mailbox, so it cannot be invited: add the user without an invitation", email)
	}
	if fullName == "" {
		return nil, cerr.Validation("Full name is required")
	}
	userType := strings.TrimSpace(inv.UserType)
	if !c.canAdministerUsers() {
		if userType == "" {
			userType = "Website User"
		}
		if userType != "Website User" {
			return nil, cerr.Permission("Only a System Manager can invite a System User")
		}
		for _, r := range inv.Roles {
			if privilegedRoles[strings.ToLower(strings.TrimSpace(r))] {
				return nil, cerr.Permission("Only a System Manager can give the role {0}", r)
			}
		}
	}
	if userType == "" {
		userType = "System User"
	}
	if userType != "System User" && userType != "Website User" {
		return nil, cerr.Validation("{0} is not a user type", userType)
	}
	// In this space only (#167): an address with an account in another
	// tenant is the same person, who joins this one — and the caller is not
	// told the address is known anywhere else.
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id FROM tab_user WHERE id = $1 OR lower(email) = lower($1)`, email)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return nil, cerr.Validation("User {0} already exists", email)
	}
	roles := []any{}
	seen := map[string]bool{}
	for _, r := range inv.Roles {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		roles = append(roles, map[string]any{"role": r})
	}
	doc, err := c.NewDoc("User", Doc{"email": email, "full_name": fullName, "enabled": true, "user_type": userType, "roles": roles})
	if err != nil {
		return nil, err
	}
	saved, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
	if err != nil {
		return nil, err
	}
	// the row joined the person of that address, whose id it now carries
	email = saved.ID()
	if shared, err := c.sharedPerson(email); err != nil {
		return nil, err
	} else if shared {
		return e.grantAccess(c, email, fullName, userType)
	}
	rec, provider, err := e.sendInvitation(c, email, userType)
	if err != nil {
		return nil, err
	}
	if err := c.Audit("account.invite", "User", email, map[string]any{"fullName": fullName, "userType": userType, "provider": provider}); err != nil {
		return nil, err
	}
	return recoveryResult(email, rec), nil
}

// grantAccess is an invitation for a person who already signs in to the
// site from another space (#167): they have a password, or an identity, and
// a link to set another would let this tenant take over the others. They are
// told they can now work here, and nothing else changes hands.
func (e *Engine) grantAccess(c *Ctx, user, fullName, userType string) (map[string]any, error) {
	title := ""
	if c.Tenant != "" {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT title FROM `+meta.TenantTable+` WHERE id = $1`, c.Tenant)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			title = db.Str(rows[0]["title"])
		}
	}
	link := strings.TrimSuffix(e.Cfg.SiteURL, "/") + "/app"
	if c.Tenant != "" {
		link += "?tenant=" + url.QueryEscape(c.Tenant)
	}
	if !Undeliverable(user) {
		if err := c.SendTemplate(MailTemplateAccess, user, map[string]any{"tenantTitle": title, "link": link}); err != nil {
			return nil, err
		}
	}
	if err := c.Audit("account.invite", "User", user, map[string]any{"fullName": fullName, "userType": userType, "access": true}); err != nil {
		return nil, err
	}
	return map[string]any{"user": user, "access": true}, nil
}

// ResendInvite sends a new invitation to an account that exists. The same
// rule as InviteUser applies: without user administration, only a Website
// User's.
func (e *Engine) ResendInvite(c *Ctx, user string) (map[string]any, error) {
	if c.User == "Guest" || c.User == "" {
		return nil, cerr.Auth("Sign in to continue")
	}
	user = strings.TrimSpace(user)
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, user_type FROM tab_user WHERE id = $1`, user)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cerr.Validation("User {0} does not exist", user)
	}
	if !c.canAdministerUsers() && db.Str(rows[0]["user_type"]) != "Website User" {
		return nil, cerr.Permission("Only a System Manager can invite a System User")
	}
	if Undeliverable(user) {
		return nil, cerr.Validation("{0} has no mailbox, so it cannot be invited: add the user without an invitation", user)
	}
	if err := c.mayManageCredential(user); err != nil {
		return nil, err
	}
	rec, provider, err := e.sendInvitation(c, user, db.Str(rows[0]["user_type"]))
	if err != nil {
		return nil, err
	}
	if err := c.Audit("account.resend_invite", "User", user, map[string]any{"provider": provider}); err != nil {
		return nil, err
	}
	return recoveryResult(user, rec), nil
}

// sendInvitation mails the way in. A System User of a site that provisions
// its identity provider gets an account there and its one-time link; anyone
// else — and a Website User always, since the portals sign in with a
// password — gets a link to choose a password. provider is the provider id,
// or "" for the password.
//
// The provider is called inside the transaction on purpose: if it refuses,
// the invitation fails and the admin reads why. If this transaction rolls back
// after the account was made there, the next invitation finds and reuses it.
func (e *Engine) sendInvitation(c *Ctx, user, userType string) (*Recovery, string, error) {
	if p := e.idpAdmin(); p != nil && userType != "Website User" {
		rec, err := e.inviteThroughPocketID(c, p, user)
		return rec, p.provider.ID, err
	}
	rec, err := e.StartRecovery(c, user, TokenInvite, db.Str(c.Request["ip"]))
	return rec, "", err
}

func recoveryResult(user string, rec *Recovery) map[string]any {
	out := map[string]any{"user": user, "expires": rec.Expires}
	if rec.Link != "" {
		out["link"] = rec.Link
	}
	return out
}

// CreateUserAPIKey issues an API key for another user, so that provisioning a
// tenant — its users and the key its integration signs in with — can be one
// method instead of a method and a CLI step. It returns { key, secret,
// expires }; the secret is shown this once, as CreateAPIKeyFor explains.
//
// Only someone who may administer users may call it: a key is that user's
// identity, so issuing one is as strong as setting their password. Inside a
// tenant the user must be of that tenant; one of another tenant is reported
// as missing rather than refused, so the call does not tell a tenant who
// exists elsewhere. The platform space issues the key in the user's own space,
// as `ddcore apikey` always has, on the caller's transaction.
//
// The audit event records who issued which key, never the secret. Its detail
// keys stay clear of the names SanitizeAuditDetail redacts ("key", "token"…),
// or the key's id would be blanked with them.
func (e *Engine) CreateUserAPIKey(c *Ctx, user, label string, days int, prefix string) (map[string]any, error) {
	if c.User == "Guest" || c.User == "" {
		return nil, cerr.Auth("Sign in to continue")
	}
	if !c.canAdministerUsers() {
		return nil, cerr.Permission("Only a System Manager can create an API key for another user")
	}
	user = strings.TrimSpace(user)
	missing := cerr.Validation("User {0} does not exist", user)
	switch user {
	case "", "Guest":
		return nil, missing
	case "Admin":
		// Admin's key would be every right on the site, more than a System
		// Manager holds
		if c.User != "Admin" {
			return nil, cerr.Permission("Only Admin can create an API key for Admin")
		}
	}
	space := c.Tenant
	if c.Tenancy() && user != "Admin" && c.Tenant == "" {
		// from the platform space: the user's row there, or their only one
		ms, err := c.memberships(user)
		if err != nil {
			return nil, err
		}
		if len(ms) == 0 {
			return nil, missing
		}
		if space, err = soleSpace(ms, user); err != nil {
			return nil, err
		}
	}
	// A key is a way to sign in as the person, like a password: one who also
	// works elsewhere gets one from the operator, or mints their own (#167).
	// someone of another space is reported as missing, before anything else
	if user != "Admin" {
		if ok, err := c.personHere(user); err != nil {
			return nil, err
		} else if !ok && c.Tenant != "" {
			return nil, missing
		}
	}
	if err := c.mayManageCredential(user); err != nil {
		return nil, err
	}
	var out map[string]any
	issue := func(c *Ctx) error {
		if user != "Admin" {
			rows, err := db.Select(c.Ctx, c.Q(), `SELECT id FROM tab_user WHERE id = $1`, user)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return missing
			}
		}
		minted, err := e.CreateAPIKeyFor(c, user, label, days, prefix)
		if err != nil {
			return err
		}
		id := minted["id"].(string)
		out = map[string]any{
			"key": id, "secret": strings.TrimPrefix(minted["token"].(string), id+":"), "expires": minted["expires"],
		}
		return c.Audit("apikey.create", "User", user, map[string]any{"id": id, "label": label, "expires": minted["expires"]})
	}
	var err error
	if space != c.Tenant {
		err = c.InTenant(space, issue)
	} else {
		err = issue(c)
	}
	return out, err
}
