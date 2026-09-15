package js

import "encoding/json"

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

// GetState returns the WorkflowState with the given name, or nil if not found.
func (w *Workflow) GetState(name string) *WorkflowState {
	for i := range w.States {
		if w.States[i].State == name {
			return &w.States[i]
		}
	}
	return nil
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
