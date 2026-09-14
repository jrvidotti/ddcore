# Design: Assignments and Pending Work (OPS-05)

Record of the design established on 2026-09-13. The public developer contract is documented in [`docs/agent/assignments.md`](../../agent/assignments.md); this document captures **why** each component is designed this way.

## Starting point

While `Comment` and `Version` existed in Core, team task coordination was repeatedly implemented within individual app domain models (e.g. ad-hoc `assignee` fields on `Task` or `Project` in test fixtures). The roadmap (Stage 1, OPS-05) called for a generic, reusable assignment and pending work capability:
1. Operations to assign, revoke, and complete assignments with due dates and priorities.
2. A unified "My pending work" view across documents and personal tasks.
3. Reuse of persistent notifications (OPS-03) for assignment alerts and due date reminders.
4. **Strict authorization boundary**: Assignment must never grant document access; if a user lacks read access to a document (or has it revoked), the assignment and referenced document must not be exposed.

## Decisions

**1. `ToDo` is a Core DocType, not an internal table.**
Following Frappe conventions and matching `Comment` and `Version`, `ToDo` lives in `core/doctypes/todo/`. It supports both document-linked assignments (`reference_type` and `reference_name` populated) and standalone personal tasks. This allows standard report, list, and SDK interactions while remaining a first-class framework entity.

**2. Hard authorization: Assignment does not grant document access.**
A ToDo referencing a document must never bypass the document's permission model:
- `todo.controller.ts`: In `hasPermission`, if `reference_type` and `reference_name` are set, the controller explicitly calls `canReadReference(doc.reference_type, doc.reference_name, user)` using `ddcore.hasPermission(reference_type, "read", ...)`. If false or if the document does not exist, access is denied.
- `GET /api/todo/pending`: When returning the user's pending tasks, the endpoint re-evaluates read permissions for each referenced document. Items referencing documents the user cannot read are **filtered out**, preventing information leakage.
- Direct document read (`/api/resource/{doctype}/{name}`) continues to enforce standard document permissions, returning `403 PermissionError` if the user lacks access, regardless of any assigned ToDo.

**3. Dedicated assignment workflow endpoints.**
While `ToDo` is a DocType, creating an assignment on a document involves atomic side-effects:
- `POST /api/assignments/assign`: Validates caller permissions on the target document, inserts the `tab_todo` record, inserts a timeline `Comment` (`comment_type: "Workflow"`) on the referenced document, and queues a persistent notification for the assignee.
- `POST /api/assignments/complete`: Updates status to `Closed` and inserts a timeline completion comment.
- `POST /api/assignments/revoke`: Updates status to `Cancelled` and logs a revocation comment.
- `GET /api/assignments/{doctype}/{name}`: Returns assignments for a document, protected by `requireDocRead`.

**4. Renaming and deletion cascades.**
Registered `{table: "tab_todo", doctypeCol: "reference_type", nameCol: "reference_name"}` in `coreRefs` in `internal/engine/rename.go`. When a document is renamed, its ToDos are automatically re-pointed. When a document is deleted, its assignments are deleted. In addition, `"ToDo": {"reference_type", "reference_name"}` is registered in `referenceFields` in `internal/api/api.go` for `referenceGuard`.

**5. Notifications and due date reminders reuse OPS-03.**
- On assignment: If `allocated_to != assigned_by`, the framework records a persistent notification in `ddcore_notification` on the caller's transaction and emits a recipient-only `notifications_changed` SSE event.
- Due date reminders: Reuses the persistent notification date sweep. A registered date rule on `ToDo` (`date: { field: "date", days: 0 }`, `condition: (doc) => doc.status === "Open"`) triggers on the due date. The 5-minute scheduler sweep evaluates this in the site's timezone and deduplicates via `ddcore_notification_due`.

**6. Desk integration: Form sidebar widget and central `/app/todo`.**
- `DocSidebar.svelte`: Displays an "Assigned To" section with assignee badges, due dates, priority pills, complete/revoke buttons, and an "+ Assign" dialog.
- `/app/todo`: A dedicated pending work view with filter tabs for "Pending" (open tasks assigned to me), "Assigned by me", and "Completed", with inline toggle for completion and direct links to referenced documents.

## Left out

Visual assignment rule automation (OPS-09), auto-repeat, round-robin assignments, and team/role-based group queues. These belong to the demand-driven backlog (OPS-09) when demonstrated app requirements arise.
