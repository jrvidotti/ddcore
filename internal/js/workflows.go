package js

import (
	"encoding/json"
	"fmt"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// Workflow represents a declarative approval workflow defined via defineWorkflow.
type Workflow struct {
	Name         string               `json:"name"`
	Doctype      string               `json:"doctype"`
	StateField   string               `json:"stateField"`
	InitialState string               `json:"initialState"`
	App          string               `json:"app"`
	SourceFile   string               `json:"sourceFile"`
	States       []WorkflowState      `json:"states"`
	Transitions  []WorkflowTransition `json:"transitions"`
}

// WorkflowState defines a state within a workflow.
type WorkflowState struct {
	State        string         `json:"state"`
	Docstatus    int            `json:"docstatus"`
	AllowEdit    string         `json:"allowEdit,omitempty"`
	UpdateFields map[string]any `json:"updateFields,omitempty"`
}

// WorkflowTransition defines an allowed transition between workflow states.
type WorkflowTransition struct {
	Index             int      `json:"index"`
	State             string   `json:"state"`
	Action            string   `json:"action"`
	NextState         string   `json:"nextState"`
	Allowed           []string `json:"allowed"`
	AllowSelfApproval bool     `json:"allowSelfApproval"`
	HasCondition      bool     `json:"hasCondition"`
}

// Snapshot is a view of the JS registry as seen by Go.
type Snapshot struct {
	Workflows map[string]Workflow `json:"workflows"`
}

// ValidateTarget runs after extensions have been merged into the registry. It
// refuses a workflow the save lifecycle could never run: Save rejects a
// docstatus 1 → 0 change and any change to a cancelled document.
func (w Workflow) ValidateTarget(reg *meta.Registry) error {
	d, ok := reg.DocTypes[w.Doctype]
	if !ok {
		return fmt.Errorf("workflow %s: unknown DocType %s", w.Name, w.Doctype)
	}
	if d.Field(w.StateField) == nil {
		return fmt.Errorf("workflow %s: state field %s is not a field of %s", w.Name, w.StateField, w.Doctype)
	}
	for _, s := range w.States {
		if s.Docstatus < 0 || s.Docstatus > 2 {
			return fmt.Errorf("workflow %s: state %s: docstatus must be 0, 1 or 2", w.Name, s.State)
		}
		if s.Docstatus != 0 && !d.Submittable {
			return fmt.Errorf("workflow %s: state %s has docstatus %d but %s is not submittable", w.Name, s.State, s.Docstatus, w.Doctype)
		}
	}
	for _, tr := range w.Transitions {
		from, to := w.GetState(tr.State), w.GetState(tr.NextState)
		if from == nil || to == nil {
			continue // the prelude already refuses unknown states
		}
		if from.Docstatus == 2 {
			return fmt.Errorf("workflow %s: transition %q cannot leave cancelled state %s", w.Name, tr.Action, tr.State)
		}
		if from.Docstatus == 1 && to.Docstatus == 0 {
			return fmt.Errorf("workflow %s: transition %q cannot go from docstatus 1 to 0 (%s → %s)", w.Name, tr.Action, tr.State, tr.NextState)
		}
	}
	return nil
}

// GetState returns the WorkflowState with the given name, or nil if not found.
func (w *Workflow) GetState(name string) *WorkflowState {
	for i := range w.States {
		if w.States[i].State == name {
			return &w.States[i]
		}
	}
	return nil
}

// FindState is an alias for GetState.
func (w *Workflow) FindState(name string) *WorkflowState {
	return w.GetState(name)
}

// EvaluateWorkflowCondition evaluates the condition of a workflow transition against a document.
func (rt *Runtime) EvaluateWorkflowCondition(wfName string, transitionIdx int, doc json.RawMessage) (bool, error) {
	docStr := string(doc)
	if len(doc) == 0 {
		docStr = "{}"
	}
	s, err := rt.callReg("evaluateWorkflowCondition", wfName, transitionIdx, docStr)
	if err != nil {
		return false, err
	}
	return s == "true", nil
}
