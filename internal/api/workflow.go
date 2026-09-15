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
		wf := c.WorkflowFor(body.Doctype)
		if wf != nil {
			actions, err := c.AvailableWorkflowActions(body.Doctype, saved)
			if err != nil {
				return nil, err
			}
			if actions == nil {
				actions = []engine.WorkflowAvailableAction{}
			}
			state := saved.Str(wf.StateField)
			if state == "" {
				state = wf.InitialState
			}
			saved["_workflow"] = map[string]any{
				"state":   state,
				"actions": actions,
			}
		}
		c.ResolveLinkTitles(body.Doctype, saved)
		return c.RedactDoc(body.Doctype, saved), nil
	})
}

func (s *Server) applyWorkflowTransitionHandler(w http.ResponseWriter, r *http.Request) {
	s.applyWorkflowTransition(w, r)
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
		wf := c.WorkflowFor(doctype)
		var currentState string
		var actions []engine.WorkflowAvailableAction
		if wf != nil {
			currentState = doc.Str(wf.StateField)
			if currentState == "" {
				currentState = wf.InitialState
			}
			acts, err := c.AvailableWorkflowActions(doctype, doc)
			if err != nil {
				return nil, err
			}
			if acts != nil {
				actions = acts
			} else {
				actions = []engine.WorkflowAvailableAction{}
			}
		} else {
			actions = []engine.WorkflowAvailableAction{}
		}
		return map[string]any{
			"state":   currentState,
			"actions": actions,
		}, nil
	})
}

func (s *Server) workflowActionsHandler(w http.ResponseWriter, r *http.Request) {
	s.workflowActions(w, r)
}
