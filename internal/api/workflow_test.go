package api

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func workflowTestApp(t *testing.T) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		p := filepath.Join(dir, filepath.Dir(rel))
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({
  name: "demo",
  title: "Demo",
  roles: ["Gestor", "Autor", "Editor", "Auditor"],
});`)

	w("doctypes/artigo/artigo.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Artigo",
  naming: { series: "ART-.####" },
  submittable: true,
  fields: [
    { fieldname: "titulo", fieldtype: "Data", label: "Título", reqd: true },
    { fieldname: "conteudo", fieldtype: "Small Text", label: "Conteúdo" },
    { fieldname: "workflow_state", fieldtype: "Data", label: "Workflow State" },
    { fieldname: "status", fieldtype: "Data", label: "Status" },
  ],
  permissions: [
    { role: "Autor", read: true, write: true, create: true, delete: true },
    { role: "Editor", read: true, write: true, create: true, submit: true, cancel: true },
    { role: "Auditor", read: true },
  ],
});`)

	w("doctypes/pessoa/pessoa.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Pessoa",
  naming: { field: "nome" },
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
  ],
  permissions: [
    { role: "All", read: true, write: true, create: true },
  ],
});`)

	w("workflows/artigo.workflow.ts", `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({
  name: "Artigo Approval",
  doctype: "Artigo",
  initialState: "Draft",
  states: [
    { state: "Draft", docstatus: 0, allowEdit: "Autor" },
    { state: "Pending Approval", docstatus: 0, allowEdit: "Editor" },
    { state: "Approved", docstatus: 1, allowEdit: "", updateFields: { status: "Published" } },
    { state: "Rejected", docstatus: 0, allowEdit: "" },
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
  ],
});`)

	return dir
}

type workflowEnv struct {
	x            *env
	autor        string
	editor       string
	auditor      string
	autoreditor  string
	unauthorized string
}

func setupWorkflowEnv(t *testing.T) *workflowEnv {
	t.Helper()
	appDir := workflowTestApp(t)
	x := setupApp(t, appDir)

	x.asAdmin(func(c *engine.Ctx) error {
		users := []struct {
			email string
			roles []string
		}{
			{"autor@x.com", []string{"Autor"}},
			{"editor@x.com", []string{"Editor"}},
			{"auditor@x.com", []string{"Auditor"}},
			{"autoreditor@x.com", []string{"Autor", "Editor"}},
		}
		for _, u := range users {
			d, err := c.NewDoc("User", engine.Doc{
				"email":        u.email,
				"full_name":    u.email,
				"new_password": "password123",
			})
			if err != nil {
				return err
			}
			var roles []any
			for _, r := range u.roles {
				roles = append(roles, map[string]any{"role": r})
			}
			d["roles"] = roles
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})

	return &workflowEnv{
		x:            x,
		autor:        "sid:" + x.sidWithPwd("autor@x.com", "password123"),
		editor:       "sid:" + x.sidWithPwd("editor@x.com", "password123"),
		auditor:      "sid:" + x.sidWithPwd("auditor@x.com", "password123"),
		autoreditor:  "sid:" + x.sidWithPwd("autoreditor@x.com", "password123"),
		unauthorized: "sid:" + x.sid("ze@x.com"),
	}
}

func (x *env) sidWithPwd(user, pwd string) string {
	x.t.Helper()
	sid, err := x.e.Login(x.ctx, user, pwd, engine.LoginFrom{IP: "127.0.0.1", UserAgent: "test"})
	if err != nil {
		x.t.Fatal(err)
	}
	return sid
}

func TestWorkflowAPI_ApplyAndActions(t *testing.T) {
	wf := setupWorkflowEnv(t)
	x, autor, editor, auditor := wf.x, wf.autor, wf.editor, wf.auditor

	// 1. Autor creates Artigo
	r := x.call("POST", "/api/resource/Artigo", map[string]any{
		"titulo":   "Artigo Teste",
		"conteudo": "Conteudo com mais de cinco caracteres",
	}, autor)
	x.expect(r, 200, "")
	artigoDoc := r.Body["data"].(map[string]any)
	artigoName := fmt.Sprint(artigoDoc["name"])
	if artigoName == "" {
		t.Fatalf("expected non-empty artigo name, got %v", artigoDoc)
	}

	// 2. Integrated doc payload on GET /api/resource/Artigo/{name}
	// For Autor: should see state Draft and action "Submit for Approval"
	r = x.call("GET", "/api/resource/Artigo/"+artigoName, nil, autor)
	x.expect(r, 200, "")
	docData := r.Body["data"].(map[string]any)
	wfData, ok := docData["_workflow"].(map[string]any)
	if !ok || wfData == nil {
		t.Fatalf("expected _workflow in GET /api/resource payload, got %v", docData)
	}
	if wfData["state"] != "Draft" {
		t.Fatalf("expected state Draft, got %v", wfData["state"])
	}
	actions, ok := wfData["actions"].([]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("expected 1 action for autor in Draft state, got %v", wfData["actions"])
	}
	firstAct := actions[0].(map[string]any)
	if firstAct["action"] != "Submit for Approval" || firstAct["nextState"] != "Pending Approval" {
		t.Fatalf("unexpected action: %v", firstAct)
	}

	// For Editor: GET /api/resource/Artigo/{name} has _workflow with 0 actions (Editor cannot act in Draft)
	r = x.call("GET", "/api/resource/Artigo/"+artigoName, nil, editor)
	x.expect(r, 200, "")
	editorDoc := r.Body["data"].(map[string]any)
	editorWf := editorDoc["_workflow"].(map[string]any)
	if editorWf["state"] != "Draft" {
		t.Fatalf("expected state Draft, got %v", editorWf["state"])
	}
	editorActions := editorWf["actions"].([]any)
	if len(editorActions) != 0 {
		t.Fatalf("expected 0 actions for editor in Draft, got %v", editorActions)
	}

	// 3. GET /api/workflow/actions
	// For Autor: returns state Draft and actions
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&name="+artigoName, nil, autor)
	x.expect(r, 200, "")
	actionsData := r.Body["data"].(map[string]any)
	if actionsData["state"] != "Draft" {
		t.Fatalf("expected state Draft, got %v", actionsData["state"])
	}
	actList := actionsData["actions"].([]any)
	if len(actList) != 1 {
		t.Fatalf("expected 1 action, got %v", actList)
	}

	// For Editor: returns state Draft and empty actions
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&name="+artigoName, nil, editor)
	x.expect(r, 200, "")
	actionsData = r.Body["data"].(map[string]any)
	if actionsData["state"] != "Draft" {
		t.Fatalf("expected state Draft, got %v", actionsData["state"])
	}
	if len(actionsData["actions"].([]any)) != 0 {
		t.Fatalf("expected 0 actions for editor, got %v", actionsData["actions"])
	}

	// Missing query params
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo", nil, autor)
	x.expect(r, 417, "ValidationError")

	// Non-existent document
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&name=NONEXISTENT", nil, autor)
	x.expect(r, 404, "DoesNotExistError")

	// 4. POST /api/workflow/apply
	// Unauthorized action: Editor tries to apply "Submit for Approval" -> 403
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Submit for Approval",
	}, editor)
	x.expect(r, 403, "PermissionError")

	// Missing fields in apply -> 417
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
	}, autor)
	x.expect(r, 417, "ValidationError")

	// Autor applies valid transition "Submit for Approval" -> 200 OK
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Submit for Approval",
	}, autor)
	x.expect(r, 200, "")
	applyData := r.Body["data"].(map[string]any)
	if applyData["workflow_state"] != "Pending Approval" {
		t.Fatalf("expected workflow_state Pending Approval, got %v", applyData["workflow_state"])
	}
	applyWf, ok := applyData["_workflow"].(map[string]any)
	if !ok || applyWf == nil {
		t.Fatalf("expected _workflow in apply response, got %v", applyData)
	}
	if applyWf["state"] != "Pending Approval" {
		t.Fatalf("expected _workflow.state Pending Approval, got %v", applyWf["state"])
	}

	// In Pending Approval:
	// GET /api/workflow/actions for Editor: should see Approve and Reject
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&name="+artigoName, nil, editor)
	x.expect(r, 200, "")
	editorActionsData := r.Body["data"].(map[string]any)
	editorActs := editorActionsData["actions"].([]any)
	if len(editorActs) != 2 {
		t.Fatalf("expected 2 actions (Approve, Reject) for editor, got %v", editorActs)
	}

	// Editor applies Approve -> 200 OK
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Approve",
	}, editor)
	x.expect(r, 200, "")
	approvedDoc := r.Body["data"].(map[string]any)
	if approvedDoc["workflow_state"] != "Approved" {
		t.Fatalf("expected workflow_state Approved, got %v", approvedDoc["workflow_state"])
	}
	if approvedDoc["status"] != "Published" {
		t.Fatalf("expected status Published (updateFields), got %v", approvedDoc["status"])
	}
	if approvedDoc["docstatus"].(float64) != 1 {
		t.Fatalf("expected docstatus 1, got %v", approvedDoc["docstatus"])
	}

	// Verify timeline comment was added for workflow transitions
	r = x.call("GET", "/api/comments/Artigo/"+artigoName, nil, editor)
	x.expect(r, 200, "")
	comments := r.Body["data"].([]any)
	if len(comments) < 2 {
		t.Fatalf("expected at least 2 workflow comments, got %d", len(comments))
	}

	// Auditor cannot execute transitions
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Approve",
	}, auditor)
	x.expect(r, 417, "ValidationError") // State is already Approved, no transition

	// Doctype without workflow: Pessoa
	r = x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Fulano"}, autor)
	x.expect(r, 200, "")
	r = x.call("GET", "/api/resource/Pessoa/Fulano", nil, autor)
	x.expect(r, 200, "")
	pessoaDoc := r.Body["data"].(map[string]any)
	if _, hasWf := pessoaDoc["_workflow"]; hasWf {
		t.Fatalf("expected Pessoa not to have _workflow, got %v", pessoaDoc["_workflow"])
	}
}

func TestWorkflowAPI_SelfApprovalAndConditions(t *testing.T) {
	wf := setupWorkflowEnv(t)
	x, autoreditor, editor := wf.x, wf.autoreditor, wf.editor

	// 1. Self-approval enforcement over HTTP
	// autoreditor owns the document and has Editor role, but allowSelfApproval is false for Approve
	r := x.call("POST", "/api/resource/Artigo", map[string]any{
		"titulo":   "Artigo Próprio",
		"conteudo": "Conteudo longo para aprovar",
	}, autoreditor)
	x.expect(r, 200, "")
	artigoName := fmt.Sprint(r.Body["data"].(map[string]any)["name"])

	// Transition to Pending Approval
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Submit for Approval",
	}, autoreditor)
	x.expect(r, 200, "")

	// autoreditor checks actions in Pending Approval:
	// Approve has allowSelfApproval: false, so it must NOT be available
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&name="+artigoName, nil, autoreditor)
	x.expect(r, 200, "")
	actions := r.Body["data"].(map[string]any)["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("expected only 1 action (Reject) for self-approver, got %v", actions)
	}
	if actions[0].(map[string]any)["action"] != "Reject" {
		t.Fatalf("expected Reject action, got %v", actions[0])
	}

	// autoreditor attempts to apply Approve -> 403 (PermissionError)
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Approve",
	}, autoreditor)
	x.expect(r, 403, "PermissionError")

	// 2. Condition evaluation over HTTP
	// Create an article with short content (length <= 5)
	r = x.call("POST", "/api/resource/Artigo", map[string]any{
		"titulo":   "Artigo Curto",
		"conteudo": "1234",
	}, autoreditor)
	x.expect(r, 200, "")
	shortName := fmt.Sprint(r.Body["data"].(map[string]any)["name"])

	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    shortName,
		"action":  "Submit for Approval",
	}, autoreditor)
	x.expect(r, 200, "")

	// Editor (not owner) checks actions: condition for Approve fails (length <= 5), so only Reject is available
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&name="+shortName, nil, editor)
	x.expect(r, 200, "")
	editorActs := r.Body["data"].(map[string]any)["actions"].([]any)
	if len(editorActs) != 1 || editorActs[0].(map[string]any)["action"] != "Reject" {
		t.Fatalf("expected only Reject when condition fails, got %v", editorActs)
	}

	// Editor attempts to apply Approve -> 417 (ValidationError: condition not met)
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    shortName,
		"action":  "Approve",
	}, editor)
	x.expect(r, 417, "ValidationError")

	// Editor applies Reject -> 200 OK
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    shortName,
		"action":  "Reject",
	}, editor)
	x.expect(r, 200, "")
	rejectedDoc := r.Body["data"].(map[string]any)
	if rejectedDoc["workflow_state"] != "Rejected" {
		t.Fatalf("expected workflow_state Rejected, got %v", rejectedDoc["workflow_state"])
	}
	if rejectedDoc["docstatus"].(float64) != 0 {
		t.Fatalf("expected docstatus 0, got %v", rejectedDoc["docstatus"])
	}
}

func TestWorkflowAPI_PermissionAndLifecycleGuards(t *testing.T) {
	wf := setupWorkflowEnv(t)
	x, autor, editor, ze := wf.x, wf.autor, wf.editor, wf.unauthorized

	// Autor creates Artigo
	r := x.call("POST", "/api/resource/Artigo", map[string]any{
		"titulo":   "Artigo Secreto",
		"conteudo": "Conteudo interessante",
	}, autor)
	x.expect(r, 200, "")
	artigoName := fmt.Sprint(r.Body["data"].(map[string]any)["name"])

	// 1. Ze (no read permission on Artigo) tries to read actions or apply transition -> 403
	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&name="+artigoName, nil, ze)
	x.expect(r, 403, "PermissionError")

	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Submit for Approval",
	}, ze)
	x.expect(r, 403, "PermissionError")

	// 2. Direct submit guard: /api/resource/Artigo/{name}/submit is forbidden
	r = x.call("POST", "/api/resource/Artigo/"+artigoName+"/submit", nil, editor)
	x.expect(r, 417, "ValidationError")

	// 3. Manual modification of workflow state field is forbidden
	r = x.call("PUT", "/api/resource/Artigo/"+artigoName, map[string]any{
		"workflow_state": "Approved",
	}, autor)
	x.expect(r, 417, "ValidationError")

	// 4. State allowEdit guard
	// Transition to Pending Approval (allowEdit: "Editor")
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Artigo",
		"name":    artigoName,
		"action":  "Submit for Approval",
	}, autor)
	x.expect(r, 200, "")

	// Autor tries to edit fields while in Pending Approval -> 403 (allowEdit is Editor)
	r = x.call("PUT", "/api/resource/Artigo/"+artigoName, map[string]any{
		"conteudo": "Nova tentativa de edicao",
	}, autor)
	x.expect(r, 403, "PermissionError")

	// Editor CAN edit fields while in Pending Approval -> 200 OK
	r = x.call("PUT", "/api/resource/Artigo/"+artigoName, map[string]any{
		"conteudo": "Edicao pelo editor permitida",
	}, editor)
	x.expect(r, 200, "")

	// 5. Apply on doctype without workflow -> 417
	r = x.call("POST", "/api/workflow/apply", map[string]any{
		"doctype": "Pessoa",
		"name":    "Ninguem",
		"action":  "Approve",
	}, autor)
	x.expect(r, 404, "DoesNotExistError") // requireDocRead fails first if not found
}
