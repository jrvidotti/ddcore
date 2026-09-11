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
		return map[string]any{"user": c.User, "roles": roles, "lang": c.Lang, "langs": c.St.I18n.Langs(), "request": c.Request}, nil
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
		return FormatCurrency(toFloat(a.Value), orDefault(a.Currency, e.Cfg.Currency), c.Lang), nil
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
		// devolve o novo modified para o prelude sincronizar o documento (B21)
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
		return HashPassword(a.Text), nil
	case "dropSessions":
		rows, _ := db.Select(c.Ctx, c.Q(), `SELECT sid FROM ddcore_session WHERE "user" = $1`, a.User)
		for _, r := range rows {
			e.Cache.Del("sid:" + db.Str(r["sid"]))
		}
		_, err := c.Q().Exec(c.Ctx, `DELETE FROM ddcore_session WHERE "user" = $1`, a.User)
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
func FormatCurrency(v float64, code, lang string) string {
	tag, err := language.Parse(lang)
	if err != nil {
		tag = language.English
	}
	u, err := currency.ParseISO(code)
	if err != nil {
		// an unknown code prints as itself: "XYZ 1,234.50" is honest, and
		// borrowing another currency's symbol would not be
		return code + " " + message.NewPrinter(tag).Sprintf("%.2f", v)
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
