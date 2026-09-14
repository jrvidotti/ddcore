# Assignments and pending work

`ddcore` provides a built-in assignment workflow and task management system based on the standard `ToDo` DocType in the `Core` module. Teams can assign documents, track due dates, set priorities, receive notifications, and review personal pending work through the Desk central (`/app/todo`) and the document sidebar.

## The `ToDo` DocType

Assignments and personal tasks are stored in `tab_to_do` using standard document fields:

| Field | Type | Description |
| --- | --- | --- |
| `status` | Select | `Open`, `Closed`, or `Cancelled` (default: `Open`) |
| `priority` | Select | `Low`, `Medium`, `High`, or `Urgent` (default: `Medium`) |
| `date` | Date | Optional due date |
| `allocated_to` | Link (`User`) | Required assignee user ID |
| `assigned_by` | Link (`User`) | User who created the assignment |
| `description` | Small Text | Task description or instructions |
| `reference_type` | Data | Referenced DocType name (e.g. `Order`, `Customer`) |
| `reference_name` | Data | Referenced document name (e.g. `ORD-0001`) |

Standalone personal tasks have `reference_type` and `reference_name` unset. Document assignments link to a target record.

## Authorization and Security Invariant

> [!IMPORTANT]
> **Assignment never grants document access.**
> An assignment is an operational pointer, not an authorization mechanism. The authenticated user must independently have read permission on the referenced document through role permissions, user permissions, and controller `hasPermission` / `permissionQuery` hooks.

When a user's permission to a referenced document is denied or revoked:
1. `GET /api/todo/pending` filters out the task from the user's pending work listing.
2. Direct read attempts to `/api/resource/{doctype}/{name}` return `403 Forbidden`.
3. Renaming or deleting the referenced document cascades automatically via `referenceFields` cascades in `internal/engine/rename.go`.

## HTTP API

All assignment endpoints require an authenticated user.

| Method | Endpoint | Description |
| --- | --- | --- |
| `POST` | `/api/assignments/assign` | Assigns a document to a user. Creates a `ToDo`, adds a `Workflow` Comment on the target document, and notifies the assignee. |
| `POST` | `/api/assignments/complete` | Closes a `ToDo` (`status: "Closed"`) and adds a timeline Comment. Allowed for the assignee, assigner, or System Manager. |
| `POST` | `/api/assignments/revoke` | Cancels a `ToDo` (`status: "Cancelled"`) and adds a timeline Comment. Allowed for the assigner or System Manager. |
| `GET` | `/api/assignments/{doctype}/{name}` | Lists assignments for a specific document. Requires read permission on the document. |
| `GET` | `/api/todo/pending` | Lists pending work for the current user. Filters out any tasks whose referenced documents are not readable by the caller. |

### Assigning a document

Request body:
```json
{
  "doctype": "Order",
  "name": "ORD-0001",
  "allocated_to": "alice@example.com",
  "priority": "High",
  "date": "2026-09-20",
  "description": "Please review discount terms"
}
```

Response: `{ "data": ToDoDoc }`

### Listing pending work

Query parameters:
- `limit`: number of records per page (default: 20, max: 100).
- `offset`: page offset (default: 0).
- `status`: filter by status (`Open`, `Closed`, or omit for all).
- `scope`: `assigned_to_me` (default) or `assigned_by_me`.

Response:
```json
{
  "data": {
    "data": [ ... ],
    "total": 5
  }
}
```

## Desk SDK API

In client scripts and Desk components, assignments are accessible via `ddcore.assignments`:

```ts
import { ddcore } from "@ddcore/desk-sdk";

// Assign a document
await ddcore.assignments.assign("Order", "ORD-0001", {
  allocated_to: "alice@example.com",
  priority: "Urgent",
  date: "2026-09-18",
  description: "Verify shipping address",
});

// Complete an assignment
await ddcore.assignments.complete("TODO-0001");

// Revoke an assignment
await ddcore.assignments.revoke("TODO-0001");

// Fetch assignments for a document
const docTasks = await ddcore.assignments.forDoc("Order", "ORD-0001");

// Fetch current user's pending work
const pending = await ddcore.assignments.pending({
  status: "Open",
  scope: "assigned_to_me",
});
```

## Due Date Reminders

The core app registers a date-driven persistent notification rule (`core.todo_due`) on `ToDo`:
- Evaluated daily during scheduler notification sweeps (`e.SweepNotifications`).
- Triggers for open `ToDo` items where `date` is today.
- Generates a persistent Desk notification for the assignee (`allocated_to`).
- Deduplicated via the notification log to ensure users receive at most one reminder per task due date.

## Desk Integration

1. **Document Sidebar (`DocSidebar.svelte`):** Active documents display an "Assigned To" section with assignee badges, due dates (highlighted red when overdue), priority pills, "+ Assign" modal, and direct complete/revoke buttons.
2. **Pending Work Central (`/app/todo`):** Dedicated page listing user tasks with scope tabs ("Assigned to me", "Assigned by me"), status filters, checkboxes for instant completion, direct reference document links, and "+ New Task" modal for personal to-dos.
3. **Sidebar Badge:** Navigation sidebar displays a "To-Do" link with a badge indicating the current user's open task count.
