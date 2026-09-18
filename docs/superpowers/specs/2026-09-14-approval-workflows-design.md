# Design: Declarative Approval Workflows (OPS-04)

Record of the design established on 2026-09-14. The contract for app authors and operators
will be documented in [`docs/agent/workflows.md`](../../agent/workflows.md); this document preserves
**why** each piece is designed the way it is.

## Starting point

While `docstatus` (0: Draft, 1: Submitted, 2: Cancelled) and controller lifecycle hooks existed in ddcore,
approval flows across business entities (e.g. Purchase Orders, Contracts, Invoices) required repeated,
hand-written state and role transition machinery in app controllers.

The roadmap (Stage 1, OPS-04) called for a declarative approval workflow subsystem:
1. Declarative states, actions, roles, conditions, and docstatus integration.
2. Server-side enforcement preventing direct writes or `submit` from bypassing approval.
3. Row-level concurrency locking preventing simultaneous approvals from duplicating effects.
4. Tamper-evident administrative audit logging (`tab_audit_event`) and document timeline history (`tab_comment`).
5. Seamless Desk UI integration surfacing workflow state badges and available action buttons.

## Key Decisions

### 1. File-Based Declarative Authoring via `defineWorkflow`
Following ddcore's core architectural principle ("file-based structural metadata, versioned in Git"),
workflows are authored as TypeScript files under `<app>/workflows/<name>.workflow.ts` using
`defineWorkflow` from `@ddcore/sdk`:

```typescript
import { defineWorkflow } from "@ddcore/sdk";

export default defineWorkflow({
  name: "Order Approval",
  doctype: "Pedido",
  stateField: "workflow_state", // Defaults to "workflow_state", but configurable (e.g. "status")
  initialState: "Draft",
  states: [
    { state: "Draft", docstatus: 0, allowEdit: "Sales User" },
    { state: "Pending Approval", docstatus: 0, allowEdit: "Sales Manager" },
    { state: "Approved", docstatus: 1, allowEdit: "Sales Manager", updateFields: { status: "Approved" } },
    { state: "Rejected", docstatus: 2, updateFields: { status: "Rejected" } },
  ],
  transitions: [
    { state: "Draft", action: "Submit for Approval", nextState: "Pending Approval", allowed: ["Sales User", "Sales Manager"] },
    { state: "Pending Approval", action: "Approve", nextState: "Approved", allowed: "Sales Manager", allowSelfApproval: false, condition: (doc) => doc.total > 0 },
    { state: "Pending Approval", action: "Reject", nextState: "Rejected", allowed: "Sales Manager" },
  ],
});
```

**Rationale:**
- Preserves Git version control, branch management, and CI validation.
- Allows native, synchronous TypeScript condition functions `(doc) => boolean` running directly in goja without string-expression parsing overhead.
- Hot reloads instantly in development via `watch.Apps`.
- Metadata is serialized to Go (`meta.Workflow`) at boot, while condition functions remain callable via `rt.EvalWorkflowCondition`.

### 2. Strict Server Enforcement and Bypass Prevention
Direct client mutations must never bypass workflow rules:
- **Auto-initialization:** When inserting a document whose DocType has an active workflow, if `doc[stateField]` is empty or not provided, the engine sets it to `initialState`. Attempting to insert a document with a non-initial state is rejected (`ValidationError`) unless running with `c.IgnorePermissions()`.
- **Direct State Field Guard:** In `SaveDoc`, any mutation where `doc[stateField] != before[stateField]` without going through a workflow transition is rejected with `400 ValidationError`.
- **Direct Submit/Cancel Guard:** In `SaveDoc` and `SubmitDoc`/`CancelDoc`, calling submit (`docstatus: 1`) or cancel (`docstatus: 2`) directly on a document governed by an active workflow is rejected: transitions are the sole authority for advancing docstatus.
- **State Editability Guard (`allowEdit`):** When in a state specifying `allowEdit`, only users with that role (or `Admin` / `c.IgnorePermissions()`) are permitted to edit document fields. Other users attempting to save changes receive a `403 PermissionError`.

### 3. Atomic Transitions and Concurrency Protection (`FOR UPDATE`)
Transitions are executed atomically through `c.ApplyWorkflowTransition(doctype, name, action)`:
1. **Row Lock:** Acquires a row lock using PostgreSQL `FOR UPDATE` (`c.getDocForUpdate`).
2. **Transition Validation:** Reads current state; validates that a transition exists for `(current_state, action)`.
3. **Role & Self-Approval Checks:**
   - Verifies `c.User` holds at least one role in `transition.Allowed` (or `Admin`). Unauthorized attempts record `AuditDenied` and return `PermissionError`.
   - If `!transition.AllowSelfApproval` and `before.Str("owner") == c.User`, rejects with `PermissionError` (self-approval prohibited).
4. **Condition Evaluation:** If `transition.HasCondition`, runs `rt.EvalWorkflowCondition(wf.Name, transition.Index, before.JSON())`. If false, records `AuditDenied` and returns `ValidationError`.
5. **State & Docstatus Mutation:**
   - Sets `doc[stateField] = nextState.State`.
   - Applies any `nextState.UpdateFields`.
   - If `nextState.Docstatus != before.Docstatus()`:
     - Transitioning to 1 invokes `beforeSubmit`, sets `docstatus = 1`, and invokes `onSubmit`.
     - Transitioning to 2 invokes `beforeCancel`, sets `docstatus = 2`, and invokes `onCancel`.
6. **Concurrent Safety:** Because the row is locked with `FOR UPDATE`, concurrent approval calls are serialized. The first caller updates the state; the second caller unblocks, reads the new state, finds `action` invalid from the new state, and returns `400 ValidationError` without repeating side effects.

### 4. Audit Ledger and Timeline History
Every workflow transition leaves verifiable evidence across two complementary channels:
- **Administrative Audit Ledger (`tab_audit_event`):**
  - Allowed transitions write `action: "workflow.transition"`, `outcome: "Allowed"` on the caller's transaction with `detail: { from_state, to_state, action }`.
  - Refused attempts (unauthorized role, self-approval violation, or failing condition) write `outcome: "Denied"` directly to the connection pool via `c.AuditDenied`.
- **Document Timeline (`tab_comment`):**
  - Writes a timeline comment with `comment_type: "Workflow"` on the document:
    `"{user} applied action '{action}' ({from_state} → {to_state})"`.
  - Visible in Desk document history and form timeline.

### 5. API Endpoints and Document Integration
- **`POST /api/workflow/apply`:**
  - Body: `{ "doctype": string, "name": string, "action": string }`
  - Validates document read permission; calls `c.ApplyWorkflowTransition`; returns updated document.
- **`GET /api/workflow/actions`:**
  - Params: `?doctype=...&name=...`
  - Evaluates transitions available from current state for the caller (roles, self-approval, conditions).
- **Integrated Payload (`GET /api/resource/{doctype}/{name}`):**
  - When fetching a document governed by a workflow, the response automatically embeds:
    ```json
    {
      "_workflow": {
        "state": "Pending Approval",
        "actions": [
          { "action": "Approve", "nextState": "Approved" },
          { "action": "Reject", "nextState": "Rejected" }
        ]
      }
    }
    ```
  - Eliminates extra HTTP roundtrips when loading forms.

### 6. Desk UI Integration
- **Form Header:** FormView displays a workflow state badge prominently in the header.
- **Workflow Action Buttons:**
  - Available actions render as primary/secondary action buttons (or dropdown if > 2) at the top of `FormView.svelte`.
  - Clicking an action prompts for confirmation and calls `POST /api/workflow/apply`.
  - Replaces manual "Submit" and "Cancel" buttons.
- **Read-Only Locking:**
  - If the active state's `allowEdit` role is not held by the user, form fields become read-only and the "Save" button is hidden.

## Left Out

1. **Visual Workflow Designer:** App developers author workflows in TypeScript code. Visual flowchart editors in the Desk belong to the demand-driven backlog (DAT-09).
2. **Parallel / Branching Approval Joins (e.g. 2-of-3 approvers required):** Single active state per document. Complex quorum logic can be handled via child table counters or app controller logic when needed.
3. **Automatic Escalation / Timeouts:** Scheduled escalations can be orchestrated via existing background jobs and notifications (OPS-03).
