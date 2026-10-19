package engine

import (
	"encoding/json"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

func init() {
	addHostOps(map[string]hostOp{
		"db.getValue": hostGetValue,
		"db.getList": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			var la ListArgs
			json.Unmarshal(a.Args, &la)
			return c.GetList(a.Doctype, la)
		},
		"db.setValue": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return nil, c.SetValue(a.Doctype, a.id(), a.Values)
		},
		"db.count": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return c.Count(a.Doctype, a.Filters, a.OrFilters)
		},
		"db.exists":  hostExists,
		"db.sql":     func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return c.SQL(a.Query, a.Params) },
		"patchSQL":   func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return c.PatchSQL(a.Query, a.Params) },
		"db.lock":    func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return nil, c.Lock(a.Key) },
		"db.tryLock": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return c.TryLock(a.Key) },
		"db.savepoint.begin": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			return nil, c.SavepointBegin()
		},
		"db.savepoint.release": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			return nil, c.SavepointRelease()
		},
		"db.savepoint.rollback": func(e *Engine, c *Ctx, _ *hostArgs, _ hostReq) (any, error) {
			return nil, c.SavepointRollback()
		},
		"db.getSingleValue": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return c.GetSingleValue(a.Doctype, a.Field)
		},
		"externalDb.sql": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return c.ExternalSQL(a.Key, a.Query, a.Params, a.Timeout)
		},
	})
}

func hostGetValue(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
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
}

func hostExists(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
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
}
