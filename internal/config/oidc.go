package config

import (
	"fmt"
	"regexp"
	"strings"
)

// OIDCProvider is one identity provider a person may sign in through: Google,
// a self-hosted PocketID, or anything else that speaks OpenID Connect.
//
// Like Mail it is read from the environment only. The client secret is a
// secret, and the issuer of a self-hosted provider differs between staging and
// production, so neither belongs in the committed ddcore.json.
type OIDCProvider struct {
	ID           string // lowercase, used in the callback path
	Label        string // what the sign-in button says
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       []string
	// AllowedDomains restricts sign-in to e-mail addresses in these domains.
	// Empty means any domain — which is still only the addresses that already
	// belong to a User.
	AllowedDomains []string
}

// GoogleIssuer is the issuer a provider with the id "google" gets by default.
const GoogleIssuer = "https://accounts.google.com"

var oidcIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

var oidcDefaultLabels = map[string]string{"google": "Google", "pocketid": "PocketID"}

func oidcFromEnv() ([]OIDCProvider, error) {
	list := strings.TrimSpace(env("DDCORE_OIDC_PROVIDERS", ""))
	if list == "" {
		return nil, nil
	}
	var out []OIDCProvider
	seen := map[string]bool{}
	for _, raw := range strings.Split(list, ",") {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if !oidcIDPattern.MatchString(id) {
			return nil, fmt.Errorf("DDCORE_OIDC_PROVIDERS: %q is not a valid provider id (lowercase letters, digits and _)", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("DDCORE_OIDC_PROVIDERS: %q is listed twice", id)
		}
		seen[id] = true
		prefix := "DDCORE_OIDC_" + strings.ToUpper(id) + "_"
		p := OIDCProvider{
			ID:           id,
			Label:        env(prefix+"LABEL", oidcDefaultLabels[id]),
			Issuer:       strings.TrimSuffix(env(prefix+"ISSUER", ""), "/"),
			ClientID:     env(prefix+"CLIENT_ID", ""),
			ClientSecret: env(prefix+"CLIENT_SECRET", ""),
			Scopes:       strings.Fields(strings.ReplaceAll(env(prefix+"SCOPES", "openid email profile"), ",", " ")),
		}
		if p.Label == "" {
			p.Label = id
		}
		if p.Issuer == "" && id == "google" {
			p.Issuer = GoogleIssuer
		}
		for _, d := range strings.Split(env(prefix+"ALLOWED_DOMAINS", ""), ",") {
			if d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@")); d != "" {
				p.AllowedDomains = append(p.AllowedDomains, d)
			}
		}
		if p.Issuer == "" {
			return nil, fmt.Errorf("provider %q needs %sISSUER", id, prefix)
		}
		if !strings.HasPrefix(p.Issuer, "https://") && !strings.HasPrefix(p.Issuer, "http://") {
			return nil, fmt.Errorf("%sISSUER: %q must be an http(s) URL", prefix, p.Issuer)
		}
		if p.ClientID == "" || p.ClientSecret == "" {
			return nil, fmt.Errorf("provider %q needs %sCLIENT_ID and %sCLIENT_SECRET", id, prefix, prefix)
		}
		hasOpenID := false
		for _, s := range p.Scopes {
			hasOpenID = hasOpenID || s == "openid"
		}
		if !hasOpenID {
			p.Scopes = append([]string{"openid"}, p.Scopes...)
		}
		out = append(out, p)
	}
	return out, nil
}

// validateOIDC checks what can only be checked once the whole file is known:
// a callback URL needs the site's public address, and a site that turns
// password sign-in off must leave some other way in.
func (f *File) validateOIDC() error {
	if len(f.OIDC) > 0 && !f.HasPublicURL() {
		// The redirect URI is registered at the provider and must match
		// exactly; a localhost fallback would work on one laptop and fail
		// everywhere else with an error from the provider nobody can read.
		return fmt.Errorf("DDCORE_OIDC_PROVIDERS needs DDCORE_URL: the callback address is built from it")
	}
	if !f.Auth.AllowPasswordLogin() && len(f.OIDC) == 0 {
		return fmt.Errorf("auth.passwordLogin is false but no DDCORE_OIDC_PROVIDERS is configured: nobody but Administrator could sign in")
	}
	return nil
}
