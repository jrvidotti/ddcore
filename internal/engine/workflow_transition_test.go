package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

func workflowTestFiles() map[string]string {
	return map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({
  name: "demo",
  title: "Demo",
  roles: ["Gestor", "Autor", "Editor", "Auditor"],
});`,
		"doctypes/artigo/artigo.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Artigo", {
  onSubmit(doc) {
    ddcore.db.setValue("Artigo", doc.name, "submitted_hook_ran", 1);
  },
  beforeCancel(doc) {
    doc.before_cancel_hook_ran = 1;
  },
  onCancel(doc) {
    ddcore.db.setValue("Artigo", doc.name, "cancel_hook_ran", 1);
  },
});`,
		"doctypes/artigo/artigo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Artigo",
  naming: { series: "ART-.####" },
  submittable: true,
  fields: [
    { fieldname: "titulo", fieldtype: "Data", label: "Título", reqd: true },
    { fieldname: "conteudo", fieldtype: "Small Text", label: "Conteúdo" },
    { fieldname: "workflow_state", fieldtype: "Data", label: "Workflow State" },
    { fieldname: "status", fieldtype: "Data", label: "Status" },
    { fieldname: "submitted_hook_ran", fieldtype: "Int", label: "Submitted Hook Ran" },
    { fieldname: "before_cancel_hook_ran", fieldtype: "Int", label: "Before Cancel Hook Ran" },
    { fieldname: "cancel_hook_ran", fieldtype: "Int", label: "Cancel Hook Ran" },
  ],
  permissions: [
    { role: "Autor", read: true, write: true, create: true, delete: true },
    { role: "Editor", read: true, write: true, create: true, submit: true, cancel: true },
    { role: "Auditor", read: true },
    { role: "All", read: true },
  ],
});`,
		"workflows/artigo.workflow.ts": `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({
  name: "Artigo Approval",
  doctype: "Artigo",
  initialState: "Draft",
  states: [
    { state: "Draft", docstatus: 0, allowEdit: "Autor" },
    { state: "Pending Approval", docstatus: 0, allowEdit: "Editor" },
    { state: "Approved", docstatus: 1, allowEdit: "", updateFields: { status: "Published" } },
    { state: "Archived", docstatus: 1, allowEdit: "", updateFields: { status: "Archived" } },
    { state: "Rejected", docstatus: 2, allowEdit: "" },
  ],
  transitions: [
    { state: "Draft", action: "Submit for Approval", nextState: "Pending Approval", allowed: "Autor" },
    {
      state: "Pending Approval",
      action: "Approve",
      nextState: "Approved",
      allowed: "Editor",
      allowSelfApproval: false,
      condition: (doc) => doc.conteudo && doc.conteudo.length > 5,
    },
    { state: "Pending Approval", action: "Reject", nextState: "Rejected", allowed: "Editor" },
    { state: "Approved", action: "Archive", nextState: "Archived", allowed: "Editor" },
  ],
});`,
	}
}

func setupWorkflowTestUsers(t *testing.T, e *Engine) {
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u1, _ := c.NewDoc("User", Doc{"email": "autor@x.com", "full_name": "Autor User", "new_password": "password123"})
		u1["roles"] = []any{map[string]any{"role": "Autor"}}
		if _, err := c.Insert(u1, SaveOpts{}); err != nil {
			return err
		}

		u2, _ := c.NewDoc("User", Doc{"email": "editor@x.com", "full_name": "Editor User", "new_password": "password123"})
		u2["roles"] = []any{map[string]any{"role": "Editor"}}
		if _, err := c.Insert(u2, SaveOpts{}); err != nil {
			return err
		}

		u3, _ := c.NewDoc("User", Doc{"email": "auditor@x.com", "full_name": "Auditor User", "new_password": "password123"})
		u3["roles"] = []any{map[string]any{"role": "Auditor"}}
		if _, err := c.Insert(u3, SaveOpts{}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to create test users: %v", err)
	}
}

func TestWorkflow_ApplyTransition_SuccessAndDocstatusBinding(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()

	var docName string

	// 1. Autor creates an article in Draft state
	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{
			"titulo":   "Important Article",
			"conteudo": "Detailed content longer than 5 chars",
		})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		docName = saved.Name()
		if saved.Str("workflow_state") != "Draft" {
			t.Fatalf("expected initial state Draft, got %s", saved.Str("workflow_state"))
		}
		if saved.Docstatus() != 0 {
			t.Fatalf("expected initial docstatus 0, got %d", saved.Docstatus())
		}

		// Check AvailableWorkflowActions for autor@x.com
		actions, err := c.AvailableWorkflowActions("Artigo", saved)
		if err != nil {
			return err
		}
		if len(actions) != 1 || actions[0].Action != "Submit for Approval" || actions[0].NextState != "Pending Approval" {
			t.Fatalf("unexpected available actions for autor in Draft: %+v", actions)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	// 2. Autor applies "Submit for Approval" -> Pending Approval (docstatus 0)
	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		// the comment is written in the actor's language; assert the English keys
		c.Lang = "en"
		saved, err := c.ApplyWorkflowTransition("Artigo", docName, "Submit for Approval")
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Pending Approval" {
			t.Fatalf("expected state Pending Approval, got %s", saved.Str("workflow_state"))
		}
		if saved.Docstatus() != 0 {
			t.Fatalf("expected docstatus 0, got %d", saved.Docstatus())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Submit for Approval failed: %v", err)
	}

	// Verify Audit log for "Submit for Approval"
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT actor, action, outcome, target_doctype, target_name, detail FROM tab_audit_event WHERE target_doctype = 'Artigo' AND target_name = $1 AND action = 'workflow.transition' ORDER BY creation ASC`, docName)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 audit event, got %d (err: %v)", len(rows), err)
	}
	if db.Str(rows[0]["actor"]) != "autor@x.com" || db.Str(rows[0]["outcome"]) != "Allowed" {
		t.Fatalf("unexpected audit row: %+v", rows[0])
	}
	detailStr := db.Str(rows[0]["detail"])
	if !strings.Contains(detailStr, "Submit for Approval") || !strings.Contains(detailStr, "Pending Approval") {
		t.Fatalf("unexpected audit detail: %s", detailStr)
	}

	// Verify timeline Comment for "Submit for Approval"
	crows, err := db.Select(ctx, e.DB.Pool, `SELECT comment_type, reference_doctype, reference_name, content FROM tab_comment WHERE reference_doctype = 'Artigo' AND reference_name = $1 ORDER BY creation ASC`, docName)
	if err != nil || len(crows) != 1 {
		t.Fatalf("expected 1 timeline comment, got %d (err: %v)", len(crows), err)
	}
	if db.Str(crows[0]["comment_type"]) != "Workflow" {
		t.Fatalf("expected comment_type Workflow, got %s", db.Str(crows[0]["comment_type"]))
	}
	expectedComment1 := "autor@x.com applied action 'Submit for Approval' (Draft → Pending Approval)"
	if db.Str(crows[0]["content"]) != expectedComment1 {
		t.Fatalf("expected comment %q, got %q", expectedComment1, db.Str(crows[0]["content"]))
	}

	// 3. Check AvailableWorkflowActions for editor@x.com in Pending Approval
	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", docName)
		if err != nil {
			return err
		}
		actions, err := c.AvailableWorkflowActions("Artigo", doc)
		if err != nil {
			return err
		}
		if len(actions) != 2 {
			t.Fatalf("expected 2 actions (Approve, Reject), got %+v", actions)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("check available actions failed: %v", err)
	}

	// 4. Editor applies "Approve" -> Approved (docstatus 1, sets updateFields { status: "Published" }, runs onSubmit hook)
	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		// the comment is written in the actor's language; assert the English keys
		c.Lang = "en"
		saved, err := c.ApplyWorkflowTransition("Artigo", docName, "Approve")
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Approved" {
			t.Fatalf("expected state Approved, got %s", saved.Str("workflow_state"))
		}
		if saved.Docstatus() != 1 {
			t.Fatalf("expected docstatus 1 (submitted), got %d", saved.Docstatus())
		}
		if saved.Str("status") != "Published" {
			t.Fatalf("expected updateFields to set status 'Published', got %q", saved.Str("status"))
		}

		// Verify onSubmit controller hook ran
		hookRan, err := c.GetValue("Artigo", docName, "submitted_hook_ran")
		if err != nil || toFloat(hookRan) != 1 {
			t.Fatalf("expected onSubmit hook to set submitted_hook_ran=1, got %v (err: %v)", hookRan, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	// Verify Audit log now has 2 Allowed events
	rows, err = db.Select(ctx, e.DB.Pool, `SELECT actor, action, outcome, target_doctype, target_name, detail FROM tab_audit_event WHERE target_doctype = 'Artigo' AND target_name = $1 AND action = 'workflow.transition' ORDER BY creation ASC`, docName)
	if err != nil || len(rows) != 2 {
		t.Fatalf("expected 2 audit events, got %d (err: %v)", len(rows), err)
	}
	if db.Str(rows[1]["actor"]) != "editor@x.com" || db.Str(rows[1]["outcome"]) != "Allowed" {
		t.Fatalf("unexpected second audit row: %+v", rows[1])
	}
	detailStr2 := db.Str(rows[1]["detail"])
	if !strings.Contains(detailStr2, "Approve") || !strings.Contains(detailStr2, "Approved") {
		t.Fatalf("unexpected audit detail: %s", detailStr2)
	}

	// Verify timeline Comment now has 2 comments
	crows, err = db.Select(ctx, e.DB.Pool, `SELECT comment_type, reference_doctype, reference_name, content FROM tab_comment WHERE reference_doctype = 'Artigo' AND reference_name = $1 ORDER BY creation ASC`, docName)
	if err != nil || len(crows) != 2 {
		t.Fatalf("expected 2 timeline comments, got %d (err: %v)", len(crows), err)
	}
	expectedComment2 := "editor@x.com applied action 'Approve' (Pending Approval → Approved)"
	if db.Str(crows[1]["content"]) != expectedComment2 {
		t.Fatalf("expected comment %q, got %q", expectedComment2, db.Str(crows[1]["content"]))
	}
}

// TestWorkflow_ApplyTransition_RejectFromDraftCancelsDocument verifies that a
// transition from a docstatus 0 state (Pending Approval) to a docstatus 2
// state (Rejected) succeeds, sets docstatus = 2, and runs the beforeCancel
// and onCancel hooks, instead of failing with "Invalid docstatus transition
// (0 → 2)" as it did before c.inWorkflowTransition exempted this path.
func TestWorkflow_ApplyTransition_RejectFromDraftCancelsDocument(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()

	var docName string
	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{
			"titulo":   "Article to reject",
			"conteudo": "Detailed content longer than 5 chars",
		})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		docName = saved.Name()
		return nil
	})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		_, err := c.ApplyWorkflowTransition("Artigo", docName, "Submit for Approval")
		return err
	})
	if err != nil {
		t.Fatalf("Submit for Approval failed: %v", err)
	}

	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		saved, err := c.ApplyWorkflowTransition("Artigo", docName, "Reject")
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Rejected" {
			t.Fatalf("expected state Rejected, got %s", saved.Str("workflow_state"))
		}
		if saved.Docstatus() != 2 {
			t.Fatalf("expected docstatus 2 (cancelled), got %d", saved.Docstatus())
		}

		beforeCancelRan, err := c.GetValue("Artigo", docName, "before_cancel_hook_ran")
		if err != nil || toFloat(beforeCancelRan) != 1 {
			t.Fatalf("expected beforeCancel hook to set before_cancel_hook_ran=1, got %v (err: %v)", beforeCancelRan, err)
		}
		cancelRan, err := c.GetValue("Artigo", docName, "cancel_hook_ran")
		if err != nil || toFloat(cancelRan) != 1 {
			t.Fatalf("expected onCancel hook to set cancel_hook_ran=1, got %v (err: %v)", cancelRan, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}
}

// TestWorkflow_ApplyTransition_PostSubmitTransitionUpdatesStateField verifies
// that a workflow transition between two submitted states (docstatus 1 → 1,
// action "update_after_submit") is not blocked by checkAllowOnSubmit: the
// workflow's stateField and the target state's updateFields keys must be
// exempt from the "cannot be changed after submission" guard.
func TestWorkflow_ApplyTransition_PostSubmitTransitionUpdatesStateField(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()

	var docName string
	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{
			"titulo":   "Article to archive",
			"conteudo": "Detailed content longer than 5 chars",
		})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		docName = saved.Name()
		return nil
	})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		_, err := c.ApplyWorkflowTransition("Artigo", docName, "Submit for Approval")
		return err
	})
	if err != nil {
		t.Fatalf("Submit for Approval failed: %v", err)
	}

	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		_, err := c.ApplyWorkflowTransition("Artigo", docName, "Approve")
		return err
	})
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		saved, err := c.ApplyWorkflowTransition("Artigo", docName, "Archive")
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Archived" {
			t.Fatalf("expected state Archived, got %s", saved.Str("workflow_state"))
		}
		if saved.Docstatus() != 1 {
			t.Fatalf("expected docstatus to remain 1, got %d", saved.Docstatus())
		}
		if saved.Str("status") != "Archived" {
			t.Fatalf("expected updateFields to set status 'Archived', got %q", saved.Str("status"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Archive (post-submit transition) failed: %v", err)
	}
}

func TestWorkflow_ApplyTransition_RoleAndConditionDenied(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()

	// 1. Role Denied: auditor@x.com tries to transition Draft -> Pending Approval (only Autor allowed)
	var doc1Name string
	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "Doc 1", "conteudo": "Valid content"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		doc1Name = saved.Name()
		return nil
	})
	if err != nil {
		t.Fatalf("insert doc1 failed: %v", err)
	}

	err = e.Run(ctx, "auditor@x.com", func(c *Ctx) error {
		_, err := c.ApplyWorkflowTransition("Artigo", doc1Name, "Submit for Approval")
		if err == nil {
			t.Fatalf("expected role check to reject auditor@x.com")
		}
		if got := cerr.From(err).Type; got != "PermissionError" {
			t.Fatalf("expected PermissionError, got %q (%v)", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("auditor test failed: %v", err)
	}

	// Check AuditDenied was written to tab_audit_event for auditor@x.com
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT actor, action, outcome, target_doctype, target_name, detail FROM tab_audit_event WHERE target_doctype = 'Artigo' AND target_name = $1 AND outcome = 'Denied'`, doc1Name)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 Denied audit row, got %d (err: %v)", len(rows), err)
	}
	if db.Str(rows[0]["actor"]) != "auditor@x.com" {
		t.Fatalf("expected actor auditor@x.com, got %s", db.Str(rows[0]["actor"]))
	}

	// 2. Condition Denied: content length <= 5 fails Approve condition: (doc) => doc.conteudo && doc.conteudo.length > 5
	var doc2Name string
	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "Doc 2", "conteudo": "tiny"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		doc2Name = saved.Name()
		_, err = c.ApplyWorkflowTransition("Artigo", doc2Name, "Submit for Approval")
		return err
	})
	if err != nil {
		t.Fatalf("prepare doc2 failed: %v", err)
	}

	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		// AvailableWorkflowActions should NOT include Approve because condition fails
		doc, err := c.GetDoc("Artigo", doc2Name)
		if err != nil {
			return err
		}
		actions, err := c.AvailableWorkflowActions("Artigo", doc)
		if err != nil {
			return err
		}
		for _, a := range actions {
			if a.Action == "Approve" {
				t.Fatalf("Approve action should not be available when condition fails")
			}
		}

		// ApplyWorkflowTransition should reject with ValidationError
		_, err = c.ApplyWorkflowTransition("Artigo", doc2Name, "Approve")
		if err == nil {
			t.Fatalf("expected condition to reject Approve")
		}
		if got := cerr.From(err).Type; got != "ValidationError" {
			t.Fatalf("expected ValidationError, got %q (%v)", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("condition test failed: %v", err)
	}

	// Check AuditDenied for condition failure
	rows, err = db.Select(ctx, e.DB.Pool, `SELECT actor, action, outcome, target_doctype, target_name FROM tab_audit_event WHERE target_doctype = 'Artigo' AND target_name = $1 AND outcome = 'Denied'`, doc2Name)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 Denied audit row for doc2, got %d (err: %v)", len(rows), err)
	}
	if db.Str(rows[0]["actor"]) != "editor@x.com" {
		t.Fatalf("expected actor editor@x.com, got %s", db.Str(rows[0]["actor"]))
	}

	// 3. Self-approval restricted: doc owned by editor@x.com cannot be approved by editor@x.com
	var doc3Name string
	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "Doc 3", "conteudo": "Long valid content owned by editor"})
		if err != nil {
			return err
		}
		// Editor has create permission
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		doc3Name = saved.Name()

		// Advance to Pending Approval using WithWorkflowTransition or Administrator
		return nil
	})
	if err != nil {
		t.Fatalf("create doc3 failed: %v", err)
	}

	// Advance doc3 to Pending Approval
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", doc3Name)
		if err != nil {
			return err
		}
		doc["workflow_state"] = "Pending Approval"
		return c.WithWorkflowTransition(func() error {
			_, err := c.Save(doc, SaveOpts{})
			return err
		})
	})
	if err != nil {
		t.Fatalf("advance doc3 failed: %v", err)
	}

	// editor@x.com (the owner) attempts self-approval
	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", doc3Name)
		if err != nil {
			return err
		}

		// AvailableWorkflowActions should NOT include Approve due to allowSelfApproval: false
		actions, err := c.AvailableWorkflowActions("Artigo", doc)
		if err != nil {
			return err
		}
		for _, a := range actions {
			if a.Action == "Approve" {
				t.Fatalf("Approve action should not be available to doc owner when allowSelfApproval is false")
			}
		}

		// ApplyWorkflowTransition should reject with PermissionError
		_, err = c.ApplyWorkflowTransition("Artigo", doc3Name, "Approve")
		if err == nil {
			t.Fatalf("expected self-approval to be rejected")
		}
		if got := cerr.From(err).Type; got != "PermissionError" {
			t.Fatalf("expected PermissionError, got %q (%v)", got, err)
		}
		if !strings.Contains(err.Error(), "Self-approval is not allowed") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("self-approval test failed: %v", err)
	}

	// Administrator CAN self-approve
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		saved, err := c.ApplyWorkflowTransition("Artigo", doc3Name, "Approve")
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Approved" {
			t.Fatalf("expected Administrator to bypass self-approval check, got %s", saved.Str("workflow_state"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("administrator approval failed: %v", err)
	}
}

func TestWorkflow_ApplyTransition_Concurrency(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()

	var docName string

	// Create article in Pending Approval state
	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{
			"titulo":   "Concurrent Article",
			"conteudo": "Valid long content for concurrency test",
		})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		docName = saved.Name()
		_, err = c.ApplyWorkflowTransition("Artigo", docName, "Submit for Approval")
		return err
	})
	if err != nil {
		t.Fatalf("setup doc failed: %v", err)
	}

	// Run 2 concurrent goroutines calling ApplyWorkflowTransition("Approve") on the same doc
	var wg sync.WaitGroup
	type result struct {
		err error
		doc Doc
	}
	results := make([]result, 2)

	for i := 0; i < 2; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := e.Run(ctx, "editor@x.com", func(c *Ctx) error {
				saved, err := c.ApplyWorkflowTransition("Artigo", docName, "Approve")
				if err != nil {
					return err
				}
				results[idx].doc = saved
				return nil
			})
			results[idx].err = err
		}()
	}

	wg.Wait()

	// Exactly 1 must succeed, exactly 1 must fail
	successCount := 0
	failCount := 0
	var failureErr error

	for _, r := range results {
		if r.err == nil {
			successCount++
		} else {
			failCount++
			failureErr = r.err
		}
	}

	if successCount != 1 || failCount != 1 {
		t.Fatalf("expected exactly 1 success and 1 failure, got %d successes and %d failures (res0: %v, res1: %v)",
			successCount, failCount, results[0].err, results[1].err)
	}

	// The failure must be a ValidationError because the state already moved to Approved
	if got := cerr.From(failureErr).Type; got != "ValidationError" {
		t.Fatalf("expected failed goroutine to get ValidationError, got %q (%v)", got, failureErr)
	}
	if !strings.Contains(failureErr.Error(), "state 'Approved'") {
		t.Fatalf("expected error to mention state 'Approved', got %v", failureErr)
	}

	// Verify no duplicated comments (exactly 1 for Submit for Approval, 1 for Approve)
	crows, err := db.Select(ctx, e.DB.Pool, `SELECT comment_type, content FROM tab_comment WHERE reference_doctype = 'Artigo' AND reference_name = $1 ORDER BY creation ASC`, docName)
	if err != nil {
		t.Fatalf("failed to query comments: %v", err)
	}
	if len(crows) != 2 {
		t.Fatalf("expected exactly 2 comments, got %d: %+v", len(crows), crows)
	}
	approveComments := 0
	for _, cr := range crows {
		if strings.Contains(db.Str(cr["content"]), "'Approve'") {
			approveComments++
		}
	}
	if approveComments != 1 {
		t.Fatalf("expected exactly 1 Approve comment, got %d", approveComments)
	}

	// Verify document is docstatus 1
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", docName)
		if err != nil {
			return err
		}
		if doc.Docstatus() != 1 {
			t.Fatalf("expected docstatus 1, got %d", doc.Docstatus())
		}
		if doc.Str("workflow_state") != "Approved" {
			t.Fatalf("expected state Approved, got %s", doc.Str("workflow_state"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
}

func TestWorkflow_AvailableActionsAndValidationErrors(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()

	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		// Non-existent workflow
		actions, err := c.AvailableWorkflowActions("NonExistentDocType", Doc{"name": "test"})
		if err != nil || actions != nil {
			t.Fatalf("expected nil actions for unknown doctype, got %+v (err: %v)", actions, err)
		}

		// Nil doc
		actions, err = c.AvailableWorkflowActions("Artigo", nil)
		if err != nil || actions != nil {
			t.Fatalf("expected nil actions for nil doc, got %+v (err: %v)", actions, err)
		}

		// Apply transition with non-existent workflow
		_, err = c.ApplyWorkflowTransition("NonExistentDocType", "test", "Approve")
		if err == nil || cerr.From(err).Type != "ValidationError" {
			t.Fatalf("expected ValidationError for doctype without workflow, got %v", err)
		}

		// Non-existent document
		_, err = c.ApplyWorkflowTransition("Artigo", "NON-EXISTING-DOC", "Submit for Approval")
		if err == nil || cerr.From(err).Type != "DoesNotExistError" {
			t.Fatalf("expected DoesNotExistError for missing document, got %v", err)
		}

		// Invalid action from Draft state
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "Draft Artigo", "conteudo": "Valid content"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}

		_, err = c.ApplyWorkflowTransition("Artigo", saved.Name(), "NonExistentAction")
		if err == nil || cerr.From(err).Type != "ValidationError" {
			t.Fatalf("expected ValidationError for invalid action, got %v", err)
		}

		// Trying "Approve" directly from Draft state (only valid from Pending Approval)
		_, err = c.ApplyWorkflowTransition("Artigo", saved.Name(), "Approve")
		if err == nil || cerr.From(err).Type != "ValidationError" {
			t.Fatalf("expected ValidationError for invalid action from Draft, got %v", err)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("edge case tests failed: %v", err)
	}
}


// TestWorkflow_ApplyWorkflowFromTS covers doc.applyWorkflow, the server-side
// binding: it runs the same checks and lifecycle hooks as the HTTP endpoint,
// and an amendment of the cancelled document starts over at the initial state.
func TestWorkflow_ApplyWorkflowFromTS(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()

	eval := func(user, code string) (json.RawMessage, error) {
		var out json.RawMessage
		err := e.Run(ctx, user, func(c *Ctx) error {
			rt, err := c.RT()
			if err != nil {
				return err
			}
			out, err = rt.Eval(code)
			return err
		})
		return out, err
	}

	raw, err := eval("autor@x.com", `(() => {
		const d = ddcore.newDoc("Artigo", { titulo: "Via TS", conteudo: "Detailed content" }).insert();
		d.applyWorkflow("Submit for Approval");
		return { name: d.name, state: d.workflow_state, docstatus: d.docstatus };
	})()`)
	if err != nil {
		t.Fatalf("applyWorkflow as autor failed: %v", err)
	}
	var r struct {
		Name      string `json:"name"`
		State     string `json:"state"`
		Docstatus int    `json:"docstatus"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.State != "Pending Approval" || r.Docstatus != 0 {
		t.Fatalf("expected Pending Approval/0, got %+v", r)
	}

	// the role check is not skipped because the call comes from app code
	_, err = eval("autor@x.com", `ddcore.getDoc("Artigo", "`+r.Name+`").applyWorkflow("Reject")`)
	if err == nil || !strings.Contains(err.Error(), "No permission") {
		t.Fatalf("expected permission error for autor rejecting, got %v", err)
	}

	raw, err = eval("editor@x.com", `(() => {
		const d = ddcore.getDoc("Artigo", "`+r.Name+`").applyWorkflow("Reject");
		return { name: d.name, state: d.workflow_state, docstatus: d.docstatus };
	})()`)
	if err != nil {
		t.Fatalf("applyWorkflow as editor failed: %v", err)
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.State != "Rejected" || r.Docstatus != 2 {
		t.Fatalf("expected Rejected/2, got %+v", r)
	}

	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		if v, _ := c.GetValue("Artigo", r.Name, "cancel_hook_ran"); toFloat(v) != 1 {
			t.Fatalf("expected onCancel to run, got %v", v)
		}
		draft, err := c.Amend("Artigo", r.Name)
		if err != nil {
			return err
		}
		if draft.Str("workflow_state") != "" {
			t.Fatalf("expected amend to clear the state, got %q", draft.Str("workflow_state"))
		}
		saved, err := c.Insert(draft, SaveOpts{})
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Draft" {
			t.Fatalf("expected amended document in Draft, got %q", saved.Str("workflow_state"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("amend failed: %v", err)
	}
}
