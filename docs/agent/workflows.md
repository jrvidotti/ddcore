# Declarative approval workflows

An app declares approval workflows in `workflows/*.workflow.ts` using `defineWorkflow` from `@ddcore/sdk`. The framework enforces transitions on the server, refuses the insert, delete and direct-write paths that would bypass a workflow, locks the row (`FOR UPDATE`) during a transition, records administrative audit events (`tab_audit_event`) and document timeline comments (`tab_comment`), and provides Desk integration with state indicators and action buttons.

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
- `docstatus`: Optional document lifecycle status bound to the state; defaults to `0` when omitted.
  - `0`: Draft / In-progress
  - `1`: Submitted (invokes `beforeSubmit` and `onSubmit` hooks when a transition changes docstatus to 1)
  - `2`: Cancelled (invokes `beforeCancel` and `onCancel` hooks when a transition changes docstatus to 2)
  A state with `docstatus: 1` or `2` requires the DocType to be `submittable`; the load-time check below rejects the workflow otherwise.
- `allowEdit`: Optional role name. When specified, only users with this role (or `Administrator`) can modify document fields while in this state. If omitted, normal DocType role permissions apply.
- `updateFields`: Optional map of field values to set atomically when the document enters this state (e.g. `{ status: "Approved" }`).
- State and action names are translation keys: the Desk shows both through `__()`, and `ddcore i18n extract` collects them into the app's translation catalogue.

### Transition declarations (`transitions`)

Each transition defines:
- `state`: Origin state name. Must exist in `states`.
- `action`: Name of the user action triggering the transition (e.g. `"Submit for Approval"`, `"Approve"`, `"Reject"`).
- `nextState`: Destination state name. Must exist in `states`.
- `allowed`: Role name or array of role names authorized to trigger this transition. Checked against the authenticated caller's roles. `Administrator` always passes.
- `allowSelfApproval`: Optional boolean (default: `true`). If `false`, the user who created the document (`owner`) cannot trigger this transition, even if they possess the authorized role. Running with `ignorePermissions` (a background job, a patch) skips the role check but not this one — a non-`Administrator` owner still cannot self-approve while `ignorePermissions` is raised. `Administrator` is exempt from self-approval through its own check (any `Administrator` action is allowed, whether or not `ignorePermissions` is set), not because `ignorePermissions` happens to be set.
- `condition`: Optional synchronous JavaScript function `(doc) => boolean` running on goja. `doc` is a plain JSON object (the document's field values), not a `Document` instance — it has no methods. Evaluated before applying the transition; if it returns `false`, the transition is rejected. Never use `async`/`await` or Promises. A condition that throws hides the action from `GET /api/workflow/actions`'s list without failing the request; applying that action returns the thrown error to the caller without writing a `Denied` audit entry (unlike a wrong role, a self-approval violation, or a condition that returns `false`, which do write one).

Workflows are linear state machines: `ApplyWorkflowTransition` matches the first transition whose `state` and `action` fit the current state and stops there, without checking its `condition`. Declaring two transitions from the same state with the same `action` name is a modeling error — the second is unreachable — so keep `action` unique per `state`.

## Server-side enforcement and bypass prevention

A workflow's guards run in `Insert`, `Delete`, `DBSet` and `SaveDoc`. The insert, delete and
direct state/docstatus mutation guards (1-3 below) are not lifted by `ignorePermissions`,
whether passed as an option (`doc.insert({ ignorePermissions: true })`,
`ddcore.deleteDoc(doctype, name, { ignorePermissions: true })`) or already raised on the
context, which every enqueued job and scheduled method runs with. They yield only to an
in-progress workflow transition, exactly like the state field and docstatus checks `SaveDoc`
already enforces for a plain save: a workflow document cannot be submitted or cancelled "not
even with ignorePermissions" (see below), and the same now holds for inserting, deleting and
`db.setValue`-ing one. A patch that genuinely needs to repair workflow data (a bad migration, a
one-off correction) writes SQL directly through `ctx.sql` in a `patches/*.ts` file, bypassing the
document API rather than asking it for an exemption. The state editability guard (4) and row
locking (5) are unrelated to that escape and keep their own rules, described below; the
load-time checks (6) always run regardless.

1. **Insert guard:**
   Inserting a document whose DocType has an active workflow initializes `doc[stateField]` to `initialState` when it is empty or omitted. An explicit state other than `initialState`, or a `docstatus` other than the initial state's, is rejected with a `ValidationError`. This closes the path where `POST /api/resource/<DocType> {docstatus: 1}` (or `doc.insert()` on a document built with `docstatus: 1`) would create a submitted document still sitting in the initial state.

2. **Delete guard:**
   Deleting a document governed by a workflow is refused with a `PermissionError` unless its current state has `docstatus: 0` and either the state is `initialState` or the user's roles satisfy that state's `allowEdit` (the same check `HasPermission(..., "write")` applies; a state with no `allowEdit` restriction allows the delete, same as it allows a plain field edit). A document with `docstatus: 1` still hits the existing "cancel before deleting" rule, and one with `docstatus: 2` follows the delete permission the DocType already grants — this guard only narrows drafts that have left `initialState`. Without it, a document's owner could delete it while it awaited approval, discarding the pending decision. `Administrator` is exempt outright, as it is from the DocType's own delete permission.

3. **Direct state field / docstatus mutation guard:**
   `DBSet` and `SaveDoc` both refuse a write to `doc[stateField]` or `docstatus` outside a workflow transition (`inWorkflowTransition`), with a `ValidationError`. Client code, form scripts and `ddcore.db.setValue` cannot alter either field directly; only `doc.applyWorkflow(action)` / `POST /api/workflow/apply` can.

4. **State editability guard (`allowEdit`):**
   When a document is in a state with `allowEdit: "Role"`, a user lacking that role (other than `Administrator`, or a context running with `c.IgnorePermissions()`) fails the DocType's `write` permission check for that document, so a plain field save is rejected with a `PermissionError`. Unlike guards 1-3, this one is a permission check like any other, so it follows the DocType's usual `ignorePermissions` rules.

5. **Atomic row locking (`FOR UPDATE`):**
   Transitions execute under a PostgreSQL row lock (`SELECT ... FOR UPDATE`). Concurrent approval requests serialize: the first caller transitions the state, and the subsequent caller reads the updated state, finds no transition matching the old state and action, and fails with a `ValidationError` without duplicating side effects.

6. **Load-time validation:**
   Engine reload (`Engine.Load`) validates every workflow against the current DocType registry and refuses to load definitions that fail:
   - the workflow's `doctype` must exist;
   - `stateField` must be a field of that DocType;
   - every state's `docstatus` must be `0`, `1` or `2`;
   - a state with `docstatus: 1` or `2` requires the DocType to be `submittable`;
   - a transition cannot go from a `docstatus: 1` state to a `docstatus: 0` state, and cannot leave a `docstatus: 2` state — `SaveDoc` always rejects those transitions, so a workflow declaring one could never execute it.

   A hot reload that fails validation keeps the previously loaded definitions; check the server log for the rejection reason.

## Server-side transitions

A document governed by a workflow cannot be submitted or cancelled with `doc.submit()` /
`doc.cancel()`, not even with `ignorePermissions`. App code — tests, fixtures, scheduled jobs,
patches — applies the action instead:

```ts
const order = ddcore.getDoc<Order>("Order", "ORD-0001");
order.applyWorkflow("Approve"); // reloads `order` with the new state and docstatus
```

`doc.applyWorkflow(action)` runs the same transition as `POST /api/workflow/apply`: the caller's
roles, `allowSelfApproval` and `condition` are checked, the row is locked, the transition runs
through the full save lifecycle (`validate`, `beforeSave`, `onUpdate` or `onUpdateAfterSubmit`,
`beforeSubmit`/`onSubmit` or `beforeCancel`/`onCancel` when docstatus changes, webhooks,
notifications), and the audit event and timeline comment are written. The document must already
be saved. `Administrator` passes every role and self-approval check, which is what a test running
as `Administrator` relies on.

A transition requires read access to the document and a role listed in the matched transition's
`allowed` — it does not require the DocType's `submit`, `cancel` or `write` permission (the save
lifecycle skips that check while `inWorkflowTransition` is set).

## Amending

Amending a cancelled document clears the state field on the copy, so the amendment starts over at
`initialState` when it is inserted.

## Audit trail and timeline comments

Every workflow transition writes an audit event and a timeline comment:

- **Administrative Audit Ledger (`tab_audit_event`):**
  - Successful transitions record `action: "workflow.transition"` and `outcome: "Allowed"` on the caller's transaction with `detail: { "from_state": ..., "to_state": ..., "action": ... }`.
  - Refused transition attempts (unauthorized role, self-approval violation, or failing condition) write `action: "workflow.transition"` and `outcome: "Denied"` directly to the connection pool via `c.AuditDenied`.
- **Document Timeline (`tab_comment`):**
  - A timeline comment with `comment_type: "Workflow"` is posted to the document, built from the
    translation key `"{0} applied action '{1}' ({2} → {3})"` with the acting user's id and the
    translated action, from-state and to-state names. Translation uses the acting user's own
    language (`c.Lang`) at the moment the transition runs, so the comment is stored already
    translated into the approver's language, not the reader's — a reader in a different language
    sees the approver's wording, not their own.
  - The comment insert runs inside a savepoint; a failure there is rolled back and logged
    (`workflow timeline comment failed`), and the transition still commits — a broken comment
    never undoes an approval.
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
      "allowEdit": true,
      "actions": []
    }
  }
}
```

### List available actions

`GET /api/workflow/actions?doctype=Order&name=ORD-0001`

Returns the current state, whether the authenticated caller may edit fields in it, and the workflow actions available to them, taking into account caller roles, self-approval rules, and condition functions.

Response:
```json
{
  "data": {
    "state": "Pending Approval",
    "allowEdit": false,
    "actions": [
      { "action": "Approve", "nextState": "Approved" },
      { "action": "Reject", "nextState": "Rejected" }
    ]
  }
}
```

On a DocType with no active workflow, the endpoint still requires read access to the document and
returns `{"state": "", "allowEdit": true, "actions": []}`.

### Integrated Document Resource

`GET /api/resource/{doctype}/{name}`

When fetching, creating, updating, or applying a transition on a document whose DocType has an active workflow, the response automatically includes a `_workflow` envelope containing the current state, whether the caller may edit fields in it (`allowEdit`), and the actions available to them:

```json
{
  "data": {
    "name": "ORD-0001",
    "workflow_state": "Pending Approval",
    "docstatus": 0,
    "_workflow": {
      "state": "Pending Approval",
      "allowEdit": false,
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
   - Clicking an action prompts for user confirmation (the action name is translated) before
     calling `/api/workflow/apply`.
   - Buttons are disabled while the form is saving or has unsaved edits — applying an action
     reloads the document, which would otherwise drop those edits silently.
   - Manual "Submit" and "Cancel" buttons are hidden when a workflow controls the DocType.

3. **Field and Save Locking:**
   If the current user does not hold the role specified by the active state's `allowEdit`, form controls switch to read-only and the Save button is suppressed.
