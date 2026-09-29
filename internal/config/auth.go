package config

import (
	"fmt"
	"strings"
	"time"
)

// AuthPolicy is the site's access policy: how long a session lasts, how many
// guesses a login gets, how long a recovery link stays good.
//
// It lives in ddcore.json and not in the environment because it is a decision
// the site makes once and keeps everywhere — staging should lock an account
// out exactly like production does, or the rehearsal is not a rehearsal.
// Secrets and per-deployment addresses go the other way, into .env.
type AuthPolicy struct {
	SessionDays       int `json:"sessionDays"`
	APIKeyDays        int `json:"apiKeyDays"` // 0 = an API key never expires
	MinPasswordLength int `json:"minPasswordLength"`
	MaxLoginAttempts  int `json:"maxLoginAttempts"`
	LockoutMinutes    int `json:"lockoutMinutes"`
	ResetMinutes      int `json:"resetMinutes"`
	InviteHours       int `json:"inviteHours"`
	// SecureCookie forces the Secure flag on the session cookie. Nil means
	// "decide per request" — on when the request arrived over TLS or the site
	// URL is https — because a hardcoded true breaks `ddcore dev` over http
	// and a hardcoded false is a production mistake nobody notices.
	SecureCookie *bool `json:"secureCookie"`
	// SelfServiceAPIKeys lets a user mint and revoke their own API keys. Nil
	// means allowed.
	SelfServiceAPIKeys *bool `json:"selfServiceApiKeys"`
	// PasswordLogin lets people sign in with a password. Nil means allowed.
	// Turning it off leaves single sign-on as the only way in for everyone
	// except Admin, who keeps a password so that an outage at the
	// identity provider is not also an outage of the site's administration.
	PasswordLogin *bool `json:"passwordLogin,omitempty"`
	// SSO is per-provider policy, keyed by the provider id in
	// DDCORE_OIDC_PROVIDERS. The providers themselves are configured in the
	// environment; what their groups mean on this site is decided here.
	SSO map[string]SSOPolicy `json:"sso,omitempty"`
}

// SSOPolicy says what a provider's groups mean on this site.
type SSOPolicy struct {
	// GroupsClaim names the claim holding the groups; empty means "groups".
	GroupsClaim string `json:"groupsClaim,omitempty"`
	// GroupRoles maps a group to the roles it grants. The roles named here
	// are *managed*: a sign-in sets them from the groups, a save of the User
	// pushes them back as groups. Every other role is left alone.
	GroupRoles map[string][]string `json:"groupRoles,omitempty"`
}

// Claim is the claim the groups are read from.
func (p SSOPolicy) Claim() string {
	if p.GroupsClaim == "" {
		return "groups"
	}
	return p.GroupsClaim
}

// ManagedRoles is every role some group maps to.
func (p SSOPolicy) ManagedRoles() map[string]bool {
	out := map[string]bool{}
	for _, roles := range p.GroupRoles {
		for _, r := range roles {
			out[r] = true
		}
	}
	return out
}

// RolesFor is the set of managed roles the given groups grant.
func (p SSOPolicy) RolesFor(groups []string) map[string]bool {
	out := map[string]bool{}
	for _, g := range groups {
		for _, r := range p.GroupRoles[g] {
			out[r] = true
		}
	}
	return out
}

// GroupsFor is the set of mapped groups a User holding these roles belongs
// in: a group whose roles the User has every one of.
func (p SSOPolicy) GroupsFor(roles map[string]bool) map[string]bool {
	out := map[string]bool{}
	for g, rs := range p.GroupRoles {
		all := len(rs) > 0
		for _, r := range rs {
			all = all && roles[r]
		}
		if all {
			out[g] = true
		}
	}
	return out
}

// DefaultAuth is the policy a site gets when it says nothing.
func DefaultAuth() AuthPolicy {
	return AuthPolicy{
		SessionDays: 30, APIKeyDays: 0, MinPasswordLength: 8,
		MaxLoginAttempts: 5, LockoutMinutes: 15, ResetMinutes: 60, InviteHours: 72,
	}
}

func (a AuthPolicy) SessionTTL() time.Duration { return time.Duration(a.SessionDays) * 24 * time.Hour }
func (a AuthPolicy) LockoutWindow() time.Duration {
	return time.Duration(a.LockoutMinutes) * time.Minute
}
func (a AuthPolicy) ResetTTL() time.Duration  { return time.Duration(a.ResetMinutes) * time.Minute }
func (a AuthPolicy) InviteTTL() time.Duration { return time.Duration(a.InviteHours) * time.Hour }

// APIKeyTTL is zero when keys do not expire.
func (a AuthPolicy) APIKeyTTL() time.Duration {
	return time.Duration(a.APIKeyDays) * 24 * time.Hour
}

func (a AuthPolicy) AllowSelfServiceAPIKeys() bool {
	return a.SelfServiceAPIKeys == nil || *a.SelfServiceAPIKeys
}

func (a AuthPolicy) AllowPasswordLogin() bool {
	return a.PasswordLogin == nil || *a.PasswordLogin
}

func (a AuthPolicy) validate() error {
	for _, c := range []struct {
		name string
		v    int
	}{
		{"sessionDays", a.SessionDays},
		{"maxLoginAttempts", a.MaxLoginAttempts},
		{"lockoutMinutes", a.LockoutMinutes},
		{"resetMinutes", a.ResetMinutes},
		{"inviteHours", a.InviteHours},
	} {
		if c.v <= 0 {
			return fmt.Errorf("auth.%s must be greater than zero", c.name)
		}
	}
	if a.APIKeyDays < 0 {
		return fmt.Errorf("auth.apiKeyDays must be zero (never expires) or greater")
	}
	// Below 6 the minimum stops being a policy and starts being a formality;
	// above 128 Argon2 is being asked to hash a document.
	if a.MinPasswordLength < 6 || a.MinPasswordLength > 128 {
		return fmt.Errorf("auth.minPasswordLength must be between 6 and 128")
	}
	for id, pol := range a.SSO {
		for g, roles := range pol.GroupRoles {
			if strings.TrimSpace(g) == "" {
				return fmt.Errorf("auth.sso.%s.groupRoles: a group name is empty", id)
			}
			if len(roles) == 0 {
				return fmt.Errorf("auth.sso.%s.groupRoles.%s: maps to no role", id, g)
			}
			for _, r := range roles {
				switch strings.ToLower(strings.TrimSpace(r)) {
				case "":
					return fmt.Errorf("auth.sso.%s.groupRoles.%s: a role name is empty", id, g)
				case "admin", "guest", "all":
					// Admin is an account, not a role to hand out; Guest and
					// All are held by everyone already.
					return fmt.Errorf("auth.sso.%s.groupRoles.%s: %q cannot be granted by a group", id, g, r)
				}
			}
		}
	}
	return nil
}
