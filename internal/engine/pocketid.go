package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
)

// PocketID provisioning (SEC-05, part).
//
// With DDCORE_OIDC_<ID>_API_KEY set on a provider of kind pocketid, ddcore
// is where accounts are made: an invitation creates the person in PocketID and
// mails them PocketID's one-time link to register a passkey, a save of a
// User pushes its mapped roles back as PocketID groups, and the desk offers to
// disable the PocketID account when a User is disabled or deleted.
//
// The admin API is plain REST with an X-API-Key header. Its PUT replaces the
// whole user, so every update reads the user first and sends it all back.

// pocketIDHTTP is the client for the admin API. Like oidcHTTP it has a short
// timeout: an invitation waits on it.
var pocketIDHTTP = &http.Client{Timeout: 10 * time.Second}

// pocketIDMaxTTL is the longest a one-time access token may live.
const pocketIDMaxTTL = 31 * 24 * time.Hour

// Job that pushes a User's mapped roles to PocketID as groups.
const (
	idpSyncJobMethod = "core.services.idp.sync"
	idpQueue         = "idp"
	// idpSyncingFlag marks a Ctx whose User save came *from* the provider — a
	// sign-in applying the groups — so the save does not push them back.
	idpSyncingFlag = "idpSyncing"
)

type pocketIDAdmin struct {
	provider config.OIDCProvider
	policy   config.SSOPolicy
}

// idpAdmin is the provisioning provider, or nil when provisioning is off.
func (e *Engine) idpAdmin() *pocketIDAdmin {
	for _, p := range e.Cfg.OIDC {
		if p.Provisions() {
			return &pocketIDAdmin{provider: p, policy: e.Cfg.Auth.SSO[p.ID]}
		}
	}
	return nil
}

// pidUser is a user as the admin API returns it. Kept as a map so that a
// read-modify-write sends back what it read.
type pidUser map[string]any

func (u pidUser) id() string    { return db.Str(u["id"]) }
func (u pidUser) email() string { return db.Str(u["email"]) }

// groups is the user's groups, name → id.
func (u pidUser) groups() map[string]string {
	out := map[string]string{}
	list, _ := u["userGroups"].([]any)
	for _, g := range list {
		if m, ok := g.(map[string]any); ok {
			out[db.Str(m["name"])] = db.Str(m["id"])
		}
	}
	return out
}

// updateBody is the UserCreateDto a PUT needs, taken from what a GET returned.
func (u pidUser) updateBody() map[string]any {
	out := map[string]any{}
	for _, k := range []string{"username", "email", "firstName", "lastName", "displayName", "isAdmin", "disabled", "emailVerified", "locale"} {
		if v, ok := u[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

// pidError is a refusal from PocketID, told to the admin in PocketID's words.
func pidError(what string, status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		msg = e.Error
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return cerr.Validation("PocketID refused to {0} ({1}): {2}", what, status, msg)
}

func (p *pocketIDAdmin) call(ctx context.Context, what, method, path string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.provider.Issuer+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-API-Key", p.provider.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := pocketIDHTTP.Do(req)
	if err != nil {
		return 0, cerr.Unavailable("PocketID did not answer ({0}): {1}", what, err.Error())
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusNotFound {
		return res.StatusCode, nil
	}
	if res.StatusCode >= 300 {
		return res.StatusCode, pidError(what, res.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return res.StatusCode, cerr.Internal("PocketID answered {0} with something that is not JSON", what)
		}
	}
	return res.StatusCode, nil
}

// getUser is nil when there is no such user.
func (p *pocketIDAdmin) getUser(ctx context.Context, id string) (pidUser, error) {
	var u pidUser
	st, err := p.call(ctx, "read a user", http.MethodGet, "/api/users/"+url.PathEscape(id), nil, &u)
	if err != nil || st == http.StatusNotFound {
		return nil, err
	}
	return u, nil
}

// findUserByEmail is nil when nobody has that address. The search is a
// substring match over several columns, so the exact match is picked here.
func (p *pocketIDAdmin) findUserByEmail(ctx context.Context, email string) (pidUser, error) {
	q := url.Values{"search": {email}, "pagination[limit]": {"100"}}
	var page struct {
		Data []pidUser `json:"data"`
	}
	if _, err := p.call(ctx, "search users", http.MethodGet, "/api/users?"+q.Encode(), nil, &page); err != nil {
		return nil, err
	}
	for _, u := range page.Data {
		if strings.EqualFold(u.email(), email) {
			// the list may carry less than the user itself does
			return p.getUser(ctx, u.id())
		}
	}
	return nil, nil
}

// locate finds the PocketID account of a User: by the subject a sign-in
// linked, then by address. nil when it has none.
func (p *pocketIDAdmin) locate(c *Ctx, user, email string) (pidUser, error) {
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT subject FROM ddcore_user_identity WHERE provider = $1 AND "user" = $2`, p.provider.ID, user)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		u, err := p.getUser(c.Ctx, db.Str(r["subject"]))
		if err != nil || u != nil {
			return u, err
		}
	}
	if email == "" {
		return nil, nil
	}
	return p.findUserByEmail(c.Ctx, email)
}

func (p *pocketIDAdmin) updateUser(ctx context.Context, u pidUser, change func(map[string]any)) (pidUser, error) {
	body := u.updateBody()
	change(body)
	var out pidUser
	if _, err := p.call(ctx, "update a user", http.MethodPut, "/api/users/"+url.PathEscape(u.id()), body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// groupIDs resolves group names to ids. A mapped group that does not exist
// yet is created: on a fresh setup nothing else would make it.
func (p *pocketIDAdmin) groupIDs(ctx context.Context, names []string) (map[string]string, error) {
	out := map[string]string{}
	for _, n := range names {
		id, err := p.findGroup(ctx, n)
		if err != nil {
			return nil, err
		}
		if id == "" {
			if id, err = p.createGroup(ctx, n); err != nil {
				return nil, err
			}
		}
		out[n] = id
	}
	return out, nil
}

// findGroup is the id of the group with exactly that name, or "" when there
// is none.
func (p *pocketIDAdmin) findGroup(ctx context.Context, name string) (string, error) {
	q := url.Values{"search": {name}, "pagination[limit]": {"100"}}
	var page struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if _, err := p.call(ctx, "list groups", http.MethodGet, "/api/user-groups?"+q.Encode(), nil, &page); err != nil {
		return "", err
	}
	for _, g := range page.Data {
		if g.Name == name {
			return g.ID, nil
		}
	}
	return "", nil
}

// createGroup makes a mapped group PocketID does not have. A refusal names
// the group and says how to make it by hand.
func (p *pocketIDAdmin) createGroup(ctx context.Context, name string) (string, error) {
	var g struct {
		ID string `json:"id"`
	}
	body := map[string]any{"name": name, "friendlyName": p.friendlyName(name)}
	if _, err := p.call(ctx, "create a group", http.MethodPost, "/api/user-groups", body, &g); err != nil {
		return "", cerr.Validation("PocketID has no group {0}, which auth.sso.{1}.groupRoles maps, and creating it failed: {2}. Create the group in PocketID (Administration → User Groups) and try again", name, p.provider.ID, err)
	}
	if g.ID == "" {
		return "", cerr.Internal("PocketID created group {0} but answered without its id", name)
	}
	return g.ID, nil
}

// friendlyName is the display name of a group ddcore creates: the roles it
// maps to, within PocketID's 2 to 50 characters.
func (p *pocketIDAdmin) friendlyName(name string) string {
	f := strings.Join(p.policy.GroupRoles[name], ", ")
	if r := []rune(f); len(r) > 50 {
		f = strings.TrimSpace(string(r[:50]))
	}
	if len([]rune(f)) < 2 {
		f = name
	}
	return f
}

// syncGroups sets the user's mapped groups from the User's roles and leaves
// every group the map does not name as it was.
func (p *pocketIDAdmin) syncGroups(ctx context.Context, u pidUser, roles []string) error {
	if len(p.policy.GroupRoles) == 0 {
		return nil
	}
	has := map[string]bool{}
	for _, r := range roles {
		has[r] = true
	}
	want := p.policy.GroupsFor(has)
	current := u.groups()
	ids := map[string]bool{}
	var missing []string
	for name, id := range current {
		if _, managed := p.policy.GroupRoles[name]; !managed {
			ids[id] = true
		}
	}
	for name := range want {
		if id, ok := current[name]; ok {
			ids[id] = true
		} else {
			missing = append(missing, name)
		}
	}
	changed := len(missing) > 0
	for name := range current {
		if _, managed := p.policy.GroupRoles[name]; managed && !want[name] {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	sort.Strings(missing)
	found, err := p.groupIDs(ctx, missing)
	if err != nil {
		return err
	}
	for _, id := range found {
		ids[id] = true
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	sort.Strings(list)
	_, err = p.call(ctx, "set a user's groups", http.MethodPut, "/api/users/"+url.PathEscape(u.id())+"/user-groups", map[string]any{"userGroupIds": list}, nil)
	return err
}

// pocketIDUsername turns an address into something PocketID accepts: letters,
// digits and _ . @ - inside, a letter or digit at each end.
var pidUsernameBad = regexp.MustCompile(`[^a-zA-Z0-9_.@-]`)

func pocketIDUsername(email string) string {
	s := pidUsernameBad.ReplaceAllString(strings.ToLower(email), "-")
	s = strings.Trim(s, "_.@-")
	if len(s) > 50 {
		s = strings.Trim(s[:50], "_.@-")
	}
	if s == "" {
		s = "user"
	}
	return s
}

// splitName is a best guess at first and last name; displayName carries the
// full name as typed either way.
func splitName(full string) (string, string) {
	full = strings.TrimSpace(full)
	if i := strings.LastIndex(full, " "); i > 0 {
		return full[:i], full[i+1:]
	}
	return full, ""
}

// provision makes sure the User has an enabled PocketID account with the
// groups its roles map to, and returns it. An account that already exists —
// made by hand, or by an invitation whose transaction was rolled back — is
// reused, not duplicated.
func (p *pocketIDAdmin) provision(c *Ctx, user string) (pidUser, error) {
	doc, err := c.GetDocIgnoringPerms("User", user)
	if err != nil {
		return nil, err
	}
	email := db.Str(doc["email"])
	fullName := db.Str(doc["full_name"])
	roles := docRoles(doc)
	u, err := p.locate(c, user, email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		first, last := splitName(fullName)
		body := map[string]any{
			"username": pocketIDUsername(email), "email": email,
			"firstName": first, "lastName": last, "displayName": fullName,
			// the invitation is going to that address: whoever registers a
			// passkey through it has shown they read it
			"emailVerified": true, "disabled": false, "isAdmin": false,
		}
		if _, err := p.call(c.Ctx, "create a user", http.MethodPost, "/api/users", body, &u); err != nil {
			return nil, err
		}
		c.E.Log.Info("pocketid: account created", "user", user, "pocketid", u.id())
	} else if u["disabled"] == true {
		if u, err = p.updateUser(c.Ctx, u, func(b map[string]any) { b["disabled"] = false }); err != nil {
			return nil, err
		}
	}
	if err := p.syncGroups(c.Ctx, u, roles); err != nil {
		return nil, err
	}
	return u, nil
}

// accessLink issues PocketID's one-time login link for the account.
func (p *pocketIDAdmin) accessLink(ctx context.Context, u pidUser, ttl time.Duration) (string, time.Time, error) {
	if ttl > pocketIDMaxTTL {
		ttl = pocketIDMaxTTL
	}
	var out struct {
		Token string `json:"token"`
	}
	if _, err := p.call(ctx, "issue a login code", http.MethodPost, "/api/users/"+url.PathEscape(u.id())+"/one-time-access-token",
		map[string]any{"ttl": int(ttl.Seconds())}, &out); err != nil {
		return "", time.Time{}, err
	}
	if out.Token == "" {
		return "", time.Time{}, cerr.Internal("PocketID issued an empty login code")
	}
	return p.provider.Issuer + "/lc/" + url.PathEscape(out.Token), time.Now().Add(ttl), nil
}

func docRoles(doc Doc) []string {
	var out []string
	list, _ := doc["roles"].([]any)
	for _, r := range list {
		if m, ok := r.(map[string]any); ok && db.Str(m["role"]) != "" {
			out = append(out, db.Str(m["role"]))
		}
		if m, ok := r.(Doc); ok && db.Str(m["role"]) != "" {
			out = append(out, db.Str(m["role"]))
		}
	}
	return out
}

// inviteThroughPocketID is InviteUser and ResendInvite for a site that
// provisions: the account is made (or found) in PocketID, and the message
// carries PocketID's one-time link instead of a link to set a password.
func (e *Engine) inviteThroughPocketID(c *Ctx, p *pocketIDAdmin, user string) (*Recovery, error) {
	u, err := p.provision(c, user)
	if err != nil {
		return nil, err
	}
	ttl := e.Cfg.Auth.InviteTTL()
	link, expires, err := p.accessLink(c.Ctx, u, ttl)
	if err != nil {
		return nil, err
	}
	if ttl > pocketIDMaxTTL {
		ttl = pocketIDMaxTTL
	}
	args := map[string]any{
		"link": link, "provider": p.provider.Label,
		"loginUrl": strings.TrimSuffix(e.Cfg.SiteURL, "/") + "/login",
		"hours":    int(ttl.Hours()),
	}
	if err := c.SendTemplate(MailTemplateInviteSSO, user, args); err != nil {
		return nil, err
	}
	r := &Recovery{Expires: expires, Delivered: e.MailDelivers()}
	if !r.Delivered {
		r.Link = link
	}
	return r, nil
}

// QueueIdPSync is called by the User controller when roles changed. It
// queues a push of the mapped roles to PocketID, unless nothing is mapped,
// no mapped role changed, or the change came from the provider itself.
func (e *Engine) QueueIdPSync(c *Ctx, user string, before, after []string) error {
	p := e.idpAdmin()
	if p == nil || len(p.policy.GroupRoles) == 0 || user == "Admin" || c.Flags[idpSyncingFlag] == true {
		return nil
	}
	managed := p.policy.ManagedRoles()
	was, is := map[string]bool{}, map[string]bool{}
	for _, r := range before {
		was[r] = true
	}
	for _, r := range after {
		is[r] = true
	}
	changed := false
	for r := range managed {
		changed = changed || was[r] != is[r]
	}
	if !changed {
		return nil
	}
	_, err := c.Enqueue(idpSyncJobMethod, map[string]any{"user": user},
		map[string]any{"queue": idpQueue, "backoff": "exponential", "maxAttempts": 5})
	return err
}

// IdPSync is the job: it pushes the User's mapped roles to PocketID as
// groups. A User with no PocketID account is nothing to do.
func (e *Engine) IdPSync(c *Ctx, user string) error {
	p := e.idpAdmin()
	if p == nil || len(p.policy.GroupRoles) == 0 {
		return nil
	}
	doc, err := c.GetDocIgnoringPerms("User", user)
	if err != nil {
		if ce := cerr.From(err); ce != nil && ce.Status == http.StatusNotFound {
			return nil // deleted since: the desk asks separately about that
		}
		return err
	}
	u, err := p.locate(c, user, db.Str(doc["email"]))
	if err != nil || u == nil {
		return err
	}
	return p.syncGroups(c.Ctx, u, docRoles(doc))
}

// SetIdPDisabled disables or re-enables the PocketID account of a User —
// or of an address, for a User already deleted. It is what the desk calls
// once the operator said yes; nothing does it on its own.
func (e *Engine) SetIdPDisabled(c *Ctx, user, email string, disabled bool) (map[string]any, error) {
	if !c.canAdministerUsers() {
		return nil, cerr.Permission("Only a System Manager can change an account at the identity provider")
	}
	p := e.idpAdmin()
	if p == nil {
		return nil, cerr.Validation("No identity provider is set up for provisioning")
	}
	if strings.EqualFold(user, "Admin") {
		return nil, cerr.Validation("Admin has no account at the identity provider to change")
	}
	if email == "" && user != "" {
		v, err := db.Select(c.Ctx, c.Q(), `SELECT email FROM tab_user WHERE id = $1`, user)
		if err != nil {
			return nil, err
		}
		if len(v) > 0 {
			email = db.Str(v[0]["email"])
		}
	}
	u, err := p.locate(c, user, email)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"provider": p.provider.Label, "found": u != nil, "disabled": disabled}
	if u == nil {
		return out, nil
	}
	if u["disabled"] != disabled {
		if _, err := p.updateUser(c.Ctx, u, func(b map[string]any) { b["disabled"] = disabled }); err != nil {
			return nil, err
		}
	}
	action := "account.idp_enable"
	if disabled {
		action = "account.idp_disable"
	}
	target := user
	if target == "" {
		target = email
	}
	if err := c.Audit(action, "User", target, map[string]any{"provider": p.provider.ID, "email": email}); err != nil {
		return nil, err
	}
	return out, nil
}

// IdPStatus tells the desk whether to offer the provider questions at all.
func (e *Engine) IdPStatus() map[string]any {
	p := e.idpAdmin()
	if p == nil {
		return nil
	}
	return map[string]any{"id": p.provider.ID, "label": p.provider.Label}
}

// ProbePocketID is the doctor's check: the key works (problems), and which
// mapped groups PocketID does not have yet (absent) — those are created the
// first time a User needs one. It never changes anything at PocketID.
func ProbePocketID(ctx context.Context, p config.OIDCProvider, pol config.SSOPolicy) (problems, absent []string) {
	a := &pocketIDAdmin{provider: p, policy: pol}
	if _, err := a.call(ctx, "list users", http.MethodGet, "/api/users?pagination[limit]=1", nil, nil); err != nil {
		return []string{fmt.Sprint(err)}, nil
	}
	names := make([]string, 0, len(pol.GroupRoles))
	for g := range pol.GroupRoles {
		names = append(names, g)
	}
	sort.Strings(names)
	for _, n := range names {
		id, err := a.findGroup(ctx, n)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprint(err))
		case id == "":
			absent = append(absent, n)
		}
	}
	return problems, absent
}
