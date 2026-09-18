package api

import (
	"net/http"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

type workflowApplyRequest struct {
	Doctype string `json:"doctype"`
	Name    string `json:"name"`
	Action  string `json:"action"`
}

// enrichWorkflow attaches "_workflow" (current state, whether the acting user
// may edit fields in that state, and the actions available to them) to doc
// when its doctype has an active workflow. It is a no-op otherwise.
func (s *Server) enrichWorkflow(c *engine.Ctx, doctype string, doc engine.Doc) error {
	wf := c.WorkflowFor(doctype)
	if wf == nil || doc == nil {
		return nil
	}
	actions, err := c.AvailableWorkflowActions(doctype, doc)
	if err != nil {
		return err
	}
	if actions == nil {
		actions = []engine.WorkflowAvailableAction{}
	}
	state := doc.Str(wf.StateField)
	if state == "" {
		state = wf.InitialState
	}
	canEdit := true
	if st := wf.FindState(state); st != nil && st.AllowEdit != "" {
		if c.User != "Admin" && !c.IgnorePermissions() {
			canEdit = c.HasRole(st.AllowEdit)
		}
	}
	doc["_workflow"] = map[string]any{
		"state":     state,
		"allowEdit": canEdit,
		"actions":   actions,
	}
	return nil
}

func (s *Server) applyWorkflowTransition(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body workflowApplyRequest
		if err := readJSON(r, &body); err != nil {
			return nil, err
		}
		if body.Doctype == "" || body.Name == "" || body.Action == "" {
			return nil, cerr.Validation("doctype, name and action are required")
		}
		if err := s.requireDocRead(c, body.Doctype, body.Name); err != nil {
			return nil, err
		}
		saved, err := c.ApplyWorkflowTransition(body.Doctype, body.Name, body.Action)
		if err != nil {
			return nil, err
		}
		if err := s.enrichWorkflow(c, body.Doctype, saved); err != nil {
			return nil, err
		}
		c.ResolveLinkTitles(body.Doctype, saved)
		return c.RedactDoc(body.Doctype, saved), nil
	})
}

func (s *Server) workflowActions(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		doctype := r.URL.Query().Get("doctype")
		name := r.URL.Query().Get("name")
		if doctype == "" || name == "" {
			return nil, cerr.Validation("doctype and name are required")
		}
		if err := s.requireDocRead(c, doctype, name); err != nil {
			return nil, err
		}
		doc, err := c.GetDoc(doctype, name)
		if err != nil {
			return nil, err
		}
		if err := s.enrichWorkflow(c, doctype, doc); err != nil {
			return nil, err
		}
		if wf, _ := doc["_workflow"].(map[string]any); wf != nil {
			return wf, nil
		}
		return map[string]any{
			"state":     "",
			"allowEdit": true,
			"actions":   []engine.WorkflowAvailableAction{},
		}, nil
	})
}
