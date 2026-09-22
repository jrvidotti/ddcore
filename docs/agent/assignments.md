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
| `reference_id` | Data | Referenced document id (e.g. `ORD-0001`) |

Standalone personal tasks have `reference_type` and `reference_id` unset. Document assignments link to a target record.

## Authorization

> [!IMPORTANT]
> **Assignment never grants document access.**
> An assignment is an operational pointer, not an authorization mechanism. The authenticated user must independently have read permission on the referenced document through role permissions, a document share, user permissions, and controller `hasPermission` / `permissionQuery` hooks. To give an assignee access, share the document with them (see `sharing`).

When a user loses read permission on a referenced document, `GET /api/todo/pending` and `GET /api/assignments/{doctype}/{id}` stop showing its tasks to that user, and a direct read of the document through `/api/resource/{doctype}/{id}` returns `403 Forbidden`.

Renaming a document updates `reference_type` and `reference_id` on its ToDos through `coreRefs` in `internal/engine/rename.go`. Deleting a document deletes its ToDos with a direct `DELETE`; no ToDo hooks run.

### Generic ToDo CRUD

`/api/resource/ToDo` follows the ToDo controller. A System Manager can do everything. Anyone else can read and write a ToDo when they are its assigner or its assignee and can read the referenced document, if there is one; only the assigner can delete it. The listing shows the tasks allocated to the caller. On insert, `assigned_by` is set to the creating user; only a System Manager can record another user as assigner. On update, anyone but a System Manager is refused a change to `assigned_by`, `allocated_to`, `reference_type` or `reference_id`: an assignment changes hands through the assignment endpoints, not by editing the ToDo.

## HTTP API

All assignment endpoints require an authenticated user. Assignments are available only over HTTP and through the Desk SDK: there is no server-side `ddcore.assign*` function, CLI command or MCP tool.

| Method | Endpoint | Description |
| --- | --- | --- |
| `POST` | `/api/assignments/assign` | Assigns a document to a user. Creates a `ToDo`, adds a `Workflow` Comment on the target document, and notifies the assignee. |
| `POST` | `/api/assignments/complete` | Closes the `ToDo` named by `{ id }` (`status: "Closed"`) and adds a timeline Comment. Allowed for the assignee, the assigner, or a System Manager. |
| `POST` | `/api/assignments/revoke` | Cancels the `ToDo` named by `{ id }` (`status: "Cancelled"`) and adds a timeline Comment. Allowed for the assigner, the assignee, or a System Manager. Returns `{ "success": true }`. |
| `GET` | `/api/assignments/{doctype}/{id}` | Lists the assignments of a document. Requires read permission on the document. |
| `GET` | `/api/todo/pending` | Lists pending work for the current user, leaving out tasks whose referenced documents the caller cannot read. |

Assignment actions write no `Audit Event`. Nothing prevents a second open assignment of the same document to the same user. Timeline comments are written in English.

### Assigning a document

Request body:
```json
{
  "doctype": "Order",
  "id": "ORD-0001",
  "allocated_to": "alice@example.com",
  "priority": "High",
  "date": "2026-09-20",
  "description": "Please review discount terms"
}
```

The caller needs read permission on the target document. `allocated_to` must be an existing, enabled user. Unknown JSON fields are rejected, and `priority` defaults to `Medium`.

The assignee's notification is written in the assignee's language: the title is "Assigned: {doctype} {id}", and the message is the description, or "{user} assigned {doctype} {id} to you" when there is none. No notification is sent when users assign to themselves, or when the assignee cannot read the document. The timeline comment and the notification each run in a savepoint: when one fails, its writes are rolled back, the error is logged, and the assignment still commits. Completing and revoking treat their timeline comments the same way.

Response: `{ "data": ToDoDoc }`

### Listing a document's assignments

Anyone who can read the document sees every assignment on it, whoever the assignee is. Cancelled tasks are left out, and at most 100 are returned, newest first.

### Listing pending work

Query parameters:
- `limit`: number of records per page (default: 20, max: 100).
- `offset`: page offset (default: 0).
- `status`: `Open` (the default), `Closed`, `Cancelled`, or `all` for every status.
- `scope`: `assigned_by_me` lists tasks the caller assigned; any other value, or none, lists tasks assigned to the caller.

The endpoint loads at most 1000 candidate tasks, newest first, then drops those whose referenced document the caller cannot read, then pages. A user with more than 1000 matching tasks can see truncated results. `total` is the count after the access check.

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
- Evaluated by the notification sweep, which the scheduler queues every five minutes while it is running.
- Triggers for open `ToDo` items where `date` is today.
- Generates a persistent Desk notification for the assignee (`allocated_to`).
- Deduplicated via the notification log to ensure users receive at most one reminder per task due date.

## Desk Integration

1. **Document Sidebar (`DocSidebar.svelte`):** Active documents display an "Assigned To" section with assignee badges, due dates (highlighted red when overdue), priority pills, "+ Assign" modal, and direct complete/revoke buttons.
2. **Pending Work Central (`/app/todo`):** Dedicated page listing user tasks with scope tabs ("Assigned to me", "Assigned by me"), status filters, checkboxes for instant completion, direct reference document links, and "+ New Task" modal for personal to-dos.
3. **Sidebar Badge:** Navigation sidebar displays a "To-Do" link with a badge indicating the current user's open task count.
