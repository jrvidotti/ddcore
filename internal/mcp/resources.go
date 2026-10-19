package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	ddcore "github.com/jrvidotti/ddcore"
	"github.com/jrvidotti/ddcore/docs"
	"github.com/jrvidotti/ddcore/internal/release"
)

// resources registers the changelog, the reference docs and the meta template.
func (s *server) resources(srv *mcp.Server) {
	e := s.e

	srv.AddResource(&mcp.Resource{URI: "ddcore://changelog", Name: "changelog", MIMEType: "text/markdown", Description: "what changed in the current ddcore minor series, plus Unreleased"},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/markdown", Text: ddcore.Changelog}}}, nil
		})
	for _, a := range release.Archive(docs.ChangelogFS) {
		text := a.Text
		srv.AddResource(&mcp.Resource{URI: "ddcore://changelog/" + a.Minor, Name: "changelog/" + a.Minor, MIMEType: "text/markdown", Description: "what changed in the ddcore " + a.Minor + " series"},
			func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/markdown", Text: text}}}, nil
			})
	}
	for _, n := range docNames() {
		name := n
		srv.AddResource(&mcp.Resource{URI: "ddcore://docs/" + name, Name: "docs/" + name, MIMEType: "text/markdown", Description: "ddcore reference: " + name},
			func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "text/markdown", Text: Docs(name)}}}, nil
			})
	}
	srv.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "ddcore://meta/{doctype}", Name: "meta", MIMEType: "application/json", Description: "A DocType's meta"},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			name := strings.TrimPrefix(req.Params.URI, "ddcore://meta/")
			d, err := e.Current().DocType(name)
			if err != nil {
				return nil, err
			}
			b, _ := json.MarshalIndent(d, "", "  ")
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(b)}}}, nil
		})
}
