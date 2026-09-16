package engine

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
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

// TestWorkflow_InsertRefusesDocstatusOutsideInitialState: an insert carrying
// docstatus 1 would create a submitted document still in the initial state,
// skipping every approval, even for a role holding submit permission.
func TestWorkflow_InsertRefusesDocstatusOutsideInitialState(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()
	err := e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "Pre-submitted", "docstatus": 1})
		if err != nil {
			return err
		}
		if _, err := c.Insert(doc, SaveOpts{}); err == nil {
			t.Fatalf("expected an insert with docstatus 1 to be refused")
		} else if !strings.Contains(err.Error(), "must start with the docstatus of initial workflow state 'Draft'") {
			t.Fatalf("unexpected error: %v", err)
		}
		doc, _ = c.NewDoc("Artigo", Doc{"titulo": "Pre-submitted", "docstatus": 1})
		saved, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
		if err != nil {
			t.Fatalf("expected IgnorePermissions to allow the insert, got %v", err)
		}
		if saved.Docstatus() != 1 {
			t.Fatalf("expected docstatus 1, got %d", saved.Docstatus())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorkflow_DeleteGuard: a document waiting for approval cannot be deleted
// by a user whose role may not edit it in that state.
func TestWorkflow_DeleteGuard(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()
	insert := func(title string) string {
		var name string
		if err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
			doc, _ := c.NewDoc("Artigo", Doc{"titulo": title, "conteudo": "long enough content"})
			saved, err := c.Insert(doc, SaveOpts{})
			if err != nil {
				return err
			}
			name = saved.Name()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return name
	}
	apply := func(user, name, action string) {
		if err := e.Run(ctx, user, func(c *Ctx) error {
			_, err := c.ApplyWorkflowTransition("Artigo", name, action)
			return err
		}); err != nil {
			t.Fatalf("%s failed: %v", action, err)
		}
	}

	// the owner deletes a draft in the initial state
	draft := insert("Draft to delete")
	if err := e.Run(ctx, "autor@x.com", func(c *Ctx) error { return c.Delete("Artigo", draft, false, false) }); err != nil {
		t.Fatalf("expected the owner to delete a draft in the initial state, got %v", err)
	}

	// the owner may not delete it once it waits for an Editor's approval
	pending := insert("Pending to delete")
	apply("autor@x.com", pending, "Submit for Approval")
	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error { return c.Delete("Artigo", pending, false, false) })
	if err == nil || !strings.Contains(err.Error(), "in workflow state 'Pending Approval'") {
		t.Fatalf("expected delete of a pending document to be refused, got %v", err)
	}
	if err := e.Run(ctx, "autor@x.com", func(c *Ctx) error { return c.Delete("Artigo", pending, true, false) }); err != nil {
		t.Fatalf("expected ignorePerms to allow the delete, got %v", err)
	}

	// a cancelled document keeps the existing rules
	rejected := insert("Rejected to delete")
	apply("autor@x.com", rejected, "Submit for Approval")
	apply("editor@x.com", rejected, "Reject")
	if err := e.Run(ctx, "autor@x.com", func(c *Ctx) error { return c.Delete("Artigo", rejected, false, false) }); err != nil {
		t.Fatalf("expected the owner to delete a cancelled document, got %v", err)
	}
}

// TestWorkflow_DBSetGuard: db.setValue must not move a document between
// workflow states or change its docstatus behind the transition's back.
func TestWorkflow_DBSetGuard(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()
	err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, _ := c.NewDoc("Artigo", Doc{"titulo": "DBSet target"})
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		name := saved.Name()
		if err := c.SetValue("Artigo", name, Doc{"workflow_state": "Approved"}); err == nil {
			t.Fatalf("expected a DBSet of the state field to be refused")
		}
		if err := c.SetValue("Artigo", name, Doc{"docstatus": 1}); err == nil {
			t.Fatalf("expected a DBSet of docstatus to be refused")
		}
		if err := c.SetValue("Artigo", name, Doc{"status": "Anything"}); err != nil {
			t.Fatalf("expected a DBSet of another field to succeed, got %v", err)
		}
		if err := c.WithWorkflowTransition(func() error {
			return c.SetValue("Artigo", name, Doc{"workflow_state": "Pending Approval"})
		}); err != nil {
			t.Fatalf("expected a DBSet inside a transition to succeed, got %v", err)
		}
		if err := c.WithIgnorePermissions(func() error {
			return c.SetValue("Artigo", name, Doc{"workflow_state": "Draft"})
		}); err != nil {
			t.Fatalf("expected a DBSet with ignorePermissions to succeed, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorkflow_LoadTimeValidation: a workflow the save lifecycle could never
// run fails loading instead of failing on the first transition.
func TestWorkflow_LoadTimeValidation(t *testing.T) {
	wf := func(doctype, stateField, states, transitions string) map[string]string {
		return map[string]string{"workflows/bad.workflow.ts": `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "Bad", doctype: "` + doctype + `", stateField: "` + stateField + `", initialState: "A",
  states: [` + states + `], transitions: [` + transitions + `] });`}
	}
	pedidoField := map[string]string{"doctypes/pessoa/pessoa.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pessoa", naming: { field: "nome" },
  fields: [{ fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true }, { fieldname: "workflow_state", fieldtype: "Data", label: "Workflow State" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`}
	cases := []struct {
		name  string
		files []map[string]string
		want  string
	}{
		{"unknown doctype", []map[string]string{wf("Nope", "workflow_state", `{ state: "A" }, { state: "B" }`, `{ state: "A", action: "Go", nextState: "B", allowed: "Gestor" }`)}, "unknown DocType Nope"},
		{"unknown state field", []map[string]string{wf("Pedido", "workflow_state", `{ state: "A" }, { state: "B" }`, `{ state: "A", action: "Go", nextState: "B", allowed: "Gestor" }`)}, "state field workflow_state is not a field of Pedido"},
		{"docstatus out of range", []map[string]string{pedidoField, wf("Pessoa", "workflow_state", `{ state: "A" }, { state: "B", docstatus: 3 }`, `{ state: "A", action: "Go", nextState: "B", allowed: "Gestor" }`)}, "docstatus must be 0, 1 or 2"},
		{"submitted state on non-submittable", []map[string]string{pedidoField, wf("Pessoa", "workflow_state", `{ state: "A" }, { state: "B", docstatus: 1 }`, `{ state: "A", action: "Go", nextState: "B", allowed: "Gestor" }`)}, "Pessoa is not submittable"},
		{"docstatus 1 back to 0", []map[string]string{pedidoField, {"doctypes/pessoa/pessoa.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pessoa", naming: { field: "nome" }, submittable: true,
  fields: [{ fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true }, { fieldname: "workflow_state", fieldtype: "Data", label: "Workflow State" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`}, wf("Pessoa", "workflow_state", `{ state: "A" }, { state: "B", docstatus: 1 }`, `{ state: "A", action: "Go", nextState: "B", allowed: "Gestor" }, { state: "B", action: "Back", nextState: "A", allowed: "Gestor" }`)}, "cannot go from docstatus 1 to 0"},
		{"leaves cancelled", []map[string]string{pedidoField, {"doctypes/pessoa/pessoa.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pessoa", naming: { field: "nome" }, submittable: true,
  fields: [{ fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true }, { fieldname: "workflow_state", fieldtype: "Data", label: "Workflow State" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`}, wf("Pessoa", "workflow_state", `{ state: "A" }, { state: "B", docstatus: 2 }`, `{ state: "A", action: "Go", nextState: "B", allowed: "Gestor" }, { state: "B", action: "Reopen", nextState: "A", allowed: "Gestor" }`)}, "cannot leave cancelled state B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(context.Background(), Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, tc.files...)}}, LogLevel: slog.LevelError})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected a load error containing %q, got %v", tc.want, err)
			}
		})
	}
	// the fixture workflow itself loads
	if _, err := New(context.Background(), Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, workflowTestFiles())}}, LogLevel: slog.LevelError}); err != nil {
		t.Fatalf("expected a valid workflow to load, got %v", err)
	}
}

// TestWorkflow_CommentFailureStillCommits: a database failure writing the
// timeline comment must not abort the transition's transaction.
func TestWorkflow_CommentFailureStillCommits(t *testing.T) {
	e := setupWith(t, workflowTestFiles())
	setupWorkflowTestUsers(t, e)
	ctx := context.Background()
	var name string
	if err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		doc, _ := c.NewDoc("Artigo", Doc{"titulo": "Comment failure"})
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		name = saved.Name()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Pool.Exec(ctx, `
CREATE OR REPLACE FUNCTION ddcore_test_fail() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'induced failure'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER ddcore_test_fail BEFORE INSERT ON tab_comment FOR EACH ROW EXECUTE FUNCTION ddcore_test_fail();`); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, "autor@x.com", func(c *Ctx) error {
		_, err := c.ApplyWorkflowTransition("Artigo", name, "Submit for Approval")
		return err
	}); err != nil {
		t.Fatalf("expected the transition to commit despite the comment failure, got %v", err)
	}
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		st, err := c.GetValue("Artigo", name, "workflow_state")
		if err != nil {
			return err
		}
		if st != "Pending Approval" {
			t.Fatalf("expected state Pending Approval, got %v", st)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
