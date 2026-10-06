package engine

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
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
		Doctype       string            `json:"doctype"`
		ID            json.RawMessage   `json:"id"`
		Fields        json.RawMessage   `json:"fields"`
		Field         string            `json:"field"`
		Args          json.RawMessage   `json:"args"`
		Filters       any               `json:"filters"`
		OrFilters     any               `json:"orFilters"`
		Values        Doc               `json:"values"`
		Doc           Doc               `json:"doc"`
		Opts          map[string]any    `json:"opts"`
		Flags         map[string]any    `json:"flags"`
		Query         string            `json:"query"`
		Params        []any             `json:"params"`
		Key           string            `json:"key"`
		Value         any               `json:"value"`
		TTL           float64           `json:"ttl"`
		Text          string            `json:"text"`
		Lang          string            `json:"lang"`
		Message       string            `json:"message"`
		Method        string            `json:"method"`
		URL           string            `json:"url"`
		Body          any               `json:"body"`
		BodyEncoding  string            `json:"bodyEncoding"`
		Headers       map[string]string `json:"headers"`
		Timeout       float64           `json:"timeout"`
		ResponseType  string            `json:"responseType"`
		MaxBytes      float64           `json:"maxBytes"`
		MaxRedirects  *float64          `json:"maxRedirects"`
		ClientCert    *httpClientCert   `json:"clientCert"`
		Level         string            `json:"level"`
		Event         string            `json:"event"`
		Payload       any               `json:"payload"`
		User          string            `json:"user"`
		Ptype         string            `json:"ptype"`
		OldID         string            `json:"oldID"`
		NewID         string            `json:"newID"`
		Currency      string            `json:"currency"`
		To            []string          `json:"to"`
		Subject       string            `json:"subject"`
		HTML          string            `json:"html"`
		ExceptSid     string            `json:"exceptSid"`
		Kind          string            `json:"kind"`
		Token         string            `json:"token"`
		Password      string            `json:"password"`
		Label         string            `json:"label"`
		Days          float64           `json:"days"`
		Limit         float64           `json:"limit"`
		Minutes       float64           `json:"minutes"`
		Delivery      string            `json:"delivery"`
		Status        string            `json:"status"`
		Error         string            `json:"error"`
		Prefix        string            `json:"prefix"`
		Action        string            `json:"action"`
		TargetDoctype string            `json:"targetDoctype"`
		TargetID      string            `json:"targetID"`
		Detail        any               `json:"detail"`
		Email         string            `json:"email"`
		FullName      string            `json:"fullName"`
		Roles         []string          `json:"roles"`
		UserType      string            `json:"userType"`
		Before        []string          `json:"before"`
		Disabled      bool              `json:"disabled"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, cerr.Internal("invalid arguments in {0}: {1}", op, err)
	}
	idStr := func() string {
		var s string
		json.Unmarshal(a.ID, &s)
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
			"url": strings.TrimSuffix(e.Cfg.SiteURL, "/"),
		}, nil
	case "getMeta":
		d, err := c.St.DocType(a.Doctype)
		return d, err
	case "getDoc":
		return c.GetDocOpts(a.Doctype, idStr(), GetOpts{IgnorePermissions: a.Opts["ignorePermissions"] == true})
	case "newDoc":
		return c.NewDoc(a.Doctype, a.Values)
	case "doc.insert", "doc.save", "doc.cancel":
		// the document's flags go with the write and come back with what the
		// hooks left in them
		opts := saveOpts(a.Opts)
		if opts.Flags = a.Flags; opts.Flags == nil {
			opts.Flags = map[string]any{}
		}
		write := c.Save
		switch op {
		case "doc.insert":
			write = c.Insert
		case "doc.cancel":
			write = c.cancel
		}
		out, err := write(a.Doc, opts)
		if err != nil {
			return nil, err
		}
		return map[string]any{"doc": out, "flags": opts.Flags}, nil
	case "doc.applyWorkflow":
		// the same transition POST /api/workflow/apply runs: role, self-approval,
		// condition, row lock, audit and timeline comment
		return c.ApplyWorkflowTransition(a.Doctype, idStr(), a.Action)
	case "doc.delete":
		return nil, c.deleteWithFlags(a.Doctype, idStr(), a.Opts["ignorePermissions"] == true, a.Opts["force"] == true, a.Flags)
	case "doc.dbSet":
		// returns the new modified timestamp for the prelude to synchronize the document (B21)
		modified, err := c.DBSet(a.Doctype, idStr(), a.Values, true)
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
		if json.Unmarshal(a.ID, &s) == nil {
			name = s
		} else {
			json.Unmarshal(a.ID, &name)
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
		return nil, c.SetValue(a.Doctype, idStr(), a.Values)
	case "db.count":
		return c.Count(a.Doctype, a.Filters, a.OrFilters)
	case "db.exists":
		var s string
		if json.Unmarshal(a.ID, &s) == nil {
			ok, err := c.Exists(a.Doctype, s)
			if err != nil || !ok {
				return nil, err
			}
			return s, nil
		}
		var f any
		json.Unmarshal(a.ID, &f)
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
	case "db.savepoint.begin":
		return nil, c.SavepointBegin()
	case "db.savepoint.release":
		return nil, c.SavepointRelease()
	case "db.savepoint.rollback":
		return nil, c.SavepointRollback()
	case "db.getSingleValue":
		return c.GetSingleValue(a.Doctype, a.Field)
	case "hasPermission":
		doc := a.Doc
		if doc == nil && len(a.ID) > 0 {
			// a document id: the rules read the stored document, which the
			// caller may not be allowed to load itself
			var id string
			if err := json.Unmarshal(a.ID, &id); err != nil {
				return nil, cerr.Internal("invalid arguments in {0}: {1}", op, err)
			}
			if err := c.WithIgnorePermissions(func() error {
				var err error
				doc, err = c.GetDoc(a.Doctype, id)
				return err
			}); err != nil {
				return nil, err
			}
		}
		ptype := orDefault(a.Ptype, "read")
		if a.User == "" || a.User == c.User {
			return c.HasPermission(a.Doctype, ptype, doc)
		}
		var ok bool
		err := c.withUser(a.User, func(u *Ctx) error {
			var err error
			ok, err = u.HasPermission(a.Doctype, ptype, doc)
			return err
		})
		return ok, err
	case "redact":
		// the border an app's own endpoint or report crosses: what an API read
		// of this document would show the current user (SEC-02)
		if a.Doc == nil {
			return nil, nil
		}
		return c.RedactDoc(a.Doctype, a.Doc.Clone()), nil
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
		v, ok := e.Cache.Get(c.appCacheKey(a.Key))
		if !ok {
			return nil, nil
		}
		return v, nil
	case "cache.set":
		e.Cache.Set(c.appCacheKey(a.Key), a.Value, time.Duration(a.TTL)*time.Second)
		return nil, nil
	case "cache.del":
		// Dropped here now, and in the other processes when the transaction
		// commits: the core controllers drop roles and user types this way.
		// Those are the engine's own keys, which carry no tenant (a user is
		// in one), so inside a tenant both spellings of the key go.
		keys := []string{a.Key}
		if k := c.appCacheKey(a.Key); k != a.Key {
			keys = append(keys, k)
		}
		for _, k := range keys {
			e.Cache.Del(k)
		}
		return nil, c.broadcastInvalidation(keys, nil)
	case "http":
		return httpCall(c.callCtx(), a.Method, a.URL, a.Body, a.BodyEncoding, a.Headers, a.Timeout, a.ResponseType, int64(a.MaxBytes), a.MaxRedirects, a.ClientCert)
	case "files.save":
		var f SaveFileArgs
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, cerr.Validation("files.save: {0}", err)
		}
		return c.SaveFile(f)
	case "files.presign":
		return c.PresignFile(a.URL, time.Duration(a.TTL*float64(time.Second)), a.Opts["ignorePermissions"] == true)
	case "externalDb.sql":
		return c.ExternalSQL(a.Key, a.Query, a.Params, a.Timeout)
	case "enqueue":
		var q struct {
			Method string         `json:"method"`
			Args   map[string]any `json:"args"`
			Opts   map[string]any `json:"opts"`
		}
		json.Unmarshal(raw, &q)
		return c.Enqueue(q.Method, q.Args, q.Opts)
	case "publish":
		if !validEventName(a.Event) {
			return nil, cerr.Validation("publish: {0} is not a valid event name (letters, digits, _ . : -)", a.Event)
		}
		ev := Event{Name: a.Event, Payload: a.Payload}
		if u, ok := a.Opts["user"].(string); ok {
			ev.User = u
		}
		// an event about a document reaches only the sessions that may read
		// it, as doc_update does; a doctype alone asks read on the DocType
		if dt, ok := a.Opts["doctype"].(string); ok && dt != "" {
			ev.Doctype = dt
			if id, ok := a.Opts["id"].(string); ok {
				ev.DocID = id
			}
		}
		c.AfterCommit(func() { c.publish(ev) })
		return nil, nil
	case "log":
		var l struct {
			Level string         `json:"level"`
			Msg   string         `json:"msg"`
			Attrs map[string]any `json:"attrs"`
		}
		json.Unmarshal(raw, &l)
		level := slog.LevelInfo
		switch l.Level {
		case "error":
			level = slog.LevelError
		case "warn":
			level = slog.LevelWarn
		case "debug":
			level = slog.LevelDebug
		}
		e.Log.Log(context.Background(), level, l.Msg, logAttrs(c.User, l.Attrs)...)
		// captured on the ctx that owns the transaction, where Eval reads them:
		// a log line written under ddcore.runAs belongs to the same output
		if o := c.owner(); o.Flags["captureLogs"] == true {
			line := l.Level + ":"
			if l.Msg != "" {
				line += " " + l.Msg
			}
			if len(l.Attrs) > 0 {
				b, _ := json.Marshal(l.Attrs)
				line += " " + string(b)
			}
			o.Flags["logs"] = append(o.Flags["logs"].([]string), line)
		}
		return nil, nil
	case "rename":
		return c.Rename(a.Doctype, a.OldID, a.NewID)
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
		if handle := idStr(); handle != "" {
			// Revoking one, addressed by handle. Scoped to the user, so a
			// handle guessed from someone else's list reaches nothing.
			rows, err := db.Select(c.Ctx, e.DB.Pool,
				`SELECT sid FROM ddcore_session WHERE "user" = $1`, a.User)
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				sid := db.Str(r["sid"])
				if TokenHandle(sid) != handle {
					continue
				}
				e.Cache.Del("sid:" + sid)
				tag, err := e.DB.Pool.Exec(c.Ctx, `DELETE FROM ddcore_session WHERE sid = $1`, sid)
				if err != nil {
					return nil, err
				}
				if err := broadcastInvalidation(c.Ctx, e.DB.Pool, []string{"sid:" + sid}, nil); err != nil {
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
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT password_hash FROM tab_user WHERE id = $1`, a.User)
		if err != nil || len(rows) == 0 {
			return false, err
		}
		return CheckPassword(db.Str(rows[0]["password_hash"]), a.Password), nil
	case "auth.setPassword":
		return nil, e.SetPasswordExcept(c.Ctx, a.User, a.Password, a.ExceptSid)
	case "users.invite":
		return e.InviteUser(c, Invitation{Email: a.Email, FullName: a.FullName, Roles: a.Roles, UserType: a.UserType})
	case "users.resendInvite":
		return e.ResendInvite(c, a.User)
	case "users.createApiKey":
		return e.CreateUserAPIKey(c, a.User, a.Label, int(a.Days))
	case "idp.queueSync":
		return nil, e.QueueIdPSync(c, a.User, a.Before, a.Roles)
	case "idp.sync":
		return nil, e.IdPSync(c, a.User)
	case "idp.setDisabled":
		return e.SetIdPDisabled(c, a.User, a.Email, a.Disabled)
	case "idp.status":
		return e.IdPStatus(), nil
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
			`SELECT id, label, enabled, creation, last_used, expires FROM tab_api_key
			 WHERE "user" = $1 ORDER BY creation DESC LIMIT 100`, a.User)
	case "auth.revokeAPIKey":
		// The owner is part of the WHERE, so a name from someone else's list
		// deletes nothing rather than deleting theirs.
		tag, err := c.Q().Exec(c.Ctx, `DELETE FROM tab_api_key WHERE id = $1 AND "user" = $2`, idStr(), a.User)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, cerr.NotFound("That key is not yours")
		}
		e.Cache.Del("apikey:" + idStr())
		if err := c.broadcastInvalidation([]string{"apikey:" + idStr()}, nil); err != nil {
			return nil, err
		}
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
	case "auditSweep":
		n, err := e.SweepAuditEvents(c.Ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"events": n}, nil
	case "secret":
		// An integration credential is read from the environment, never from a
		// column: that is what keeps it out of every backup, export and
		// Version diff by construction rather than by remembering to.
		v, ok := e.Secret(a.Text)
		if !ok {
			return nil, nil
		}
		return v, nil
	case "crypto.hmacSha256":
		// Key and Text are the secret and the message; the answer is hex unless
		// opts.output says otherwise, the form a webhook provider puts in its
		// signature header.
		return hmacSha256Op(a.Key, a.Text, a.Opts)
	case "crypto.sha256":
		return sha256Op(a.Text, a.Opts)
	case "crypto.randomToken":
		return randomTokenOp(a.Opts)
	case "crypto.randomInt":
		return randomIntOp(a.Opts)
	case "crypto.randomString":
		return randomStringOp(a.Opts)
	case "webhooks.verify":
		// Never throws on a bad request: a receiver wants a boolean to refuse on.
		tol := 300.0
		if f, ok := a.Opts["toleranceSeconds"].(float64); ok {
			tol = f
		}
		return VerifyWebhook(a.Key, a.Headers, []byte(a.Text), tol, time.Now()), nil
	case "crypto.pfxInfo":
		if a.ClientCert == nil {
			return nil, cerr.Validation("crypto: pfx is not valid base64")
		}
		return pfxInfo(a.ClientCert.Pfx, a.ClientCert.Password)
	case "crypto.certInfo":
		if a.ClientCert == nil {
			return nil, cerr.Validation("crypto: cert is not a valid PEM certificate")
		}
		return pemCertInfo(a.ClientCert.Cert)
	case "push.send":
		var p pushSendArgs
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, cerr.Validation("push.send: {0}", err)
		}
		return e.PushSend(c.callCtx(), p)
	case "push.publicKey":
		// what a page subscribes with, unpadded as applicationServerKey takes
		// it; null until the site has a VAPID pair
		if v, ok := e.Secret("vapid_public_key"); ok {
			return strings.TrimRight(strings.TrimSpace(v), "="), nil
		}
		return nil, nil
	case "crypto.timingSafeEqual":
		// Compared here, not in JS: `===` stops at the first differing byte,
		// which is what a signature check must not do.
		return subtle.ConstantTimeCompare([]byte(a.Key), []byte(a.Text)) == 1, nil
	case "vault.set":
		valStr := ""
		if a.Value != nil {
			valStr = fmt.Sprint(a.Value)
		}
		if err := c.sharedVaultWrite(a.Opts); err != nil {
			return nil, err
		}
		return nil, e.VaultSet(c, a.Key, valStr)
	case "vault.get":
		get := e.VaultGet
		if a.Opts["shared"] == true {
			get = e.VaultGetShared
		}
		v, ok, err := get(c, a.Key)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return map[string]any{"value": v}, nil
	case "vault.del":
		if err := c.sharedVaultWrite(a.Opts); err != nil {
			return nil, err
		}
		return nil, e.VaultDel(c, a.Key)
	case "vault.list":
		return e.VaultList(c, a.Prefix)
	case "dropSessions":
		_, err := e.DropSessions(c.Ctx, c.Q(), a.User, "")
		return nil, err
	case "share.add", "share.remove", "share.list":
		var sa struct {
			Doctype string      `json:"doctype"`
			ID      string      `json:"id"`
			User    string      `json:"user"`
			Rights  ShareRights `json:"rights"`
		}
		// Strictly, as the endpoint decodes it: `override_scope` is the column's
		// name and `overrideScope` the argument's, so the near miss is the one
		// mistake to expect — and ignoring it would hand back a share that
		// silently grants less than the caller asked for.
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&sa); err != nil {
			return nil, cerr.Validation("Invalid arguments: {0}", err)
		}
		switch op {
		case "share.add":
			return c.ShareDoc(sa.Doctype, sa.ID, sa.User, sa.Rights)
		case "share.remove":
			return nil, c.UnshareDoc(sa.Doctype, sa.ID, sa.User)
		}
		return c.ListDocShares(sa.Doctype, sa.ID)
	case "audit":
		var d map[string]any
		if m, ok := a.Detail.(map[string]any); ok {
			d = m
		}
		return nil, c.Audit(a.Action, a.TargetDoctype, a.TargetID, d)
	case "auditDenied":
		var d map[string]any
		if m, ok := a.Detail.(map[string]any); ok {
			d = m
		}
		c.AuditDenied(a.Action, a.TargetDoctype, a.TargetID, d)
		return nil, nil
	case "test.begin":
		if !c.E.Cfg.Test {
			return nil, cerr.Permission("ddcore.test is only available inside ddcore test")
		}
		return nil, c.Begin()
	case "test.rollback":
		if !c.E.Cfg.Test {
			return nil, cerr.Permission("ddcore.test is only available inside ddcore test")
		}
		return nil, c.testRootCtx().RollbackTo()
	// ---- tenancy ------------------------------------------------------------
	case "tenant.current":
		return c.Tenant, nil
	case "tenant.list":
		return c.TenantList()
	case "tenant.enter":
		if !c.Tenancy() {
			return nil, cerr.Validation("This site has no tenants: tenancy is off")
		}
		if c.Tenant != "" && c.Tenant != a.Key {
			return nil, cerr.Permission("A tenant cannot enter another tenant")
		}
		// entering the tenant the ctx is already in still nests, so that the
		// leave that follows has something to undo
		_, err := c.enterTenantCtx(a.Key)
		return nil, err
	case "tenant.leave":
		if _, err := c.leaveTenantCtx(); err != nil {
			c.E.Log.Warn("could not return from a tenant", "err", err)
		}
		return nil, nil
	case "tenant.created":
		if c.Tenant != "" {
			return nil, cerr.Permission("Tenants are managed from the platform space")
		}
		return nil, c.TenantCreated(a.Key)
	case "test.asUser":
		return nil, c.testAsUser(a.User)
	case "test.restoreUser":
		c.restoreUser()
		return nil, nil
	case "runAs.enter":
		return nil, c.runAsEnter(a.User)
	case "runAs.exit":
		c.restoreUser()
		return nil, nil
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
// FormatDuration writes whole seconds the way a person reads them: "1d 2h 30m".
// A hidden unit is not dropped, it is folded into the next one — with days
// hidden, a day and a half is "36h" and not "12h".
func FormatDuration(secs int64, hideDays, hideSeconds bool) string {
	if secs < 0 {
		secs = 0
	}
	if hideSeconds {
		secs -= secs % 60
	}
	var parts []string
	if !hideDays {
		if d := secs / 86400; d > 0 {
			parts = append(parts, fmt.Sprintf("%dd", d))
		}
		secs %= 86400
	}
	if h := secs / 3600; h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	secs %= 3600
	if m := secs / 60; m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s := secs % 60; s > 0 && !hideSeconds {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	if len(parts) == 0 {
		if hideSeconds {
			return "0m"
		}
		return "0s"
	}
	return strings.Join(parts, " ")
}

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

// httpMaxBytes is the response size ddcore.http reads when the call names
// no maxBytes.
const httpMaxBytes = 10 << 20

// httpMaxRedirects is how many redirects ddcore.http follows when the call
// names no maxRedirects: the limit net/http applies by default.
const httpMaxRedirects = 10

func httpCall(ctx context.Context, method, url string, body any, bodyEncoding string, headers map[string]string, timeout float64, responseType string, maxBytes int64, maxRedirects *float64, cert *httpClientCert) (any, error) {
	if responseType != "" && responseType != "text" && responseType != "base64" {
		return nil, cerr.Validation("http: responseType must be \"text\" or \"base64\", not {0}", responseType)
	}
	hops := httpMaxRedirects
	if maxRedirects != nil {
		if m := *maxRedirects; m < 0 || m != float64(int(m)) {
			return nil, cerr.Validation("http: maxRedirects must be a whole number from 0, not {0}", m)
		}
		hops = int(*maxRedirects)
	}
	res, b, err := httpFetch(ctx, method, url, body, bodyEncoding, headers, timeout, maxBytes, hops, cert)
	if err != nil {
		return nil, err
	}
	out := string(b)
	if responseType == "base64" {
		out = base64.StdEncoding.EncodeToString(b)
	}
	return map[string]any{"status": res.StatusCode, "body": out, "headers": flatHeaders(res.Header)}, nil
}

// httpBody encodes a request body: a string is sent as is, anything else as
// JSON, a base64 string as the bytes it stands for (bodyEncoding "base64") and
// an array of parts as multipart/form-data (bodyEncoding "multipart"). The
// content type it returns, when not empty, is one the caller must send: the
// multipart boundary always, the JSON and octet-stream types only when the
// headers name no Content-Type of their own (the comparison ignores case).
func httpBody(body any, bodyEncoding string, headers map[string]string) (io.Reader, string, error) {
	hasType := false
	for k := range headers {
		if strings.EqualFold(k, "Content-Type") {
			hasType = true
		}
	}
	switch bodyEncoding {
	case "":
		if body == nil {
			return nil, "", nil
		}
		if s, ok := body.(string); ok {
			return strings.NewReader(s), "", nil
		}
		b, _ := json.Marshal(body)
		if hasType {
			return bytes.NewReader(b), "", nil
		}
		return bytes.NewReader(b), "application/json", nil
	case "base64":
		s, ok := body.(string)
		if !ok {
			return nil, "", cerr.Validation("http: a body with bodyEncoding \"base64\" must be a base64 string")
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, "", cerr.Validation("http: the body is not valid base64: {0}", err)
		}
		if hasType {
			return bytes.NewReader(b), "", nil
		}
		return bytes.NewReader(b), "application/octet-stream", nil
	case "multipart":
		parts, ok := body.([]any)
		if !ok {
			return nil, "", cerr.Validation("http: a body with bodyEncoding \"multipart\" must be an array of parts")
		}
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for i, p := range parts {
			m, _ := p.(map[string]any)
			name, _ := m["name"].(string)
			if name == "" {
				return nil, "", cerr.Validation("http: multipart part {0} has no name", i)
			}
			value, hasValue := m["value"]
			b64, hasB64 := m["base64"]
			if hasValue && hasB64 {
				return nil, "", cerr.Validation("http: multipart part {0} has both value and base64", name)
			}
			if !hasValue && !hasB64 {
				return nil, "", cerr.Validation("http: multipart part {0} needs a value or base64", name)
			}
			if hasValue {
				s, ok := value.(string)
				if !ok {
					return nil, "", cerr.Validation("http: the value of multipart part {0} must be a string", name)
				}
				if err := w.WriteField(name, s); err != nil {
					return nil, "", cerr.Validation("http: {0}", err)
				}
				continue
			}
			s, ok := b64.(string)
			if !ok {
				return nil, "", cerr.Validation("http: the base64 of multipart part {0} must be a string", name)
			}
			data, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return nil, "", cerr.Validation("http: multipart part {0} is not valid base64: {1}", name, err)
			}
			filename, _ := m["filename"].(string)
			if filename == "" {
				// a part without a file name reads as a plain field to most servers
				filename = name
			}
			ctype, _ := m["contentType"].(string)
			if ctype == "" {
				ctype = "application/octet-stream"
			}
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, multipartQuote(name), multipartQuote(filename)))
			h.Set("Content-Type", ctype)
			pw, err := w.CreatePart(h)
			if err != nil {
				return nil, "", cerr.Validation("http: {0}", err)
			}
			if _, err := pw.Write(data); err != nil {
				return nil, "", cerr.Validation("http: {0}", err)
			}
		}
		if err := w.Close(); err != nil {
			return nil, "", cerr.Validation("http: {0}", err)
		}
		return &buf, w.FormDataContentType(), nil
	}
	return nil, "", cerr.Validation("http: bodyEncoding must be \"base64\" or \"multipart\", not {0}", bodyEncoding)
}

var multipartQuoter = strings.NewReplacer("\\", "\\\\", `"`, "\\\"", "\r", "%0D", "\n", "%0A")

// multipartQuote escapes a name or file name for a Content-Disposition
// parameter, the way mime/multipart does for CreateFormFile.
func multipartQuote(s string) string { return multipartQuoter.Replace(s) }

// httpFetch sends one request and reads the whole response body, at most
// maxBytes of it (httpMaxBytes when zero). The response's body is closed.
// A cert is presented to a server that asks for one (mutual TLS).
//
// It follows at most maxRedirects redirects; past that, the 3xx itself is the
// response. See redirectPolicy for the headers a redirect keeps. ctx cancels
// the request, the body's read included: in a job it is the job's own.
func httpFetch(ctx context.Context, method, url string, body any, bodyEncoding string, headers map[string]string, timeout float64, maxBytes int64, maxRedirects int, cert *httpClientCert) (*http.Response, []byte, error) {
	if timeout <= 0 {
		timeout = 15
	}
	if maxBytes <= 0 {
		maxBytes = httpMaxBytes
	}
	rd, contentType, err := httpBody(body, bodyEncoding, headers)
	if err != nil {
		return nil, nil, err
	}
	if contentType != "" {
		hs := make(map[string]string, len(headers)+1)
		for k, v := range headers {
			if !strings.EqualFold(k, "Content-Type") {
				hs[k] = v
			}
		}
		hs["Content-Type"] = contentType
		headers = hs
	}
	req, err := http.NewRequestWithContext(ctx, orDefault(method, "GET"), url, rd)
	if err != nil {
		return nil, nil, cerr.Validation("http: {0}", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "ddcore/0.1")
	client := &http.Client{
		Timeout:       time.Duration(timeout * float64(time.Second)),
		CheckRedirect: redirectPolicy(maxRedirects, headers),
	}
	if cert != nil {
		t, err := clientCertTransport(cert)
		if err != nil {
			return nil, nil, err
		}
		client.Transport = t
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, cerr.Validation("http: {0}", err)
	}
	defer res.Body.Close()
	// One byte past the limit tells a body of exactly maxBytes from a larger
	// one, which is an error: a truncated body would pass for the whole.
	b, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return nil, nil, cerr.Validation("http: {0}", err)
	}
	if int64(len(b)) > maxBytes {
		return nil, nil, cerr.Validation("http: the response from {0} is larger than {1} bytes (raise opts.maxBytes)", url, maxBytes)
	}
	return res, b, nil
}

// logAttrs is the fields of a ddcore.log record: the user, then the fields
// the call passed in key order. A field named like one the record already has
// (time, level, msg, user) is written as arg.<name> rather than shadowing it.
func logAttrs(user string, attrs map[string]any) []any {
	out := make([]any, 0, 2+2*len(attrs))
	out = append(out, "user", user)
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := k
		switch k {
		case slog.TimeKey, slog.LevelKey, slog.MessageKey, "user":
			name = "arg." + k
		}
		out = append(out, name, attrs[k])
	}
	return out
}

// redirectPolicy follows at most max redirects and, on a hop to another host
// than the first request's, or from https to anything else, drops every header
// the caller passed. net/http only drops Authorization, Cookie and their kin,
// and an API that takes its token as `api_access_token` or `X-Api-Key` would
// hand it to whatever host the redirect names: object storage, a CDN, or a
// plain-http mirror. Content-Type describes the body, which a 307 resends, and
// is kept.
func redirectPolicy(max int, headers map[string]string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= max {
			return http.ErrUseLastResponse
		}
		first := via[0].URL
		if strings.EqualFold(req.URL.Host, first.Host) && (first.Scheme != "https" || req.URL.Scheme == "https") {
			return nil
		}
		for k := range headers {
			if !strings.EqualFold(k, "Content-Type") {
				req.Header.Del(k)
			}
		}
		return nil
	}
}

// validEventName keeps an event name to one SSE token: it is written into the
// stream as is, and a line break in it would forge the events after it.
func validEventName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.:-", r)) {
			return false
		}
	}
	return true
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
