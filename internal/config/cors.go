package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// CORS is which other origins may call the methods that opt in with
// `cors: true` (#68). It is a list of the site's own partners, so it lives in
// ddcore.json; DDCORE_CORS_ORIGINS replaces it where a deployment differs — a
// staging front end, a developer's localhost.
type CORS struct {
	// Origins are "*", "scheme://host[:port]" or "scheme://*.domain", which
	// matches any subdomain of domain but not domain itself.
	Origins []string `json:"origins,omitempty"`
}

// fromEnv applies DDCORE_CORS_ORIGINS, comma-separated, over the file.
func (c *CORS) fromEnv() {
	v := strings.TrimSpace(os.Getenv("DDCORE_CORS_ORIGINS"))
	if v == "" {
		return
	}
	c.Origins = nil
	for _, o := range strings.Split(v, ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.Origins = append(c.Origins, o)
		}
	}
}

// validate refuses an entry no browser would ever send as an Origin. A typo
// there does not fail loudly on its own: the partner's calls are simply
// refused, and the first to notice is their customer.
func (c CORS) validate() error {
	for _, o := range c.Origins {
		if err := validOrigin(o); err != nil {
			return fmt.Errorf("cors.origins: %q %w", o, err)
		}
	}
	return nil
}

func validOrigin(o string) error {
	if o == "*" {
		return nil
	}
	u, err := url.Parse(o)
	if err != nil || u.Opaque != "" {
		return fmt.Errorf("is not scheme://host[:port]")
	}
	if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
		return fmt.Errorf("must start with http:// or https://")
	}
	if u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(o, "?") || strings.HasSuffix(o, "#") {
		return fmt.Errorf("must be scheme://host[:port], with nothing after it: an Origin has no path")
	}
	host := u.Hostname()
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("has an invalid port")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return fmt.Errorf("has an invalid port")
	}
	domain, wild := strings.CutPrefix(host, "*.")
	if domain == "" || strings.Contains(domain, "*") || (wild && !strings.Contains(domain, ".")) {
		return fmt.Errorf("may hold a wildcard only as its first label, as https://*.example.com")
	}
	return nil
}
