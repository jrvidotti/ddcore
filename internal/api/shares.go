package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// Document sharing (SEC-03). The engine checks the sharer; these handlers only
// decode the request, as the server SDK's ddcore.share.* does.

type shareRequest struct {
	Doctype       string `json:"doctype"`
	ID            string `json:"id"`
	User          string `json:"user"`
	Read          bool   `json:"read"`
	Write         bool   `json:"write"`
	Share         bool   `json:"share"`
	OverrideScope bool   `json:"overrideScope"`
}

type unshareRequest struct {
	Doctype string `json:"doctype"`
	ID      string `json:"id"`
	User    string `json:"user"`
}

func decodeStrict(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return cerr.Validation("Invalid JSON: {0}", err)
	}
	return nil
}

func (s *Server) listDocShares(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		return c.ListDocShares(urlParam(r, "doctype"), urlParam(r, "id"))
	})
}

func (s *Server) shareDoc(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body shareRequest
		if err := decodeStrict(r, &body); err != nil {
			return nil, err
		}
		return c.ShareDoc(body.Doctype, body.ID, body.User, engine.ShareRights{
			Read: body.Read, Write: body.Write, Share: body.Share, OverrideScope: body.OverrideScope,
		})
	})
}

func (s *Server) unshareDoc(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body unshareRequest
		if err := decodeStrict(r, &body); err != nil {
			return nil, err
		}
		if body.Doctype == "" || body.ID == "" || body.User == "" {
			return nil, cerr.Validation("doctype, id and user are required")
		}
		return map[string]any{"ok": true}, c.UnshareDoc(body.Doctype, body.ID, body.User)
	})
}
