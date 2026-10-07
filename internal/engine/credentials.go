package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Sign-in through an app: credential providers (#115).
//
// An app declares, in defineApp's auth.providers, a way to check a username
// and a password against a system of its own — an ERP, a directory. ddcore
// keeps everything that makes that a sign-in: the throttle, the tenant, which
// User the username belongs to, the session and the audit. The app only
// answers "is this the password of this username", inside the tenant the
// person picked, and never chooses the account: that is the User whose
// userField holds the username, which an administrator filled in beforehand.

// credentialVerifyTimeout bounds a provider's verify: it usually calls
// another system over the network while a person waits on the sign-in screen.
const credentialVerifyTimeout = 20 * time.Second

// credentialTenantsTTL is how long the list of tenants offering a provider is
// kept. The list is public, so without a cache every anonymous visit to the
// sign-in screen would run enabled() once per tenant.
const credentialTenantsTTL = time.Minute

// credentialDecl is a provider as defineApp declared it, functions replaced
// by flags.
type credentialDecl struct {
	Label          string `json:"label"`
	UserField      string `json:"userField"`
	HasEnabled     bool   `json:"hasEnabled"`
	HasVerify      bool   `json:"hasVerify"`
	HasAfterSignIn bool   `json:"hasAfterSignIn"`
}

// CredentialProvider is a validated provider.
type CredentialProvider struct {
	ID             string
	App            string
	Label          string
	UserField      string
	HasEnabled     bool
	HasAfterSignIn bool
}

// credentialIDPattern is the OIDC providers' grammar: the two kinds share the
// sign-in screen and the audit detail, so an id names one of either.
var credentialIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// buildCredentialProviders validates every app's auth.providers.
func buildCredentialProviders(apps []js.App, snap *Snapshot, reg *meta.Registry, oidc []config.OIDCProvider) (map[string]CredentialProvider, error) {
	out := map[string]CredentialProvider{}
	for _, a := range apps {
		am := snap.Apps[a.Name]
		if am == nil || len(am.Auth.Providers) == 0 {
			continue
		}
		ids := make([]string, 0, len(am.Auth.Providers))
		for id := range am.Auth.Providers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			d := am.Auth.Providers[id]
			fail := func(format string, args ...any) error {
				return fmt.Errorf("app %q: auth provider %q: %s", a.Name, id, fmt.Sprintf(format, args...))
			}
			if !credentialIDPattern.MatchString(id) {
				return nil, fail("the id must be lowercase letters, digits and _, starting with a letter, at most 32 characters")
			}
			if other, ok := out[id]; ok {
				return nil, fail("the id is taken by app %q", other.App)
			}
			for _, p := range oidc {
				if p.ID == id {
					return nil, fail("the id is taken by a single sign-on provider (DDCORE_OIDC_PROVIDERS)")
				}
			}
			if strings.TrimSpace(d.Label) == "" {
				return nil, fail("label is required")
			}
			if !d.HasVerify {
				return nil, fail("verify must be a function")
			}
			if err := credentialUserField(reg, d.UserField); err != nil {
				return nil, fail("%v", err)
			}
			out[id] = CredentialProvider{ID: id, App: a.Name, Label: d.Label, UserField: d.UserField,
				HasEnabled: d.HasEnabled, HasAfterSignIn: d.HasAfterSignIn}
		}
	}
	return out, nil
}

// credentialUserField checks that the field a provider looks the account up
// by is a stored text field of User — one an app adds with extendDoctype.
func credentialUserField(reg *meta.Registry, name string) error {
	if name == "" {
		return errors.New("userField is required: the User field that holds the username")
	}
	u, ok := reg.Get("User")
	if !ok {
		return errors.New("there is no User DocType")
	}
	f := u.Field(name)
	if f == nil {
		return fmt.Errorf("userField %q is not a field of User (add it with extendDoctype)", name)
	}
	if f.Fieldtype != "Data" || f.Computed {
		return fmt.Errorf("userField %q must be a stored Data field, not %s", name, f.Fieldtype)
	}
	return nil
}

// CredentialProviders returns the providers sorted by id.
func (s *State) CredentialProviders() []CredentialProvider {
	out := make([]CredentialProvider, 0, len(s.credentials))
	for _, p := range s.credentials {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CredentialProvider returns the provider with this id.
func (s *State) CredentialProvider(id string) (CredentialProvider, bool) {
	p, ok := s.credentials[id]
	return p, ok
}

// credentialHook runs one of a provider's hooks on c, permissions ignored and
// bounded by ctx's deadline.
func credentialHook(c *Ctx, ctx context.Context, p CredentialProvider, hook string, args any) (json.RawMessage, error) {
	rt, err := c.RT()
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if args != nil {
		if raw, err = json.Marshal(args); err != nil {
			return nil, err
		}
	}
	var out json.RawMessage
	err = c.WithIgnorePermissions(func() error {
		return rt.WithContext(ctx, func() (err error) {
			out, err = rt.AuthProvider(p.App, p.ID, hook, raw)
			return err
		})
	})
	return out, err
}

// credentialEnabled asks a provider whether it is offered in c's space.
func credentialEnabled(c *Ctx, p CredentialProvider) (bool, error) {
	if !p.HasEnabled {
		return true, nil
	}
	out, err := credentialHook(c, c.Ctx, p, "enabled", nil)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// CredentialTenants is the enabled tenants where the provider is offered,
// for the sign-in screen's list, and whether the site has tenancy at all. It
// is public: a tenant is listed only when its app's enabled() says so.
// Without tenancy the list is empty and no tenant is asked for.
func (e *Engine) CredentialTenants(ctx context.Context, id string) (list []map[string]any, tenancy bool, err error) {
	st := e.Current()
	p, ok := st.CredentialProvider(id)
	if !ok {
		return nil, false, cerr.NotFound("Unknown sign-in provider")
	}
	if on, err := e.tenancy(ctx); err != nil || !on {
		return []map[string]any{}, false, err
	}
	key := "credtenants:" + id + ":" + strconv.FormatInt(st.Loaded.UnixNano(), 10)
	if v, ok := e.Cache.Get(key); ok {
		return v.([]map[string]any), true, nil
	}
	gen := e.Cache.Gen()
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT id, title FROM `+meta.TenantTable+`
		WHERE enabled IS TRUE ORDER BY coalesce(nullif(title, ''), id), id`)
	if err != nil {
		return nil, true, err
	}
	out := []map[string]any{}
	for _, r := range rows {
		tenant := db.Str(r["id"])
		var offered bool
		err := e.Run(WithTenant(ctx, tenant), "Admin", func(c *Ctx) (err error) {
			offered, err = credentialEnabled(c, p)
			return err
		})
		if err != nil {
			// one tenant's broken settings must not empty everybody's list
			e.Log.Warn("sign-in provider: enabled() failed", "provider", id, "tenant", tenant, "err", err)
			continue
		}
		if offered {
			title := db.Str(r["title"])
			if title == "" {
				title = tenant
			}
			out = append(out, map[string]any{"id": tenant, "title": title})
		}
	}
	e.Cache.SetAt(key, out, credentialTenantsTTL, gen)
	return out, true, nil
}

// credentialRefusal is a refused sign-in: what the person is told, and the
// short reason the audit records.
type credentialRefusal struct {
	reason string
	err    error
}

func (r *credentialRefusal) Error() string { return r.err.Error() }
func (r *credentialRefusal) Unwrap() error { return r.err }

func refuseCredential(reason string, err error) *credentialRefusal {
	return &credentialRefusal{reason: reason, err: err}
}

// credentialAccountKey is the throttle key of one username in one tenant
// through one provider. Like a password sign-in's, it is the name as typed:
// an unknown username is counted the same as a known one.
func credentialAccountKey(provider, tenant, username string) string {
	return throttleKey("cred:"+provider, tenant+"/"+username)
}

// CredentialLogin signs a person in through an app's provider and returns
// the new session.
//
// It runs in two transactions on purpose. verify usually waits on another
// system over the network, and no transaction should stay open across that
// wait; the second one only looks the account up and opens the session.
func (e *Engine) CredentialLogin(ctx context.Context, id, tenant, username, password string, from LoginFrom) (string, error) {
	sid, err := e.credentialLogin(ctx, id, tenant, username, password, from)
	// the audit reason stays here; the caller gets the error to answer with
	var r *credentialRefusal
	if errors.As(err, &r) {
		return "", r.err
	}
	return sid, err
}

func (e *Engine) credentialLogin(ctx context.Context, id, tenant, username, password string, from LoginFrom) (sid string, err error) {
	st := e.Current()
	p, ok := st.CredentialProvider(id)
	if !ok {
		return "", cerr.NotFound("Unknown sign-in provider")
	}
	username, tenant = strings.TrimSpace(username), strings.TrimSpace(tenant)
	if username == "" || password == "" {
		return "", cerr.Validation("Enter your username and password")
	}
	on, err := e.tenancy(ctx)
	if err != nil {
		return "", err
	}
	if on && tenant == "" {
		return "", cerr.Validation("Choose where to sign in")
	}
	if !on {
		tenant = ""
	}

	pol := e.Cfg.Auth
	acctKey := credentialAccountKey(id, tenant, username)
	ipKey := throttleKey("loginip", from.IP)
	if err := e.CheckThrottle(ctx, acctKey, pol.MaxLoginAttempts, pol.LockoutWindow()); err != nil {
		return "", err
	}
	if from.IP != "" {
		if err := e.CheckThrottle(ctx, ipKey, pol.MaxLoginAttempts*5, pol.LockoutWindow()); err != nil {
			return "", err
		}
	}

	var user, subject string
	// where the audit row goes: the tenant once it is known to exist, else
	// the platform space — a row in a space nobody has would be read by nobody
	auditSpace := ""
	countAccount := true
	defer func() {
		// Recorded on the pool, after the transactions: an attempt written
		// inside one would be rolled back by the error it is counting.
		if countAccount {
			e.RecordAttempt(ctx, acctKey, from.IP, err == nil)
		}
		if from.IP != "" {
			e.RecordAttempt(ctx, ipKey, from.IP, err == nil)
		}
		if err == nil {
			e.ClearAttempts(ctx, acctKey)
			e.ClearAttempts(ctx, throttleKey("login", user))
			if from.IP != "" {
				e.ClearAttempts(ctx, ipKey)
			}
		}
		detail := map[string]any{"provider": id, "username": username}
		if tenant != "" {
			detail["tenant"] = tenant
		}
		if subject != "" && subject != username {
			detail["subject"] = subject
		}
		outcome := "Allowed"
		if err != nil {
			outcome = "Denied"
			detail["reason"] = "error"
			var r *credentialRefusal
			if errors.As(err, &r) {
				detail["reason"] = r.reason
			}
		}
		actor := user
		if actor == "" {
			actor = "Guest"
		}
		if aerr := e.InSpace(ctx, auditSpace, func(q db.Querier) error {
			return e.RecordAuditOn(ctx, q, actor, "account.login_credentials", outcome, "User", user, from.IP, "", detail)
		}); aerr != nil {
			e.Log.Warn("could not record a sign-in", "err", aerr)
		}
		if err != nil {
			e.Log.Info("credential sign-in refused", "provider", id, "tenant", tenant, "username", username, "err", err)
		}
	}()

	invalid := cerr.Auth("Invalid username or password")
	if on {
		// After the throttle and with the same answer as a wrong password:
		// whether a tenant exists is not something to tell a stranger, past
		// what the public list already says.
		if !db.ValidTenantID(tenant) || e.checkTenant(ctx, nil, tenant) != nil {
			return "", refuseCredential("tenant", invalid)
		}
		auditSpace = tenant
	}
	space := ctx
	if tenant != "" {
		space = WithTenant(ctx, tenant)
	}
	label := p.Label

	// Phase 1: is this the password of this username, there?
	var result struct {
		OK      json.RawMessage `json:"ok"`
		Subject any             `json:"subject"`
		Data    json.RawMessage `json:"data"`
		Reason  any             `json:"reason"`
	}
	err = e.Run(space, "Admin", func(c *Ctx) error {
		label = c.T(p.Label)
		offered, err := credentialEnabled(c, p)
		if err != nil {
			return err
		}
		if !offered {
			// the list the browser was given may be stale, or forged
			return refuseCredential("not_offered", cerr.Auth("Sign-in with {0} is not available here", label))
		}
		vctx, cancel := context.WithTimeout(ctx, credentialVerifyTimeout)
		defer cancel()
		out, err := credentialHook(c, vctx, p, "verify", map[string]any{"username": username, "password": password})
		if err != nil {
			return err
		}
		if err := json.Unmarshal(out, &result); err != nil {
			result.OK = nil
		}
		return nil
	})
	if err != nil {
		var r *credentialRefusal
		if errors.As(err, &r) {
			countAccount = false
			return "", err
		}
		// The other system is down, slow or the app has a bug: not the
		// person's fault, so only the address is counted — an outage there
		// must not lock everybody out here. The error is logged, never the
		// arguments: they carry the password.
		countAccount = false
		e.Log.Warn("sign-in provider: verify failed", "provider", id, "tenant", tenant, "err", err)
		return "", refuseCredential("unavailable", cerr.Unavailable("Sign-in with {0} is unavailable. Try again later.", label))
	}
	if strings.TrimSpace(string(result.OK)) != "true" {
		reason := "invalid"
		if s, ok := result.Reason.(string); ok && s != "" {
			reason = "invalid: " + truncate(s, 200)
		}
		return "", refuseCredential(reason, invalid)
	}
	subject = username
	if s, ok := result.Subject.(string); ok && strings.TrimSpace(s) != "" && len(s) <= 255 {
		subject = strings.TrimSpace(s)
	}

	// Phase 2: the account an administrator linked to that username.
	err = e.Run(space, "Admin", func(c *Ctx) error {
		rows, err := db.Select(c.Ctx, c.Tx, `SELECT id, enabled FROM tab_user WHERE lower(`+db.Ident(p.UserField)+`) = lower($1)`, subject)
		if err != nil {
			return err
		}
		switch {
		case len(rows) == 0:
			// Said plainly: the person has just proved the password is theirs.
			return refuseCredential("no_account", cerr.Auth("There is no account for this user here. Ask an administrator to add you."))
		case len(rows) > 1:
			e.Log.Warn("sign-in provider: a username is linked to more than one User", "provider", id, "tenant", tenant, "username", subject)
			return refuseCredential("ambiguous", cerr.Auth("This user is linked to more than one account. Ask an administrator."))
		}
		name := db.Str(rows[0]["id"])
		if name == "Admin" || name == "Guest" {
			return refuseCredential("no_account", cerr.Auth("There is no account for this user here. Ask an administrator to add you."))
		}
		user = name
		if en, ok := rows[0]["enabled"].(bool); ok && !en {
			return refuseCredential("disabled", cerr.Auth("User is disabled"))
		}
		if p.HasAfterSignIn {
			args := map[string]any{"user": user, "username": username, "subject": subject}
			if len(result.Data) > 0 {
				args["data"] = result.Data
			}
			if _, err := credentialHook(c, ctx, p, "afterSignIn", args); err != nil {
				e.Log.Warn("sign-in provider: afterSignIn failed", "provider", id, "tenant", tenant, "user", user, "err", err)
				return refuseCredential("unavailable", cerr.Unavailable("Sign-in with {0} is unavailable. Try again later.", label))
			}
		}
		sid, err = e.createSession(c, user, from)
		return err
	})
	if err != nil {
		var r *credentialRefusal
		if errors.As(err, &r) && r.reason == "unavailable" {
			countAccount = false
		}
		return "", err
	}
	return sid, nil
}

// ClearCredentialAttempts forgets the failed credential sign-ins of a user:
// for each provider, the username in the user's userField, in the user's
// tenant. c works in that tenant. It is what unlocking a user does besides
// clearing the password sign-in counter.
func (e *Engine) ClearCredentialAttempts(c *Ctx, user string) (int, error) {
	n := 0
	for _, p := range c.St.CredentialProviders() {
		var v any
		if err := c.WithIgnorePermissions(func() (err error) {
			rows, err := db.Select(c.Ctx, c.Q(), `SELECT `+db.Ident(p.UserField)+` AS v FROM tab_user WHERE id = $1`, user)
			if len(rows) > 0 {
				v = rows[0]["v"]
			}
			return err
		}); err != nil {
			return n, err
		}
		username := strings.TrimSpace(db.Str(v))
		if username == "" {
			continue
		}
		m, err := e.ClearAttempts(c.Ctx, credentialAccountKey(p.ID, c.Tenant, username))
		if err != nil {
			return n, err
		}
		n += m
	}
	return n, nil
}
