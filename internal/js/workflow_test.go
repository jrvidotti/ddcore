package js_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	raw, err := rt.Meta()
	if err != nil {
		t.Fatalf("rt.Meta failed: %v", err)
	}
	var snap js.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
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

	// Test transition without condition: transition 0 has no condition, should return true
	resDefault, err := rt.EvaluateWorkflowCondition("Order Approval", 0, docLow)
	if err != nil {
		t.Fatalf("EvaluateWorkflowCondition for transition 0 failed: %v", err)
	}
	if !resDefault {
		t.Errorf("expected condition to default to true when omitted")
	}
}

func workflowRuntime(t *testing.T, wfCode string) (*js.Runtime, error) {
	t.Helper()
	dir := t.TempDir()
	appDir := filepath.Join(dir, "testapp")
	os.MkdirAll(filepath.Join(appDir, "workflows"), 0755)
	appDef := `import { defineApp } from "@ddcore/sdk"; export default defineApp({ name: "testapp" });`
	os.WriteFile(filepath.Join(appDir, "ddcore.app.ts"), []byte(appDef), 0644)
	os.WriteFile(filepath.Join(appDir, "workflows/test.workflow.ts"), []byte(wfCode), 0644)

	bundle, err := js.BuildServerBundle(js.App{Name: "testapp", Dir: appDir}, false)
	if err != nil {
		return nil, err
	}
	return js.New(bundle, nil)
}

func TestDefineWorkflow_ValidationErrors(t *testing.T) {
	cases := []struct {
		name        string
		code        string
		errContains string
	}{
		{
			name: "missing name",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] } as any);`,
			errContains: "name is required",
		},
		{
			name: "missing doctype",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] } as any);`,
			errContains: "doctype is required",
		},
		{
			name: "missing initialState",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] } as any);`,
			errContains: "initialState is required",
		},
		{
			name: "initialState not in states",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "NonExistent", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] });`,
			errContains: "initialState must exist in states",
		},
		{
			name: "empty states array",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] });`,
			errContains: "states array is required",
		},
		{
			name: "empty transitions array",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [] });`,
			errContains: "transitions array is required",
		},
		{
			name: "transition state not in states",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Unknown", action: "A", nextState: "Draft", allowed: "Role"}] });`,
			errContains: "state 'Unknown' does not exist in states",
		},
		{
			name: "transition nextState not in states",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Unknown", allowed: "Role"}] });`,
			errContains: "nextState 'Unknown' does not exist in states",
		},
		{
			name: "transition missing action",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "", nextState: "Draft", allowed: "Role"}] });`,
			errContains: "action is required",
		},
		{
			name: "transition missing allowed",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: ""}] });`,
			errContains: "allowed role is required",
		},
		{
			name: "async condition rejected",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role", condition: async () => true}] });`,
			errContains: "condition must be a function",
		},
		{
			name: "non-function condition rejected",
			code: `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({ name: "W", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role", condition: "not-a-fn" as any}] });`,
			errContains: "condition must be a function",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := workflowRuntime(t, tc.code)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errContains)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("expected error containing %q, got %v", tc.errContains, err)
			}
		})
	}
}

func TestDefineWorkflow_DuplicateErrors(t *testing.T) {
	t.Run("duplicate workflow name", func(t *testing.T) {
		dir := t.TempDir()
		appDir := filepath.Join(dir, "testapp")
		os.MkdirAll(filepath.Join(appDir, "workflows"), 0755)
		appDef := `import { defineApp } from "@ddcore/sdk"; export default defineApp({ name: "testapp" });`
		os.WriteFile(filepath.Join(appDir, "ddcore.app.ts"), []byte(appDef), 0644)
		wfDef := `import { defineWorkflow } from "@ddcore/sdk";
defineWorkflow({ name: "W1", doctype: "Order1", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] });
defineWorkflow({ name: "W1", doctype: "Order2", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] });
`
		os.WriteFile(filepath.Join(appDir, "workflows/dup.workflow.ts"), []byte(wfDef), 0644)
		bundle, err := js.BuildServerBundle(js.App{Name: "testapp", Dir: appDir}, false)
		if err != nil {
			t.Fatal(err)
		}
		_, err = js.New(bundle, nil)
		if err == nil || !strings.Contains(err.Error(), "name is defined twice") {
			t.Fatalf("expected 'name is defined twice', got %v", err)
		}
	})

	t.Run("duplicate workflow for same DocType", func(t *testing.T) {
		dir := t.TempDir()
		appDir := filepath.Join(dir, "testapp")
		os.MkdirAll(filepath.Join(appDir, "workflows"), 0755)
		appDef := `import { defineApp } from "@ddcore/sdk"; export default defineApp({ name: "testapp" });`
		os.WriteFile(filepath.Join(appDir, "ddcore.app.ts"), []byte(appDef), 0644)
		wfDef := `import { defineWorkflow } from "@ddcore/sdk";
defineWorkflow({ name: "W1", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] });
defineWorkflow({ name: "W2", doctype: "Order", initialState: "Draft", states: [{state: "Draft"}], transitions: [{state: "Draft", action: "A", nextState: "Draft", allowed: "Role"}] });
`
		os.WriteFile(filepath.Join(appDir, "workflows/dup2.workflow.ts"), []byte(wfDef), 0644)
		bundle, err := js.BuildServerBundle(js.App{Name: "testapp", Dir: appDir}, false)
		if err != nil {
			t.Fatal(err)
		}
		_, err = js.New(bundle, nil)
		if err == nil || !strings.Contains(err.Error(), "DocType Order already has a workflow defined") {
			t.Fatalf("expected 'DocType Order already has a workflow defined', got %v", err)
		}
	})
}

func TestDefineWorkflow_ConditionEvaluationErrors(t *testing.T) {
	rt, err := workflowRuntime(t, `import { defineWorkflow } from "@ddcore/sdk";
export default defineWorkflow({
  name: "Order Workflow",
  doctype: "Order",
  initialState: "Draft",
  states: [{state: "Draft"}, {state: "Submitted"}],
  transitions: [
    { state: "Draft", action: "Submit", nextState: "Submitted", allowed: "Role", condition: (doc) => { if (doc.throw) throw new Error("bad condition"); return true; } },
  ],
});`)
	if err != nil {
		t.Fatal(err)
	}

	// Unknown workflow
	_, err = rt.EvaluateWorkflowCondition("Unknown Workflow", 0, []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "Unknown workflow") {
		t.Fatalf("expected unknown workflow error, got %v", err)
	}

	// Unknown transition index
	_, err = rt.EvaluateWorkflowCondition("Order Workflow", 99, []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "Unknown transition index") {
		t.Fatalf("expected unknown transition index error, got %v", err)
	}

	// Condition that throws
	_, err = rt.EvaluateWorkflowCondition("Order Workflow", 0, []byte(`{"throw": true}`))
	if err == nil || !strings.Contains(err.Error(), "bad condition") {
		t.Fatalf("expected error from thrown exception, got %v", err)
	}
}
