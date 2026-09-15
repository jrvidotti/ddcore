package engine

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflow_LifecycleGuards(t *testing.T) {
	files := map[string]string{
		"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({
  name: "demo",
  title: "Demo",
  roles: ["Gestor", "Autor", "Editor"],
  docEvents: { "*": { validate(doc) { doc.flags.seen = true } } }
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
  ],
  permissions: [
    { role: "Autor", read: true, write: true, create: true, delete: true },
    { role: "Editor", read: true, write: true, create: true, submit: true, cancel: true },
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
    { state: "Approved", docstatus: 1, allowEdit: "" },
  ],
  transitions: [
    { state: "Draft", action: "Submit for Approval", nextState: "Pending Approval", allowed: "Autor" },
    { state: "Pending Approval", action: "Approve", nextState: "Approved", allowed: "Editor" },
  ],
});`,
	}

	e := setupWith(t, files)
	ctx := context.Background()

	// Verify workflow loaded in State
	if wf := e.WorkflowFor("Artigo"); wf == nil {
		t.Fatalf("expected workflow for Artigo in State, got nil")
	}

	// Create test users: autor_user (Autor) and editor_user (Editor)
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
		return nil
	})
	if err != nil {
		t.Fatalf("failed to create test users: %v", err)
	}

	var articleDoc Doc

	// 1. Insert: uninitialized doc gets initialState
	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "First Article", "conteudo": "Draft content"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Draft" {
			t.Fatalf("expected workflow_state to be 'Draft', got %q", saved.Str("workflow_state"))
		}
		articleDoc = saved
		return nil
	})
	if err != nil {
		t.Fatalf("1. Insert uninitialized failed: %v", err)
	}

	// 2. Insert with invalid non-initial state: rejected unless IgnorePermissions
	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "Invalid State Article", "workflow_state": "Approved"})
		if err != nil {
			return err
		}
		_, err = c.Insert(doc, SaveOpts{})
		if err == nil {
			t.Fatalf("expected insert with non-initial state to fail")
		}
		if !strings.Contains(err.Error(), "must start in initial workflow state 'Draft'") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// With IgnorePermissions, it should be allowed
		allowedDoc, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
		if err != nil {
			t.Fatalf("expected insert with IgnorePermissions to succeed, got %v", err)
		}
		if allowedDoc.Str("workflow_state") != "Approved" {
			t.Fatalf("expected workflow_state to be 'Approved', got %q", allowedDoc.Str("workflow_state"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("2. Insert invalid state tests failed: %v", err)
	}

	// 3. SaveDoc: direct modification of stateField: rejected
	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", articleDoc.Name())
		if err != nil {
			return err
		}
		doc["workflow_state"] = "Pending Approval"
		_, err = c.Save(doc, SaveOpts{})
		if err == nil {
			t.Fatalf("expected direct stateField mutation to fail")
		}
		if !strings.Contains(err.Error(), "Cannot manually modify workflow state field 'workflow_state'") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("3. SaveDoc direct stateField mutation tests failed: %v", err)
	}

	// 4. SaveDoc: direct submit (docstatus 1): rejected
	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", articleDoc.Name())
		if err != nil {
			return err
		}
		doc["docstatus"] = 1
		_, err = c.Save(doc, SaveOpts{})
		if err == nil {
			t.Fatalf("expected direct submit to fail")
		}
		if !strings.Contains(err.Error(), "Direct submit or cancel is disabled for documents governed by workflow 'Artigo Approval'") {
			t.Fatalf("unexpected error message: %v", err)
		}

		// SubmitDoc / Submit should also fail with same error
		doc, err = c.GetDoc("Artigo", articleDoc.Name())
		if err != nil {
			return err
		}
		_, err = c.Submit(doc)
		if err == nil {
			t.Fatalf("expected Submit to fail")
		}
		if !strings.Contains(err.Error(), "Direct submit or cancel is disabled for documents governed by workflow 'Artigo Approval'") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("4. SaveDoc direct submit tests failed: %v", err)
	}

	// 5. allowEdit: user without allowEdit role cannot edit fields
	// In "Draft" state, allowEdit is "Autor".
	// editor@x.com has "Editor" role, which has write permission on Artigo doctype,
	// but NOT "Autor" role.
	err = e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", articleDoc.Name())
		if err != nil {
			return err
		}

		// HasPermission("write") should return false
		ok, err := c.HasPermission("Artigo", "write", doc)
		if err != nil {
			return err
		}
		if ok {
			t.Fatalf("expected HasPermission(write) to return false for Editor in Draft state")
		}

		// Save should fail
		doc["conteudo"] = "Editor attempted modification"
		_, err = c.Save(doc, SaveOpts{})
		if err == nil {
			t.Fatalf("expected save by Editor to fail due to allowEdit restriction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("5. allowEdit tests for Editor failed: %v", err)
	}

	// autor@x.com has "Autor" role, which matches allowEdit.
	err = e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", articleDoc.Name())
		if err != nil {
			return err
		}

		// HasPermission("write") should return true
		ok, err := c.HasPermission("Artigo", "write", doc)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatalf("expected HasPermission(write) to return true for Autor in Draft state")
		}

		// Save should succeed
		doc["conteudo"] = "Autor valid modification"
		saved, err := c.Save(doc, SaveOpts{})
		if err != nil {
			return err
		}
		if saved.Str("conteudo") != "Autor valid modification" {
			t.Fatalf("expected conteudo to be updated")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("5. allowEdit tests for Autor failed: %v", err)
	}

	// Administrator can edit regardless of allowEdit
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", articleDoc.Name())
		if err != nil {
			return err
		}
		ok, err := c.HasPermission("Artigo", "write", doc)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatalf("expected Administrator to have write permission")
		}
		doc["conteudo"] = "Administrator edit"
		_, err = c.Save(doc, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatalf("5. allowEdit tests for Administrator failed: %v", err)
	}

	// 6. inWorkflowTransition bypasses direct mutation and submit guards
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", articleDoc.Name())
		if err != nil {
			return err
		}
		doc["workflow_state"] = "Pending Approval"
		return c.WithWorkflowTransition(func() error {
			saved, err := c.Save(doc, SaveOpts{})
			if err != nil {
				return err
			}
			if saved.Str("workflow_state") != "Pending Approval" {
				t.Fatalf("expected state to be 'Pending Approval', got %q", saved.Str("workflow_state"))
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("6. inWorkflowTransition bypass test failed: %v", err)
	}
}
