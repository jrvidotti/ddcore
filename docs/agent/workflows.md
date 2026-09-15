# Declarative approval workflows

An app declares approval workflows in `workflows/*.workflow.ts` using `defineWorkflow` from `@ddcore/sdk`. The framework enforces transitions on the server, guards against bypasses, manages PostgreSQL row locking (`FOR UPDATE`) for concurrent safety, records administrative audit logs (`tab_audit_event`) and document timeline comments (`tab_comment`), and provides Desk integration with state indicators and action buttons.

## Defining a workflow

```ts
import { defineWorkflow } from "@ddcore/sdk";

export default defineWorkflow({
  name: "Order Approval",
  doctype: "Order",
  stateField: "workflow_state", // Defaults to "workflow_state", configurable
  initialState: "Draft",
  states: [
    { state: "Draft", docstatus: 0, allowEdit: "Sales User" },
    { state: "Pending Approval", docstatus: 0, allowEdit: "Sales Manager" },
    { state: "Approved", docstatus: 1, allowEdit: "Sales Manager", updateFields: { status: "Approved" } },
    { state: "Rejected", docstatus: 2, updateFields: { status: "Rejected" } },
  ],
  transitions: [
    {
      state: "Draft",
      action: "Submit for Approval",
      nextState: "Pending Approval",
      allowed: ["Sales User", "Sales Manager"],
    },
    {
      state: "Pending Approval",
      action: "Approve",
      nextState: "Approved",
      allowed: "Sales Manager",
      allowSelfApproval: false,
      condition: (doc) => doc.total > 0,
    },
    {
      state: "Pending Approval",
      action: "Reject",
      nextState: "Rejected",
      allowed: "Sales Manager",
    },
  ],
});
```

### Top-level properties

- `name`: Unique name for the workflow definition.
- `doctype`: Target DocType governed by the workflow. Each DocType may have at most one active workflow.
- `stateField`: The field on the DocType storing the active state name. Defaults to `"workflow_state"` if omitted.
- `initialState`: The initial state assigned to newly inserted documents. Must exist in `states`.
- `states`: Non-empty list of state definitions (`WorkflowStateDef`).
- `transitions`: Non-empty list of transition definitions (`WorkflowTransitionDef`).

### State declarations (`states`)

Each state defines:
- `state`: String identifier of the state (e.g. `"Draft"`, `"Pending Approval"`, `"Approved"`).
- `docstatus`: Integer document lifecycle status bound to the state:
  - `0`: Draft / In-progress
  - `1`: Submitted (invokes `beforeSubmit` and `onSubmit` hooks upon transition)
  - `2`: Cancelled (invokes `beforeCancel` and `onCancel` hooks upon transition)
- `allowEdit`: Optional role name. When specified, only users with this role (or `Administrator`) can modify document fields while in this state. If omitted, normal DocType role permissions apply.
- `updateFields`: Optional map of field values to set atomically when the document enters this state (e.g. `{ status: "Approved" }`).

### Transition declarations (`transitions`)

Each transition defines:
- `state`: Origin state name. Must exist in `states`.
- `action`: Name of the user action triggering the transition (e.g. `"Submit for Approval"`, `"Approve"`, `"Reject"`).
- `nextState`: Destination state name. Must exist in `states`.
- `allowed`: Role name or array of role names authorized to trigger this transition. Checked against the authenticated caller's roles. `Administrator` always passes.
- `allowSelfApproval`: Optional boolean (default: `true`). If `false`, the user who created the document (`owner`) cannot trigger this transition, even if they possess the authorized role.
- `condition`: Optional synchronous JavaScript function `(doc) => boolean` running on goja. Evaluated against the document before applying the transition. If it returns `false`, the transition is rejected. Never use `async`/`await` or Promises.

## Server-side enforcement and bypass prevention

The ddcore engine strictly protects workflow integrity at the database layer:

1. **Auto-initialization:**
   When inserting a new document whose DocType has an active workflow, if `doc[stateField]` is empty or omitted, it is automatically initialized to `initialState`. Inserting a document with an explicit state other than `initialState` is rejected with `ValidationError` (unless running with `c.IgnorePermissions()`).

2. **Direct State Field Mutation Guard:**
   In `SaveDoc`, any direct modification where `doc[stateField] != before[stateField]` without executing a workflow transition is rejected with `ValidationError`. Client code or form scripts cannot alter the state field directly.

3. **Direct Submit / Cancel Guard:**
   Directly calling `c.Submit()` or `c.Cancel()` (or sending `docstatus: 1` or `docstatus: 2` via standard save/insert endpoints) on a workflow-controlled document is rejected with `ValidationError`. Document submission and cancellation must occur exclusively through workflow transitions bound to states with `docstatus: 1` or `docstatus: 2`.

4. **State Editability Guard (`allowEdit`):**
   When a document is in a state with `allowEdit: "Role"`, any user lacking that role (other than `Administrator` or system contexts with `c.IgnorePermissions()`) who attempts to save field modifications is rejected with `PermissionError`.

5. **Atomic Row Locking (`FOR UPDATE`):**
   Transitions execute under a PostgreSQL row lock (`SELECT ... FOR UPDATE`). Concurrent approval requests serialize safely: the first caller transitions the state, and the subsequent caller reads the updated state, encounters an invalid transition from the new state, and fails cleanly with `ValidationError` without duplicating side effects.

## Audit trail and timeline comments

Every workflow transition generates verifiable audit and history records:

- **Administrative Audit Ledger (`tab_audit_event`):**
  - Successful transitions record `action: "workflow.transition"` and `outcome: "Allowed"` on the caller's transaction with `detail: { "from_state": ..., "to_state": ..., "action": ... }`.
  - Refused transition attempts (unauthorized role, self-approval violation, or failing condition) write `action: "workflow.transition"` and `outcome: "Denied"` directly to the connection pool via `c.AuditDenied`.
- **Document Timeline (`tab_comment`):**
  - A timeline comment with `comment_type: "Workflow"` is posted to the document:
    `"{user} applied action '{action}' ({from_state} → {to_state})"`.
  - Visible in the Desk form view timeline.

## HTTP API

### Apply transition

`POST /api/workflow/apply`

Executes a workflow action on a document. Requires read permission on the document and the authorized role for the transition.

Request:
```json
{
  "doctype": "Order",
  "name": "ORD-0001",
  "action": "Approve"
}
```

Response:
```json
{
  "data": {
    "name": "ORD-0001",
    "workflow_state": "Approved",
    "docstatus": 1,
    "status": "Approved",
    "_workflow": {
      "state": "Approved",
      "actions": []
    }
  }
}
```

### List available actions

`GET /api/workflow/actions?doctype=Order&name=ORD-0001`

Returns the currently available workflow actions for the authenticated user, taking into account current state, caller roles, self-approval rules, and condition functions.

Response:
```json
{
  "data": [
    { "action": "Approve", "nextState": "Approved" },
    { "action": "Reject", "nextState": "Rejected" }
  ]
}
```

### Integrated Document Resource

`GET /api/resource/{doctype}/{name}`

When fetching a document whose DocType has an active workflow, the response automatically includes a `_workflow` envelope containing the current state and available actions for the caller:

```json
{
  "data": {
    "name": "ORD-0001",
    "workflow_state": "Pending Approval",
    "docstatus": 0,
    "_workflow": {
      "state": "Pending Approval",
      "actions": [
        { "action": "Approve", "nextState": "Approved" },
        { "action": "Reject", "nextState": "Rejected" }
      ]
    }
  }
}
```

This avoids extra HTTP round-trips when loading forms in the Desk.

## Desk integration

The ddcore Desk automatically provides workflow UI on governed DocTypes:

1. **Workflow State Badge:**
   The form view header displays an indicator badge showing the current workflow state (e.g. `Draft`, `Pending Approval`, `Approved`), styled using standard status colors.

2. **Workflow Action Buttons:**
   - If available actions exist (and the document is saved), buttons render in the header.
   - If up to 2 actions are available, they render as standalone action buttons (the first styled as primary).
   - If more than 2 actions are available, they render inside an **Actions** dropdown menu.
   - Clicking an action prompts for user confirmation before calling `/api/workflow/apply`.
   - Manual "Submit" and "Cancel" buttons are hidden when a workflow controls the DocType.

3. **Field and Save Locking:**
   If the current user does not hold the role specified by the active state's `allowEdit`, form controls switch to read-only and the Save button is suppressed.
