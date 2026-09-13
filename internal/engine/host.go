package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/mail"
)

// HostCall is the single entry point for every ddcore.* call made from TS.
func (e *Engine) HostCall(rt *js.Runtime, op string, raw json.RawMessage) (any, error) {
	c, _ := rt.Ctx.(*Ctx)
	if c == nil {
		return nil, cerr.Internal("runtime without a context (op {0})", op)
	}
	var a struct {
		Doctype   string            `json:"doctype"`
		Name      json.RawMessage   `json:"name"`
		Fields    json.RawMessage   `json:"fields"`
		Field     string            `json:"field"`
		Args      json.RawMessage   `json:"args"`
		Filters   any               `json:"filters"`
		OrFilters any               `json:"orFilters"`
		Values    Doc               `json:"values"`
		Doc       Doc               `json:"doc"`
		Opts      map[string]any    `json:"opts"`
		Query     string            `json:"query"`
		Params    []any             `json:"params"`
		Key       string            `json:"key"`
		Value     any               `json:"value"`
		TTL       float64           `json:"ttl"`
		Text      string            `json:"text"`
		Lang      string            `json:"lang"`
		Message   string            `json:"message"`
		Method    string            `json:"method"`
		URL       string            `json:"url"`
		Body      any               `json:"body"`
		Headers   map[string]string `json:"headers"`
		Timeout   float64           `json:"timeout"`
		Level     string            `json:"level"`
		LogArgs   []string          `json:"args2"`
		Event     string            `json:"event"`
		Payload   any               `json:"payload"`
		User      string            `json:"user"`
		Ptype     string            `json:"ptype"`
		OldName   string            `json:"oldName"`
		NewName   string            `json:"newName"`
		Currency  string            `json:"currency"`
		To        []string          `json:"to"`
		Subject   string            `json:"subject"`
		HTML      string            `json:"html"`
		ExceptSid string            `json:"exceptSid"`
		Kind      string            `json:"kind"`
		Token     string            `json:"token"`
		Password  string            `json:"password"`
		Label     string            `json:"label"`
		Days      float64           `json:"days"`
		Limit     float64           `json:"limit"`
		Minutes   float64           `json:"minutes"`
		ID        string            `json:"id"`
		Delivery  string            `json:"delivery"`
		Status    string            `json:"status"`
		Error     string            `json:"error"`
		Prefix    string            `json:"prefix"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, cerr.Internal("invalid arguments in {0}: {1}", op, err)
	}
	nameStr := func() string {
		var s string
		json.Unmarshal(a.Name, &s)
		return s
	}
	switch op {
	case "session":
		roles, _ := c.Roles()
		// langs is the site's language list, next to the request's own language:
		// an app needs it to offer a choice, and the JS-side registry does not
		// carry User.language's options (they are injected into the Go one).
		return map[string]any{"user": c.User, "roles": roles, "lang": c.Lang, "langs": c.St.I18n.Langs(),
			"request": c.Request, "requestId": c.ReqID}, nil
	case "getRoles":
		u := a.User
		if u == "" {
			u = c.User
		}
		return c.RolesOf(u)
	case "nowdate":
		return c.Today(), nil
	case "now":
		// the site's wall clock, for the same reason nowdate is
		return time.Now().In(e.Location()).Format("2006-01-02 15:04:05"), nil
	case "translate":
		return c.T(a.Text), nil
	case "catalogue":
		// the whole catalogue for a language, so the prelude can interpolate
		// in JS instead of crossing the bridge for every string
		lang := a.Lang
		if lang == "" {
			lang = c.Lang
		}
		return c.St.I18n.Catalogue(lang), nil
	case "formatCurrency":
		return FormatCurrency(toFloat(a.Value), orDefault(a.Currency, e.Cfg.Currency), c.Lang, e.CurrencyPrecision()), nil
	case "site":
		// everything about the site the app runtime needs but must not cross
		// the bridge for repeatedly: it is immutable inside a State, so the
		// prelude mirrors it once per VM, as it already does the catalogue
		return map[string]any{
			"currency": e.Cfg.Currency, "currencyPrecision": e.CurrencyPrecision(),
			"rounding": e.Cfg.Rounding.String(), "timezone": e.Cfg.Timezone, "name": c.St.SiteTitle(),
		}, nil
	case "getMeta":
		d, err := c.St.DocType(a.Doctype)
		return d, err
	case "getDoc":
		return c.GetDoc(a.Doctype, nameStr())
	case "newDoc":
		return c.NewDoc(a.Doctype, a.Values)
	case "doc.insert":
		return c.Insert(a.Doc, saveOpts(a.Opts))
	case "doc.save":
		return c.Save(a.Doc, saveOpts(a.Opts))
	case "doc.cancel":
		return c.Cancel(a.Doc)
	case "doc.delete":
		return nil, c.Delete(a.Doctype, nameStr(), a.Opts["ignorePermissions"] == true, a.Opts["force"] == true)
	case "doc.dbSet":
		// returns the new modified timestamp for the prelude to synchronize the document (B21)
		modified, err := c.DBSet(a.Doctype, nameStr(), a.Values, true)
		if err != nil {
			return nil, err
		}
		return map[string]any{"modified": modified}, nil
	case "db.getValue":
		var fields []string
		var one string
		var name any
		if json.Unmarshal(a.Fields, &one) == nil {
			fields = []string{one}
		} else if err := json.Unmarshal(a.Fields, &fields); err != nil {
			return nil, cerr.Validation("getValue: invalid fields")
		}
		var s string
		if json.Unmarshal(a.Name, &s) == nil {
			name = s
		} else {
			json.Unmarshal(a.Name, &name)
		}
		m, err := c.GetValues(a.Doctype, name, fields)
		if err != nil {
			return nil, err
		}
		if one != "" {
			if m == nil {
				return nil, nil
			}
			key := one
			if i := strings.Index(strings.ToLower(one), " as "); i > 0 {
				key = strings.TrimSpace(one[i+4:])
			}
			return m[key], nil
		}
		return m, nil
	case "db.getList":
		var la ListArgs
		json.Unmarshal(a.Args, &la)
		return c.GetList(a.Doctype, la)
	case "db.setValue":
		return nil, c.SetValue(a.Doctype, nameStr(), a.Values)
	case "db.count":
		return c.Count(a.Doctype, a.Filters, a.OrFilters)
	case "db.exists":
		var s string
		if json.Unmarshal(a.Name, &s) == nil {
			ok, err := c.Exists(a.Doctype, s)
			if err != nil || !ok {
				return nil, err
			}
			return s, nil
		}
		var f any
		json.Unmarshal(a.Name, &f)
		n, err := c.ExistsWhere(a.Doctype, f)
		if err != nil || n == "" {
			return nil, err
		}
		return n, nil
	case "db.sql":
		return c.SQL(a.Query, a.Params)
	case "patchSQL":
		return c.PatchSQL(a.Query, a.Params)
	case "db.lock":
		return nil, c.Lock(a.Key)
	case "db.getSingleValue":
		return c.GetValue(a.Doctype, a.Doctype, a.Field)
	case "hasPermission":
		var doc Doc
		if len(a.Fields) == 0 {
			// doc may be passed under "doc"
		}
		if a.Doc != nil {
			doc = a.Doc
		}
		return c.HasPermission(a.Doctype, orDefault(a.Ptype, "read"), doc)
	case "msgprint":
		m := Message{Message: a.Message}
		if t, ok := a.Opts["title"].(string); ok {
			m.Title = t
		}
		if i, ok := a.Opts["indicator"].(string); ok {
			m.Indicator = i
		}
		m.Alert, _ = a.Opts["alert"].(bool)
		c.Msgprint(m)
		return nil, nil
	case "cache.get":
		v, ok := e.Cache.Get(a.Key)
		if !ok {
			return nil, nil
		}
		return v, nil
	case "cache.set":
		e.Cache.Set(a.Key, a.Value, time.Duration(a.TTL)*time.Second)
		return nil, nil
	case "cache.del":
		e.Cache.Del(a.Key)
		return nil, nil
	case "http":
		return httpCall(a.Method, a.URL, a.Body, a.Headers, a.Timeout)
	case "enqueue":
		var q struct {
			Method string         `json:"method"`
			Args   map[string]any `json:"args"`
			Opts   map[string]any `json:"opts"`
		}
		json.Unmarshal(raw, &q)
		return c.Enqueue(q.Method, q.Args, q.Opts)
	case "publish":
		ev := Event{Name: a.Event, Payload: a.Payload}
		if u, ok := a.Opts["user"].(string); ok {
			ev.User = u
		}
		c.AfterCommit(func() { e.Events.Publish(ev) })
		return nil, nil
	case "log":
		var l struct {
			Level string   `json:"level"`
			Args  []string `json:"args"`
		}
		json.Unmarshal(raw, &l)
		msg := strings.Join(l.Args, " ")
		switch l.Level {
		case "error":
			e.Log.Error(msg, "user", c.User)
		case "warn":
			e.Log.Warn(msg, "user", c.User)
		case "debug":
			e.Log.Debug(msg, "user", c.User)
		default:
			e.Log.Info(msg, "user", c.User)
		}
		if c.Flags["captureLogs"] == true {
			c.Flags["logs"] = append(c.Flags["logs"].([]string), l.Level+": "+msg)
		}
		return nil, nil
	case "rename":
		return c.Rename(a.Doctype, a.OldName, a.NewName)
	case "hashPassword":
		// Routed through the policy so that the User form, `ddcore user add`
		// and any app that writes new_password all meet the same minimum. The
		// returned *cerr.Error surfaces in TS as a throw, like ddcore.throw.
		return e.HashNewPassword(a.User, a.Text)
	// ---- self-service auth -------------------------------------------------
	//
	// These exist because ddcore.db.sql is read-only and a self-service write
	// has nowhere else to go: no TS can touch ddcore_session or
	// ddcore_auth_token. They take the user explicitly and the callers in
	// core/services check that it is the session's own.
	case "auth.sessions":
		rows, err := db.Select(c.Ctx, e.DB.Pool,
			`SELECT sid, ip, user_agent, created, last_seen, expires FROM ddcore_session
			 WHERE "user" = $1 AND expires > now() ORDER BY last_seen DESC`, a.User)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			sid := db.Str(r["sid"])
			// The raw sid never leaves: it is a bearer token, and one XSS
			// reading this list would otherwise own every device the person
			// is signed in on. The handle is enough to revoke by and useless
			// to replay.
			out = append(out, map[string]any{
				"id": TokenHandle(sid), "current": sid == c.Sid,
				"ip": r["ip"], "userAgent": r["user_agent"],
				"created": r["created"], "lastSeen": r["last_seen"], "expires": r["expires"],
			})
		}
		return out, nil
	case "auth.revokeSessions":
		if a.ID != "" {
			// Revoking one, addressed by handle. Scoped to the user, so a
			// handle guessed from someone else's list reaches nothing.
			rows, err := db.Select(c.Ctx, e.DB.Pool,
				`SELECT sid FROM ddcore_session WHERE "user" = $1`, a.User)
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				sid := db.Str(r["sid"])
				if TokenHandle(sid) != a.ID {
					continue
				}
				e.Cache.Del("sid:" + sid)
				tag, err := e.DB.Pool.Exec(c.Ctx, `DELETE FROM ddcore_session WHERE sid = $1`, sid)
				if err != nil {
					return nil, err
				}
				return map[string]any{"revoked": int(tag.RowsAffected())}, nil
			}
			return map[string]any{"revoked": 0}, nil
		}
		n, err := e.DropSessions(c.Ctx, e.DB.Pool, a.User, a.ExceptSid)
		if err != nil {
			return nil, err
		}
		return map[string]any{"revoked": n}, nil
	case "auth.currentSid":
		return c.Sid, nil
	case "auth.checkPassword":
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT password_hash FROM tab_user WHERE name = $1`, a.User)
		if err != nil || len(rows) == 0 {
			return false, err
		}
		return CheckPassword(db.Str(rows[0]["password_hash"]), a.Password), nil
	case "auth.setPassword":
		return nil, e.SetPasswordExcept(c.Ctx, a.User, a.Password, a.ExceptSid)
	case "auth.startRecovery":
		rec, err := e.StartRecovery(c, a.User, orDefault(a.Kind, TokenReset), db.Str(c.Request["ip"]))
		if err != nil {
			return nil, err
		}
		return map[string]any{"link": rec.Link, "expires": rec.Expires, "delivered": rec.Delivered}, nil
	case "auth.throttle":
		limit := int(a.Limit)
		if limit <= 0 {
			limit = e.Cfg.Auth.MaxLoginAttempts
		}
		window := time.Duration(a.Minutes) * time.Minute
		if window <= 0 {
			window = e.Cfg.Auth.LockoutWindow()
		}
		if err := e.CheckThrottle(c.Ctx, a.Key, limit, window); err != nil {
			return nil, err
		}
		// Written on the pool, outside this request's transaction: an attempt
		// recorded on c.Tx would be rolled back by the error it counts.
		e.RecordAttempt(c.Ctx, a.Key, db.Str(c.Request["ip"]), false)
		return nil, nil
	case "auth.clearAttempts":
		n, err := e.ClearAttempts(c.Ctx, a.Key)
		return n, err
	case "auth.createAPIKey":
		return e.CreateAPIKeyFor(c, a.User, a.Label, int(a.Days))
	case "auth.apiKeys":
		// Not db.getList: that checks the doctype's permissions, and API Key
		// is System Manager only — deliberately, since read there would be
		// read on everyone's keys. Scoped to the one user instead.
		return db.Select(c.Ctx, c.Q(),
			`SELECT name, label, enabled, creation, last_used, expires FROM tab_api_key
			 WHERE "user" = $1 ORDER BY creation DESC LIMIT 100`, a.User)
	case "auth.revokeAPIKey":
		// The owner is part of the WHERE, so a name from someone else's list
		// deletes nothing rather than deleting theirs.
		tag, err := c.Q().Exec(c.Ctx, `DELETE FROM tab_api_key WHERE name = $1 AND "user" = $2`, nameStr(), a.User)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, cerr.NotFound("That key is not yours")
		}
		e.Cache.Del("apikey:" + nameStr())
		return map[string]any{"ok": true}, nil
	case "mail.prepare":
		// The language the message will be written in, decided before the
		// prelude renders anything: a reader gets their own language, not the
		// language of whoever pressed the button.
		return map[string]any{"lang": c.RecipientLang(a.To)}, nil
	case "mail.queue":
		var r MailRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, cerr.Internal("invalid arguments in {0}: {1}", op, err)
		}
		return c.QueueMail(r)
	case "mail.load":
		return e.LoadMail(c, a.Delivery)
	case "mail.deliver":
		var d struct {
			Blocks []mail.Block `json:"blocks"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, cerr.Internal("invalid arguments in {0}: {1}", op, err)
		}
		return e.DeliverMail(c, a.Delivery, a.Subject, d.Blocks)
	case "mail.result":
		return nil, e.RecordMail(c, a.Delivery, a.Status, a.Error)
	case "webhook.emit":
		var r struct {
			Event     string            `json:"event"`
			Data      any               `json:"data"`
			Key       string            `json:"key"`
			Reference *WebhookReference `json:"reference"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, cerr.Internal("invalid arguments in {0}: {1}", op, err)
		}
		names, err := c.EmitWebhook(r.Event, r.Data, r.Reference, r.Key)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deliveries": names}, nil
	case "webhook.validate":
		return nil, c.ValidateWebhook(a.Doc)
	case "webhook.deliver":
		return nil, e.DeliverWebhook(c, a.Delivery)
	case "webhook.replay":
		if err := c.ReplayWebhook(a.Delivery); err != nil {
			return nil, err
		}
		return map[string]any{"delivery": a.Delivery, "status": WebhookQueued}, nil
	case "webhook.sweep":
		n, err := e.SweepWebhookDeliveries(c.Ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deliveries": n}, nil
	case "jobSweep":
		n, err := e.SweepJobs(c.Ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"done": n.Done, "failed": n.Failed}, nil
	case "authSweep":
		n, err := e.SweepAuth(c.Ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sessions": n.Sessions, "tokens": n.Tokens, "attempts": n.Attempts}, nil
	case "secret":
		// An integration credential is read from the environment, never from a
		// column: that is what keeps it out of every backup, export and
		// Version diff by construction rather than by remembering to.
		v, ok := e.Secret(a.Text)
		if !ok {
			return nil, nil
		}
		return v, nil
	case "vault.set":
		valStr := ""
		if a.Value != nil {
			valStr = fmt.Sprint(a.Value)
		}
		return nil, e.VaultSet(c, a.Key, valStr)
	case "vault.get":
		v, ok, err := e.VaultGet(c, a.Key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return map[string]any{"value": v}, nil
	case "vault.del":
		return nil, e.VaultDel(c, a.Key)
	case "vault.list":
		return e.VaultList(c, a.Prefix)
	case "dropSessions":
		_, err := e.DropSessions(c.Ctx, c.Q(), a.User, "")
		return nil, err
	case "test.begin":
		return nil, c.Begin()
	case "test.rollback":
		return nil, c.RollbackTo()
	}
	return nil, cerr.Internal("unknown bridge operation: {0}", op)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func saveOpts(o map[string]any) SaveOpts {
	var s SaveOpts
	if o == nil {
		return s
	}
	s.IgnorePermissions, _ = o["ignorePermissions"].(bool)
	s.IgnoreVersion, _ = o["ignoreVersion"].(bool)
	s.IgnoreLinks, _ = o["ignoreLinks"].(bool)
	return s
}

// FormatCurrency renders a number as money.
//
// The grouping and the decimal mark come from `lang` and the symbol from
// `currency`, because they are two independent facts: a Brazilian reading a
// site in English still wants "R$" if the site's currency is BRL, and an
// American reading it in Portuguese still wants "$" if it is USD. Choosing the
// language tag *from the currency*, as this did, conflated them and got both
// wrong for every mixed case.
func FormatCurrency(v float64, code, lang string, precision int) string {
	tag, err := language.Parse(lang)
	if err != nil {
		tag = language.English
	}
	u, err := currency.ParseISO(code)
	if err != nil {
		// an unknown code prints as itself: "XYZ 1,234.50" is honest, and
		// borrowing another currency's symbol would not be
		return code + " " + message.NewPrinter(tag).Sprintf("%.*f", precision, v)
	}
	return message.NewPrinter(tag).Sprint(currency.Symbol(u.Amount(v)))
}

func httpCall(method, url string, body any, headers map[string]string, timeout float64) (any, error) {
	if timeout <= 0 {
		timeout = 15
	}
	var rd io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			rd = strings.NewReader(s)
		} else {
			b, _ := json.Marshal(body)
			rd = strings.NewReader(string(b))
			if headers == nil {
				headers = map[string]string{}
			}
			if _, ok := headers["Content-Type"]; !ok {
				headers["Content-Type"] = "application/json"
			}
		}
	}
	req, err := http.NewRequest(orDefault(method, "GET"), url, rd)
	if err != nil {
		return nil, cerr.Validation("http: {0}", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "ddcore/0.1")
	client := &http.Client{Timeout: time.Duration(timeout * float64(time.Second))}
	res, err := client.Do(req)
	if err != nil {
		return nil, cerr.Validation("http: {0}", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	return map[string]any{"status": res.StatusCode, "body": string(b), "headers": flatHeaders(res.Header)}, nil
}

func flatHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

func (e *Engine) String() string {
	return fmt.Sprintf("ddcore engine (%d doctypes)", len(e.Current().Meta.DocTypes))
}

var _ = db.Str
