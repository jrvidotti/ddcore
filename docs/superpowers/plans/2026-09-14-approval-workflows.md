# Declarative Approval Workflows (OPS-04) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement declarative approval workflows (OPS-04) in ddcore, enabling file-based workflow definitions (`defineWorkflow`), server-side bypass prevention, PostgreSQL row-level concurrency locking (`FOR UPDATE`), audit logging, timeline comments, and Desk UI action buttons.

**Architecture:** Workflows are authored via `defineWorkflow` in `<app>/workflows/*.workflow.ts` with synchronous TypeScript condition functions running in goja. The Go engine guards document lifecycles (blocking direct state edits and manual submit/cancel, enforcing state `allowEdit`), executes atomic transitions under row locks (`FOR UPDATE`), emits `tab_audit_event` and `tab_comment` records, exposes `/api/workflow/apply`, and renders workflow action buttons and state badges in the Desk.

**Tech Stack:** Go (1.24+), TypeScript, PostgreSQL (16+), Svelte 5 (Desk), goja JS runtime, Vitest.

**Spec:** [`docs/superpowers/specs/2026-09-14-approval-workflows-design.md`](file:///Users/junior/dev/ddcore/docs/superpowers/specs/2026-09-14-approval-workflows-design.md)

## Global Constraints

- Synchronous TypeScript on server: no `async`/`await` in workflow condition functions.
- English is the canonical language for all code, errors, and comments.
- File-based structural metadata: workflows are declared in code files, versioned in Git.
- Row-level concurrency locking (`FOR UPDATE`) for deterministic serialization of simultaneous approvals.
- All tests must pass: `go test ./...` and `npm test` in `desk/`.

---

### Task 1: TypeScript SDK & JS Runtime Workflow Declaration (`defineWorkflow`)

**Files:**
- Modify: `packages/sdk/src/types.ts`
- Modify: `packages/sdk/src/index.ts`
- Modify: `internal/js/prelude.js`
- Create: `internal/js/workflows.go`
- Create: `internal/js/workflow_test.go`

**Interfaces:**
- Consumes: `@ddcore/sdk`, `__ddcore.register`
- Produces: `defineWorkflow`, `js.Workflow`, `rt.EvaluateWorkflowCondition(wfName string, transitionIndex int, doc json.RawMessage) (bool, error)`

- [ ] **Step 1: Write failing unit tests for `defineWorkflow` and JS runtime metadata**

In `internal/js/workflow_test.go`:
```go
package js_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

func TestDefineWorkflow_RegistrationAndEvaluation(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "testapp")
	os.MkdirAll(filepath.Join(appDir, "workflows"), 0755)
	appDef := `import { defineApp } from "@ddcore/sdk"; export default defineApp({ name: "testapp" });`
	os.WriteFile(filepath.Join(appDir, "ddcore.app.ts"), []byte(appDef), 0644)

	wfDef := `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({
  name: "Order Approval",
  doctype: "Order",
  stateField: "workflow_state",
  initialState: "Draft",
  states: [
    { state: "Draft", docstatus: 0, allowEdit: "Sales User" },
    { state: "Pending Approval", docstatus: 0, allowEdit: "Sales Manager" },
    { state: "Approved", docstatus: 1, allowEdit: "Sales Manager", updateFields: { status: "Approved" } },
    { state: "Rejected", docstatus: 2, updateFields: { status: "Rejected" } },
  ],
  transitions: [
    { state: "Draft", action: "Submit for Approval", nextState: "Pending Approval", allowed: ["Sales User", "Sales Manager"] },
    { state: "Pending Approval", action: "Approve", nextState: "Approved", allowed: "Sales Manager", allowSelfApproval: false, condition: (doc) => doc.total > 100 },
    { state: "Pending Approval", action: "Reject", nextState: "Rejected", allowed: "Sales Manager" },
  ],
});`
	os.WriteFile(filepath.Join(appDir, "workflows/order.workflow.ts"), []byte(wfDef), 0644)

	bundle, err := js.BuildServerBundle(js.App{Name: "testapp", Dir: appDir}, false)
	if err != nil {
		t.Fatalf("BuildServerBundle failed: %v", err)
	}

	rt, err := js.New(bundle, nil)
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}

	snap := rt.Meta()
	if snap.Workflows == nil {
		t.Fatalf("expected snap.Workflows to be populated")
	}
	wf, ok := snap.Workflows["Order Approval"]
	if !ok {
		t.Fatalf("expected workflow 'Order Approval' in snap.Workflows")
	}
	if wf.Doctype != "Order" || wf.InitialState != "Draft" {
		t.Errorf("unexpected workflow meta: %+v", wf)
	}
	if len(wf.States) != 4 || len(wf.Transitions) != 3 {
		t.Errorf("expected 4 states and 3 transitions, got %d and %d", len(wf.States), len(wf.Transitions))
	}

	// Test condition evaluation: total <= 100 should be false
	docLow, _ := json.Marshal(map[string]any{"name": "ORD-1", "total": 50})
	res, err := rt.EvaluateWorkflowCondition("Order Approval", 1, docLow)
	if err != nil {
		t.Fatalf("EvaluateWorkflowCondition failed: %v", err)
	}
	if res {
		t.Errorf("expected condition to be false for total=50")
	}

	// Test condition evaluation: total > 100 should be true
	docHigh, _ := json.Marshal(map[string]any{"name": "ORD-1", "total": 150})
	resHigh, err := rt.EvaluateWorkflowCondition("Order Approval", 1, docHigh)
	if err != nil {
		t.Fatalf("EvaluateWorkflowCondition failed: %v", err)
	}
	if !resHigh {
		t.Errorf("expected condition to be true for total=150")
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -v ./internal/js -run TestDefineWorkflow`
Expected: FAIL (compilation error: unknown `defineWorkflow` and `EvaluateWorkflowCondition`)

- [ ] **Step 3: Implement SDK types and runtime support**

1. In `packages/sdk/src/types.ts`, append:
```typescript
export interface WorkflowStateDef {
  state: string;
  docstatus?: 0 | 1 | 2;
  allowEdit?: string;
  updateFields?: Record<string, any>;
}

export interface WorkflowTransitionDef<D = Record<string, any>> {
  state: string;
  action: string;
  nextState: string;
  allowed: string | string[];
  allowSelfApproval?: boolean;
  condition?: (doc: D) => boolean;
}

export interface WorkflowDef<D = Record<string, any>> {
  name: string;
  doctype: string;
  stateField?: string;
  initialState: string;
  states: WorkflowStateDef[];
  transitions: WorkflowTransitionDef<D>[];
}
```

2. In `packages/sdk/src/index.ts`, export `defineWorkflow`:
```typescript
export function defineWorkflow<D = Record<string, any>>(def: WorkflowDef<D>): WorkflowDef<D> {
  __ddcore.register("workflow", def);
  return def;
}
```

3. In `internal/js/prelude.js`:
- In `reg = { ... }`, add `workflows: {}, workflowsByDoctype: {}`.
- In `reg.register`:
```javascript
case "workflow": {
  const fail = (msg) => { throw new DDCoreError("ValidationError", "", "Workflow " + (value?.name || "<unnamed>") + ": " + msg); };
  if (!value || typeof value.name !== "string" || !value.name.trim()) fail("name is required");
  if (reg.workflows[value.name]) fail("name is defined twice");
  if (typeof value.doctype !== "string" || !value.doctype.trim()) fail("doctype is required");
  if (reg.workflowsByDoctype[value.doctype]) fail("DocType " + value.doctype + " already has a workflow defined");
  if (typeof value.initialState !== "string" || !value.initialState.trim()) fail("initialState is required");
  if (!Array.isArray(value.states) || value.states.length === 0) fail("states array is required");
  if (!Array.isArray(value.transitions) || value.transitions.length === 0) fail("transitions array is required");
  const stateNames = new Set(value.states.map((s) => s.state));
  if (!stateNames.has(value.initialState)) fail("initialState must exist in states");
  value.transitions.forEach((tr, i) => {
    if (!stateNames.has(tr.state)) fail("transition " + i + " state '" + tr.state + "' does not exist in states");
    if (!stateNames.has(tr.nextState)) fail("transition " + i + " nextState '" + tr.nextState + "' does not exist in states");
    if (!tr.action) fail("transition " + i + " action is required");
    if (!tr.allowed) fail("transition " + i + " allowed role is required");
    if (tr.condition !== undefined && typeof tr.condition !== "function") fail("transition " + i + " condition must be a function");
  });
  value.app = reg.app;
  value.sourceFile = reg.current;
  value.stateField = value.stateField || "workflow_state";
  reg.workflows[value.name] = value;
  reg.workflowsByDoctype[value.doctype] = value.name;
  break;
}
```
- In `reg.meta()`:
```javascript
const workflows = {};
for (const n in reg.workflows) {
  const wf = reg.workflows[n];
  workflows[n] = {
    name: wf.name,
    doctype: wf.doctype,
    stateField: wf.stateField || "workflow_state",
    initialState: wf.initialState,
    app: wf.app,
    sourceFile: wf.sourceFile,
    states: wf.states.map((s) => ({
      state: s.state,
      docstatus: s.docstatus !== undefined ? s.docstatus : 0,
      allowEdit: s.allowEdit || "",
      updateFields: s.updateFields || null,
    })),
    transitions: wf.transitions.map((t, idx) => ({
      index: idx,
      state: t.state,
      action: t.action,
      nextState: t.nextState,
      allowed: Array.isArray(t.allowed) ? t.allowed : [t.allowed],
      allowSelfApproval: t.allowSelfApproval !== false,
      hasCondition: typeof t.condition === "function",
    })),
  };
}
```
- Add `reg.evaluateWorkflowCondition`:
```javascript
reg.evaluateWorkflowCondition = function(name, transitionIndex, docJSON) {
  const wf = reg.workflows[name];
  if (!wf) throw new DDCoreError("ValidationError", "", "Unknown workflow: " + name);
  const tr = wf.transitions[transitionIndex];
  if (!tr) throw new DDCoreError("ValidationError", "", "Unknown transition index: " + transitionIndex);
  if (!tr.condition) return "true";
  const doc = JSON.parse(docJSON);
  const res = tr.condition(doc);
  return res ? "true" : "false";
};
```

4. Create `internal/js/workflows.go`:
Define `Workflow`, `WorkflowState`, `WorkflowTransition`, and `(rt *Runtime) EvaluateWorkflowCondition(wfName string, transitionIdx int, doc json.RawMessage) (bool, error)`.
Update `js.Snapshot` in `internal/js/runtime.go` to include `Workflows map[string]Workflow `json:"workflows"``.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/js -run TestDefineWorkflow`
Expected: PASS

- [ ] **Step 5: Commit Task 1**

```bash
git add packages/sdk/src/types.ts packages/sdk/src/index.ts internal/js/prelude.js internal/js/workflows.go internal/js/workflow_test.go internal/js/runtime.go
git commit -m "feat(js): add defineWorkflow support and runtime condition evaluation"
```

---

### Task 2: Engine State & DocType Lifecycle Guard Integration

**Files:**
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/doc.go`
- Modify: `internal/engine/perm.go`
- Create: `internal/engine/workflow_lifecycle_test.go`

**Interfaces:**
- Consumes: `js.Workflow`, `engine.Ctx`
- Produces: `c.WorkflowFor(doctype string) *js.Workflow`, lifecycle protection in `Insert`, `SaveDoc`, and `HasPermission`

- [ ] **Step 1: Write failing tests for workflow lifecycle guards**

In `internal/engine/workflow_lifecycle_test.go`:
```go
package engine_test

import (
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func TestWorkflow_LifecycleGuards(t *testing.T) {
	// Setup with a DocType having an active workflow
	// 1. Insert: uninitialized doc gets initialState
	// 2. Insert with invalid non-initial state: rejected
	// 3. SaveDoc: direct modification of stateField: rejected
	// 4. SaveDoc: direct submit (docstatus 1): rejected
	// 5. allowEdit: user without allowEdit role cannot edit fields
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -v ./internal/engine -run TestWorkflow_LifecycleGuards`
Expected: FAIL

- [ ] **Step 3: Implement lifecycle guards in engine**

1. In `internal/engine/engine.go`:
- In `Snapshot`, add `Workflows map[string]js.Workflow `json:"workflows"``.
- In `State`, add `Workflows map[string]*js.Workflow` and `WorkflowByDocType map[string]*js.Workflow`.
- Populate maps in `Load(...)`.
- Add helper on `(c *Ctx) WorkflowFor(doctype string) *js.Workflow`.

2. In `internal/engine/doc.go`:
- In `Insert`:
  - If `wf := c.WorkflowFor(d.Name); wf != nil`:
    - If `doc[wf.StateField] == nil || doc[wf.StateField] == ""`: `doc[wf.StateField] = wf.InitialState`.
    - Else if doc[wf.StateField] != wf.InitialState && !opts.IgnorePermissions && !c.IgnorePermissions():
      return nil, cerr.Validation("New {0} must start in initial workflow state '{1}'", d.Name, wf.InitialState)
- In `SaveDoc`:
  - If `wf := c.WorkflowFor(d.Name); wf != nil && !c.inWorkflowTransition`:
    - If `doc[wf.StateField] != before[wf.StateField]`:
      return nil, cerr.Validation("Cannot manually modify workflow state field '{0}'. Use workflow actions to transition.", wf.StateField)
    - If `before.Docstatus() != doc.Docstatus()`:
      return nil, cerr.Validation("Direct submit or cancel is disabled for documents governed by workflow '{0}'", wf.Name)
- In `internal/engine/perm.go`:
  - In `HasPermission` for `ptype == "write"`:
    - If `doc != nil`:
      - If `wf := c.WorkflowFor(doctype); wf != nil`:
        - Current state: `st := doc.Str(wf.StateField); if st == "" { st = wf.InitialState }`
        - If state has `allowEdit != ""` and `c.User != "Administrator"` and `!c.IgnorePermissions()`:
          - If `!c.HasRole(state.AllowEdit)` -> return `false, nil`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/engine -run TestWorkflow_LifecycleGuards`
Expected: PASS

- [ ] **Step 5: Commit Task 2**

```bash
git add internal/engine/engine.go internal/engine/doc.go internal/engine/perm.go internal/engine/workflow_lifecycle_test.go
git commit -m "feat(engine): add workflow lifecycle guards and state allowEdit enforcement"
```

---

### Task 3: Atomic Transition Engine & Concurrency Locking

**Files:**
- Create: `internal/engine/workflow.go`
- Create: `internal/engine/workflow_transition_test.go`

**Interfaces:**
- Consumes: `c.getDocForUpdate`, `c.Audit`, `c.AuditDenied`, `rt.EvaluateWorkflowCondition`
- Produces: `(c *Ctx) ApplyWorkflowTransition(doctype, name, action string) (Doc, error)`, `(c *Ctx) AvailableWorkflowActions(doctype string, doc Doc) ([]WorkflowAvailableAction, error)`

- [ ] **Step 1: Write failing tests for transitions, audit, timeline comments, and concurrency**

In `internal/engine/workflow_transition_test.go`:
```go
package engine_test

import (
	"sync"
	"testing"
)

func TestWorkflow_ApplyTransition_SuccessAndDocstatusBinding(t *testing.T) {
	// Tests:
	// - Transitioning Draft -> Pending Approval (docstatus 0)
	// - Transitioning Pending Approval -> Approved (docstatus 1, runs onSubmit, sets updateFields)
	// - Verification of tab_audit_event record
	// - Verification of tab_comment (comment_type: "Workflow")
}

func TestWorkflow_ApplyTransition_RoleAndConditionDenied(t *testing.T) {
	// Tests:
	// - User lacking allowed role gets PermissionError and tab_audit_event Denied record
	// - Condition returning false gets ValidationError and tab_audit_event Denied record
	// - Self approval restriction blocks doc owner
}

func TestWorkflow_ApplyTransition_Concurrency(t *testing.T) {
	// Tests:
	// - 2 concurrent goroutines trying to call ApplyWorkflowTransition("Approve") on the same doc
	// - Exactly 1 succeeds, 1 fails
	// - No duplicated comments or double submit
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -v ./internal/engine -run TestWorkflow_ApplyTransition`
Expected: FAIL

- [ ] **Step 3: Implement `ApplyWorkflowTransition` and `AvailableWorkflowActions`**

In `internal/engine/workflow.go`:
```go
package engine

type WorkflowAvailableAction struct {
	Action    string `json:"action"`
	NextState string `json:"nextState"`
}

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

func (c *Ctx) ApplyWorkflowTransition(doctype, name, action string) (Doc, error) {
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

	// Mark inWorkflowTransition on context so SaveDoc allows docstatus and state change
	c.inWorkflowTransition = true
	defer func() { c.inWorkflowTransition = false }()

	oldStatus := doc.Docstatus()
	newStatus := oldStatus
	if targetState != nil {
		newStatus = targetState.Docstatus
	}

	doc["docstatus"] = newStatus
	saved, err := c.SaveDoc(doc, SaveOpts{})
	if err != nil {
		return nil, err
	}

	// 7. Write Audit Event & Comment
	if err := c.Audit("workflow.transition", doctype, name, detail); err != nil {
		return nil, err
	}
	commentContent := fmt.Sprintf("%s applied action '%s' (%s → %s)", c.User, action, currentState, matched.NextState)
	c.NewDoc("Comment", Doc{
		"comment_type":   "Workflow",
		"reference_type": doctype,
		"reference_name": name,
		"content":        commentContent,
	}).Insert(InsertOpts{IgnorePermissions: true})

	return saved, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/engine -run TestWorkflow_ApplyTransition`
Expected: PASS

- [ ] **Step 5: Commit Task 3**

```bash
git add internal/engine/workflow.go internal/engine/workflow_transition_test.go
git commit -m "feat(engine): implement atomic workflow transition engine with row locking and audit"
```

---

### Task 4: HTTP API Endpoints & Integrated Doc Payload

**Files:**
- Create: `internal/api/workflow.go`
- Modify: `internal/api/api.go`
- Create: `internal/api/workflow_test.go`

**Interfaces:**
- Consumes: `c.ApplyWorkflowTransition`, `c.AvailableWorkflowActions`
- Produces: `POST /api/workflow/apply`, `GET /api/workflow/actions`, `_workflow` payload in `GET /api/resource`

- [ ] **Step 1: Write failing API tests**

In `internal/api/workflow_test.go`:
```go
package api_test

import (
	"testing"
)

func TestWorkflowAPI_ApplyAndActions(t *testing.T) {
	// Test POST /api/workflow/apply with valid action
	// Test GET /api/workflow/actions returns allowed actions
	// Test GET /api/resource/{doctype}/{name} contains _workflow
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test -v ./internal/api -run TestWorkflowAPI`
Expected: FAIL

- [ ] **Step 3: Implement API endpoints & handler wiring**

1. In `internal/api/workflow.go`:
- Implement `applyWorkflowTransitionHandler`:
  - Parses `{ doctype, name, action }` from body.
  - Checks `requireDocRead(c, doctype, name)`.
  - Calls `saved, err := c.ApplyWorkflowTransition(doctype, name, action)`.
  - Returns `200 OK` with JSON of `saved` plus `_workflow`.
- Implement `workflowActionsHandler`:
  - Reads `doctype` and `name` query params.
  - Checks `requireDocRead(c, doctype, name)`.
  - Calls `c.AvailableWorkflowActions(doctype, doc)`.
  - Returns `200 OK` with `{ state: currentState, actions: actions }`.

2. In `internal/api/api.go`:
- Register routes:
  - `mux.HandleFunc("POST /api/workflow/apply", s.handle(applyWorkflowTransitionHandler))`
  - `mux.HandleFunc("GET /api/workflow/actions", s.handle(workflowActionsHandler))`
- In `getResourceHandler` (when returning single document):
  - If `wf := c.WorkflowFor(doctype); wf != nil`:
    - Compute available actions using `c.AvailableWorkflowActions(doctype, doc)`.
    - Attach `doc["_workflow"] = map[string]any{"state": doc.Str(wf.StateField), "actions": actions}` before writing response.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/api -run TestWorkflowAPI`
Expected: PASS

- [ ] **Step 5: Commit Task 4**

```bash
git add internal/api/workflow.go internal/api/api.go internal/api/workflow_test.go
git commit -m "feat(api): add workflow apply and actions endpoints with integrated payload"
```

---

### Task 5: Desk FormView Integration

**Files:**
- Modify: `desk/src/lib/components/FormView.svelte`
- Modify: `desk/src/lib/form.svelte.ts`
- Create: `desk/src/lib/components/workflow.test.ts`

**Interfaces:**
- Consumes: `frm.doc._workflow`, `POST /api/workflow/apply`
- Produces: Workflow action buttons, state badge, read-only field locking

- [ ] **Step 1: Write failing desk test for workflow button rendering and read-only logic**

In `desk/src/lib/components/workflow.test.ts`:
```typescript
import { describe, it, expect } from "vitest";

describe("Workflow form integration", () => {
  it("determines available workflow actions from doc._workflow", () => {
    const doc = {
      name: "PED-1",
      doctype: "Pedido",
      _workflow: {
        state: "Pending Approval",
        actions: [{ action: "Approve", nextState: "Approved" }, { action: "Reject", nextState: "Rejected" }],
      },
    };
    expect(doc._workflow.actions).toHaveLength(2);
  });
});
```

- [ ] **Step 2: Run test to verify execution**

Run: `npm test -- desk/src/lib/components/workflow.test.ts` in `desk/`
Expected: PASS/FAIL depending on setup

- [ ] **Step 3: Implement Desk FormView & form logic**

1. In `desk/src/lib/form.svelte.ts`:
- Add getter `get workflow() { return this.doc._workflow; }`.
- Add `applyWorkflowAction(action: string)`:
  - Calls `api.post("/api/workflow/apply", { doctype: this.meta.doctype.name, name: this.doc.name, action })`.
  - Reloads document.
- In `readOnly` getter:
  - If document has `_workflow` and the workflow specifies that the current user cannot edit in this state (or `_workflow.allowEdit === false`), return `true`.

2. In `desk/src/lib/components/FormView.svelte`:
- If `frm.workflow`:
  - Show workflow state pill in the header.
  - In action buttons container:
    - If `frm.workflow.actions?.length > 0`:
      - Render button for each action (e.g. `[Approve]`, `[Reject]`).
      - Hide standard manual Submit button.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm test` in `desk/`
Expected: PASS

- [ ] **Step 5: Commit Task 5**

```bash
git add desk/src/lib/components/FormView.svelte desk/src/lib/form.svelte.ts desk/src/lib/components/workflow.test.ts
git commit -m "feat(desk): render workflow action buttons, state pill, and handle field locking"
```

---

### Task 6: Documentation, Testapp Workflow Fixture & Verification

**Files:**
- Create: `docs/agent/workflows.md`
- Modify: `docs/agent/index.md`
- Modify: `ROADMAP.md`
- Modify: `docs/frappe-port-inventory.md`
- Create: `apps/testapp/workflows/pedido_approval.workflow.ts`

**Interfaces:**
- Consumes: Complete OPS-04 capability
- Produces: Public documentation, verified testapp fixture, updated roadmap

- [ ] **Step 1: Create documentation in `docs/agent/workflows.md`**
Document:
- Declaring workflows via `defineWorkflow`.
- States, actions, roles, and conditions.
- Docstatus binding and updateFields.
- Server enforcement and bypass protection.
- Desk actions and timeline comments.
- Update `docs/agent/index.md` to link to `workflows.md`.

- [ ] **Step 2: Add testapp fixture workflow**
Create `apps/testapp/workflows/pedido_approval.workflow.ts` managing approval of `Pedido`.

- [ ] **Step 3: Update `ROADMAP.md` and `docs/frappe-port-inventory.md`**
Move OPS-04 from Stage 1 to "Available foundations" in `ROADMAP.md` and mark P1 delivered in `docs/frappe-port-inventory.md`.

- [ ] **Step 4: Run full verification suite**
Run:
```bash
./bin/ddcore types
make test
```
Expected: All Go tests and TypeScript desk tests PASS.

- [ ] **Step 5: Commit Task 6**

```bash
git add docs/agent/workflows.md docs/agent/index.md ROADMAP.md docs/frappe-port-inventory.md apps/testapp/workflows/pedido_approval.workflow.ts
git commit -m "docs: document approval workflows (OPS-04) and add testapp fixture"
```
