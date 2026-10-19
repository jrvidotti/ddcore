package engine

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

func init() {
	addHostOps(map[string]hostOp{
		"getMeta": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return orErr(c.St.DocType(a.Doctype)) },
		"getDoc": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return c.GetDocOpts(a.Doctype, a.id(), GetOpts{IgnorePermissions: a.ignorePermissions()})
		},
		"newDoc":     func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return c.NewDoc(a.Doctype, a.Values) },
		"doc.insert": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return docWrite(a, c.Insert) },
		"doc.save":   func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return docWrite(a, c.Save) },
		"doc.cancel": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) { return docWrite(a, c.cancel) },
		// the same transition POST /api/workflow/apply runs: role, self-approval,
		// condition, row lock, audit and timeline comment
		"doc.applyWorkflow": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return c.ApplyWorkflowTransition(a.Doctype, a.id(), a.Action)
		},
		"doc.delete": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return nil, c.deleteWithFlags(a.Doctype, a.id(), a.ignorePermissions(), a.Opts["force"] == true, a.Flags)
		},
		// returns the new modified timestamp for the prelude to synchronize the document (B21)
		"doc.dbSet": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			modified, err := c.DBSet(a.Doctype, a.id(), a.Values, true)
			return orErr(map[string]any{"modified": modified}, err)
		},
		"hasPermission": hostHasPermission,
		// the border an app's own endpoint or report crosses: what an API read
		// of this document would show the current user (SEC-02)
		"redact": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			if a.Doc == nil {
				return nil, nil
			}
			return c.RedactDoc(a.Doctype, a.Doc.Clone())
		},
		"rename": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return c.RenameOrMerge(a.Doctype, a.OldID, a.NewID, a.Opts["merge"] == true)
		},
		"share.add":    hostShare,
		"share.remove": hostShare,
		"share.list":   hostShare,
		"files.save": func(e *Engine, c *Ctx, _ *hostArgs, in hostReq) (any, error) {
			var f SaveFileArgs
			if err := json.Unmarshal(in.raw, &f); err != nil {
				return nil, cerr.Validation("files.save: {0}", err)
			}
			return c.SaveFile(f)
		},
		"files.presign": func(e *Engine, c *Ctx, a *hostArgs, _ hostReq) (any, error) {
			return c.PresignFile(a.URL, time.Duration(a.TTL*float64(time.Second)), a.ignorePermissions())
		},
	})
}

// docWrite runs a write with the document's flags: they go with it and come
// back with what the hooks left in them.
func docWrite(a *hostArgs, write func(Doc, SaveOpts) (Doc, error)) (any, error) {
	opts := saveOpts(a.Opts)
	if opts.Flags = a.Flags; opts.Flags == nil {
		opts.Flags = map[string]any{}
	}
	out, err := write(a.Doc, opts)
	return orErr(map[string]any{"doc": out, "flags": opts.Flags}, err)
}

func hostHasPermission(e *Engine, c *Ctx, a *hostArgs, in hostReq) (any, error) {
	doc := a.Doc
	if doc == nil && len(a.ID) > 0 {
		// a document id: the rules read the stored document, which the
		// caller may not be allowed to load itself
		var id string
		if err := json.Unmarshal(a.ID, &id); err != nil {
			return nil, cerr.Internal("invalid arguments in {0}: {1}", in.op, err)
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
}

func hostShare(e *Engine, c *Ctx, _ *hostArgs, in hostReq) (any, error) {
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
	dec := json.NewDecoder(bytes.NewReader(in.raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sa); err != nil {
		return nil, cerr.Validation("Invalid arguments: {0}", err)
	}
	switch in.op {
	case "share.add":
		return c.ShareDoc(sa.Doctype, sa.ID, sa.User, sa.Rights)
	case "share.remove":
		return nil, c.UnshareDoc(sa.Doctype, sa.ID, sa.User)
	}
	return c.ListDocShares(sa.Doctype, sa.ID)
}
