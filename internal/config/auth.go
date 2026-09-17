package config

import (
	"fmt"
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
	// except Administrator, who keeps a password so that an outage at the
	// identity provider is not also an outage of the site's administration.
	PasswordLogin *bool `json:"passwordLogin,omitempty"`
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
	return nil
}
