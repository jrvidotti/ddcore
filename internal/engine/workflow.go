package engine

import (
	"fmt"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

// WorkflowAvailableAction describes an action available for a document in its current state.
type WorkflowAvailableAction struct {
	Action    string `json:"action"`
	NextState string `json:"nextState"`
}

// AvailableWorkflowActions returns all workflow actions the current user can execute
// on doc given its current state, role permissions, self-approval rules, and conditions.
func (c *Ctx) AvailableWorkflowActions(doctype string, doc Doc) ([]WorkflowAvailableAction, error) {
	wf := c.WorkflowFor(doctype)
	if wf == nil || doc == nil {
		return nil, nil
	}
	currentState := doc.Str(wf.StateField)
	if currentState == "" {
		currentState = wf.InitialState
	}
	var out []WorkflowAvailableAction
	for _, tr := range wf.Transitions {
		if tr.State != currentState {
			continue
		}
		// Check roles
		hasRole := c.User == "Administrator" || c.IgnorePermissions()
		if !hasRole {
			for _, role := range tr.Allowed {
				if c.HasRole(role) {
					hasRole = true
					break
				}
			}
		}
		if !hasRole {
			continue
		}
		// Check self approval
		if !tr.AllowSelfApproval && doc.Str("owner") == c.User && c.User != "Administrator" {
			continue
		}
		// Check condition
		if tr.HasCondition {
			rt, err := c.RT()
			if err != nil {
				return nil, err
			}
			ok, err := rt.EvaluateWorkflowCondition(wf.Name, tr.Index, doc.JSON())
			if err != nil || !ok {
				continue
			}
		}
		out = append(out, WorkflowAvailableAction{Action: tr.Action, NextState: tr.NextState})
	}
	return out, nil
}

// ApplyWorkflowTransition executes an atomic workflow transition on a document.
// It acquires a row lock (FOR UPDATE), validates the transition against current state,
// checks role authorization, self-approval restrictions, and conditions, mutates state/docstatus,
// writes an audit event, and logs a timeline comment.
func (c *Ctx) ApplyWorkflowTransition(doctype, name, action string) (Doc, error) {
	if c.Tx == nil {
		var res Doc
		err := c.Run(func(txCtx *Ctx) error {
			var err error
			res, err = txCtx.ApplyWorkflowTransition(doctype, name, action)
			return err
		})
		return res, err
	}

	wf := c.WorkflowFor(doctype)
	if wf == nil {
		return nil, cerr.Validation("No workflow configured for {0}", doctype)
	}

	// 1. Lock document with FOR UPDATE
	doc, err := c.getDocForUpdate(doctype, name)
	if err != nil {
		return nil, err
	}
	currentState := doc.Str(wf.StateField)
	if currentState == "" {
		currentState = wf.InitialState
	}

	// 2. Find matching transition
	var matched *js.WorkflowTransition
	for i := range wf.Transitions {
		tr := &wf.Transitions[i]
		if tr.State == currentState && tr.Action == action {
			matched = tr
			break
		}
	}
	if matched == nil {
		return nil, cerr.Validation("Action '{0}' is not valid for {1} {2} in state '{3}'", action, doctype, name, currentState)
	}

	// 3. Check role authorization
	hasRole := c.User == "Administrator" || c.IgnorePermissions()
	if !hasRole {
		for _, role := range matched.Allowed {
			if c.HasRole(role) {
				hasRole = true
				break
			}
		}
	}
	detail := map[string]any{
		"action":     action,
		"from_state": currentState,
		"to_state":   matched.NextState,
	}
	if !hasRole {
		c.AuditDenied("workflow.transition", doctype, name, detail)
		return nil, cerr.Permission("No permission to execute action '{0}' on {1} {2}", action, doctype, name)
	}

	// 4. Check self approval
	if !matched.AllowSelfApproval && doc.Str("owner") == c.User && c.User != "Administrator" {
		c.AuditDenied("workflow.transition", doctype, name, detail)
		return nil, cerr.Permission("Self-approval is not allowed for action '{0}' on {1} {2}", action, doctype, name)
	}

	// 5. Evaluate condition
	if matched.HasCondition {
		rt, err := c.RT()
		if err != nil {
			return nil, err
		}
		ok, err := rt.EvaluateWorkflowCondition(wf.Name, matched.Index, doc.JSON())
		if err != nil {
			return nil, err
		}
		if !ok {
			c.AuditDenied("workflow.transition", doctype, name, detail)
			return nil, cerr.Validation("Condition for action '{0}' was not met", action)
		}
	}

	// 6. Apply state change & docstatus
	targetState := wf.FindState(matched.NextState)
	doc[wf.StateField] = matched.NextState
	if targetState != nil && targetState.UpdateFields != nil {
		for k, v := range targetState.UpdateFields {
			doc[k] = v
		}
	}

	oldStatus := doc.Docstatus()
	newStatus := oldStatus
	if targetState != nil {
		newStatus = targetState.Docstatus
	}
	doc["docstatus"] = newStatus

	var saved Doc
	err = c.WithWorkflowTransition(func() error {
		var saveErr error
		saved, saveErr = c.SaveDoc(doc, SaveOpts{})
		return saveErr
	})
	if err != nil {
		return nil, err
	}

	// 7. Write Audit Event & Comment
	if err := c.Audit("workflow.transition", doctype, name, detail); err != nil {
		return nil, err
	}
	commentContent := fmt.Sprintf("%s applied action '%s' (%s → %s)", c.User, action, currentState, matched.NextState)
	comment, err := c.NewDoc("Comment", Doc{
		"comment_type":      "Workflow",
		"reference_doctype": doctype,
		"reference_type":    doctype,
		"reference_name":    name,
		"content":           commentContent,
	})
	if err == nil {
		_, _ = c.Insert(comment, SaveOpts{IgnorePermissions: true})
	}

	return saved, nil
}
