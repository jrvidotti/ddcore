package engine

import (
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
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
	if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); err != nil {
		return nil, err
	}
	rec, err := e.StartRecovery(c, email, TokenInvite, db.Str(c.Request["ip"]))
	if err != nil {
		return nil, err
	}
	if err := c.Audit("account.invite", "User", email, map[string]any{"fullName": fullName, "userType": userType}); err != nil {
		return nil, err
	}
	return recoveryResult(email, rec), nil
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
	rec, err := e.StartRecovery(c, user, TokenInvite, db.Str(c.Request["ip"]))
	if err != nil {
		return nil, err
	}
	if err := c.Audit("account.resend_invite", "User", user, nil); err != nil {
		return nil, err
	}
	return recoveryResult(user, rec), nil
}

func recoveryResult(user string, rec *Recovery) map[string]any {
	out := map[string]any{"user": user, "expires": rec.Expires}
	if rec.Link != "" {
		out["link"] = rec.Link
	}
	return out
}
