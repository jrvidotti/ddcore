# Assignments and Pending Work (OPS-05) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement generic team task handling and assignments (OPS-05) in Core: a standard `ToDo` DocType, assignment lifecycle operations (`assign`, `revoke`, `complete`), due dates, "my pending work" view, persistent notification reminders, and strict document access boundaries.

**Architecture:** A Core `ToDo` DocType with controller-enforced authorization ensuring assignment never grants document access; dedicated API endpoints in `internal/api/assignments.go` handling atomic assignment workflows, timeline comments (`Workflow`), and persistent notifications; Desk integration via a `DocSidebar.svelte` assignment widget and a central `/app/todo` pending work page.

**Tech Stack:** Go (engine & API), TypeScript (SDK, Desk, controllers), PostgreSQL, SvelteKit (Desk UI).

**Spec:** [`docs/superpowers/specs/2026-09-13-assignments-design.md`](../specs/2026-09-13-assignments-design.md)

## Global Constraints

- **Workspace:** `.worktrees/ops-05-assignments` on branch `feat/ops-05-assignments`.
- **Language:** English is canonical for all code, labels, comments, docs, and git commits. Translations in `translations/pt-BR.csv`.
- **Server Runtime:** Synchronous TypeScript on the server (`goja`). Never use `async`/`await` in server controllers or services.
- **Security Boundary:** Assignment must NOT grant document access. If a user cannot read the referenced document, reading the ToDo fails with 403 or is filtered out from pending work.
- **Verification:** Every task must end with automated tests passing (`go test ./...` and/or `npm run test`). Final verification requires `make check` and `make test`.

---

### Task 1: Core `ToDo` DocType & Controller

**Files:**
- Create: `core/doctypes/todo/todo.doctype.ts`
- Create: `core/doctypes/todo/todo.controller.ts`
- Test: `internal/engine/assignments_test.go`

**Interfaces:**
- Produces: DocType `ToDo` with fields (`status`, `priority`, `date`, `allocated_to`, `assigned_by`, `description`, `reference_type`, `reference_name`).
- Controller exports `defineController("ToDo", { hasPermission, beforeInsert })`.

- [ ] **Step 1: Write the failing test**

In `internal/engine/assignments_test.go`:
```go
package engine_test

import (
	"context"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func TestAssignment_ToDoDocTypeLoaded(t *testing.T) {
	e := setupEngine(t)
	dt, err := e.DocType("ToDo")
	if err != nil {
		t.Fatalf("expected ToDo doctype: %v", err)
	}
	if dt.Module != "Core" {
		t.Fatalf("expected module Core, got %s", dt.Module)
	}
	for _, field := range []string{"status", "priority", "date", "allocated_to", "assigned_by", "description", "reference_type", "reference_name"} {
		if dt.Field(field) == nil {
			t.Errorf("missing field %s on ToDo", field)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine -run TestAssignment_ToDoDocTypeLoaded`
Expected: FAIL ("unknown DocType ToDo" or similar)

- [ ] **Step 3: Implement DocType and Controller**

In `core/doctypes/todo/todo.doctype.ts`:
```ts
import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "ToDo",
  module: "Core",
  label: "ToDo",
  icon: "check-square",
  fields: [
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["Open", "Closed", "Cancelled"], default: "Open", inStandardFilter: true },
    { fieldname: "priority", fieldtype: "Select", label: "Priority", options: ["Low", "Medium", "High", "Urgent"], default: "Medium", inStandardFilter: true },
    { fieldname: "date", fieldtype: "Date", label: "Due Date", inStandardFilter: true },
    { fieldname: "allocated_to", fieldtype: "Link", label: "Assigned To", options: "User", reqd: true, inStandardFilter: true },
    { fieldname: "assigned_by", fieldtype: "Link", label: "Assigned By", options: "User", readOnly: true },
    { fieldname: "description", fieldtype: "Small Text", label: "Description" },
    { fieldname: "reference_type", fieldtype: "Data", label: "Reference DocType", searchIndex: true },
    { fieldname: "reference_name", fieldtype: "Data", label: "Reference Document", searchIndex: true },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "All", read: true, create: true, write: true, delete: true },
  ],
});
```

In `core/doctypes/todo/todo.controller.ts`:
```ts
import { defineController } from "@ddcore/sdk";

function canReadReference(doctype: string, docname: string): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false;
  return ddcore.hasPermission(doctype, "read", { name: docname, owner }) === true;
}

export default defineController("ToDo", {
  beforeInsert(doc) {
    if (!doc.assigned_by) {
      doc.assigned_by = ddcore.user();
    }
  },
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined;
    const roles = ddcore.getRoles(user) || [];
    if (roles.indexOf("System Manager") >= 0) return true;
    if (user !== doc.allocated_to && user !== doc.assigned_by) return false;
    if (doc.reference_type && doc.reference_name) {
      if (!canReadReference(String(doc.reference_type), String(doc.reference_name))) {
        return false;
      }
    }
    if (ptype === "write") {
      return user === doc.allocated_to || user === doc.assigned_by;
    }
    if (ptype === "delete") {
      return user === doc.assigned_by;
    }
    return true;
  },
});
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine -run TestAssignment_ToDoDocTypeLoaded`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/doctypes/todo/ internal/engine/assignments_test.go
git commit -m "feat(assignments): define ToDo doctype and controller"
```

---

### Task 2: Reference Authorization, Rename Cascades & API Guards

**Files:**
- Modify: `internal/engine/rename.go`
- Modify: `internal/api/api.go`
- Test: `internal/engine/assignments_test.go`
- Test: `internal/api/assignments_test.go`

**Interfaces:**
- `coreRefs`: includes `tab_todo` with `reference_type` and `reference_name`.
- `referenceFields`: includes `"ToDo": {"reference_type", "reference_name"}`.

- [ ] **Step 1: Write failing tests for rename, deletion cascade, and authorization**

In `internal/engine/assignments_test.go`:
```go
func TestAssignment_RenameAndDeletionCascade(t *testing.T) {
	// 1. Create a document (e.g. Project or custom Doc)
	// 2. Insert a ToDo referencing that document
	// 3. Rename document -> verify ToDo.reference_name is updated
	// 4. Delete document -> verify ToDo is removed
}

func TestAssignment_DoesNotGrantDocumentAccess(t *testing.T) {
	// User B is assigned Document X, but has no read permission on Document X.
	// 1. Verify User B reading Document X returns 403 PermissionError.
	// 2. Verify User B reading the ToDo returns 403 PermissionError.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/engine -run TestAssignment_`
Expected: FAIL

- [ ] **Step 3: Register `tab_todo` in `rename.go` and `api.go`**

In `internal/engine/rename.go`:
Add `{table: "tab_todo", doctypeCol: "reference_type", nameCol: "reference_name"}` to `coreRefs`.

In `internal/api/api.go`:
Add `"ToDo": {"reference_type", "reference_name"}` to `referenceFields`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine -run TestAssignment_`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/rename.go internal/api/api.go internal/engine/assignments_test.go
git commit -m "feat(assignments): register ToDo in rename cascades and referenceGuard"
```

---

### Task 3: Assignment Workflow Operations & Endpoints

**Files:**
- Create: `internal/api/assignments.go`
- Modify: `internal/api/api.go` (register routes)
- Test: `internal/api/assignments_test.go`

**Interfaces:**
- `POST /api/assignments/assign` -> `{ data: ToDo }`
- `POST /api/assignments/complete` -> `{ data: ToDo }`
- `POST /api/assignments/revoke` -> `{ data: { success: true } }`
- `GET /api/assignments/{doctype}/{name}` -> `{ data: ToDo[] }`

- [ ] **Step 1: Write failing tests for assignment endpoints**

In `internal/api/assignments_test.go`:
- Test assigning a document creates a ToDo, creates a `Workflow` Comment on the document, and creates a persistent notification for the assignee.
- Test completing marks ToDo as Closed and adds a timeline Comment.
- Test revoking marks ToDo as Cancelled and adds a timeline Comment.
- Test listing document assignments returns current assignments when authorized, and 403 when caller lacks read permission.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api -run TestAssignmentEndpoints`
Expected: FAIL (404 Not Found)

- [ ] **Step 3: Implement `internal/api/assignments.go`**

Implement handlers:
- `assignHandler`: parses request, checks `requireDocRead`, inserts ToDo with `allocated_to`, `assigned_by = c.User`, `reference_type`, `reference_name`, `date`, `priority`, `description`. Inserts a Comment (`comment_type: "Workflow"`, `content: fmt.Sprintf("%s assigned this document to %s", c.User, allocatedTo)`). Dispatches notification to `allocated_to` if `allocated_to != c.User`.
- `completeHandler`: loads ToDo, checks caller is `allocated_to` or `assigned_by` or System Manager, sets status to `Closed`. Adds timeline comment on reference doc if set.
- `revokeHandler`: loads ToDo, checks caller is `assigned_by` or System Manager, sets status to `Cancelled`. Adds timeline comment on reference doc if set.
- `listDocAssignmentsHandler`: checks `requireDocRead`, queries `tab_todo` for `reference_type` and `reference_name`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api -run TestAssignmentEndpoints`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/assignments.go internal/api/api.go internal/api/assignments_test.go
git commit -m "feat(assignments): implement assign, complete, revoke and document list endpoints"
```

---

### Task 4: "My Pending Work" API Endpoint with Access Filtering

**Files:**
- Modify: `internal/api/assignments.go`
- Test: `internal/api/assignments_test.go`

**Interfaces:**
- `GET /api/todo/pending?status=Open&limit=50&offset=0` -> `{ data: { data: ToDo[], total: number } }`

- [ ] **Step 1: Write failing test for pending work and recipient visibility**

In `internal/api/assignments_test.go`:
```go
func TestPendingWork_FiltersUnauthorizedDocuments(t *testing.T) {
	// User B has two ToDos:
	// - One standalone personal ToDo
	// - One referencing Document X (which User B can read)
	// - One referencing Document Y (which User B cannot read)
	// Query GET /api/todo/pending as User B
	// Verify result contains personal ToDo and Document X ToDo, but NOT Document Y ToDo.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestPendingWork`
Expected: FAIL

- [ ] **Step 3: Implement pending work handler in `internal/api/assignments.go`**

Implement `pendingWorkHandler`:
- Selects ToDos where `allocated_to = c.User` (or `assigned_by = c.User` if filtered by scope).
- For each ToDo with a reference document:
  - Calls `c.HasPermission(reference_type, "read", doc)` or `requireDocRead`.
  - If access denied or doc not found, skips the item.
- Returns paginated list and total count.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run TestPendingWork`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/assignments.go internal/api/assignments_test.go
git commit -m "feat(assignments): implement pending work endpoint with strict reference access filtering"
```

---

### Task 5: Due Date Reminders Integration

**Files:**
- Create: `core/notifications/todo_due.notification.ts` (or core notification registration)
- Modify: `core/embed.go` (embed notifications if placed in `core/notifications`)
- Test: `internal/engine/assignments_test.go`

**Interfaces:**
- Date notification rule on `ToDo`: `field: "date"`, `days: 0`, condition: `doc.status === "Open"`.

- [ ] **Step 1: Write failing test for ToDo due date reminder**

In `internal/engine/assignments_test.go`:
```go
func TestAssignment_DueDateReminder(t *testing.T) {
	// Insert an open ToDo for User B with date = today
	// Trigger the notification date sweep (ScanDueNotifications)
	// Verify ddcore_notification has an occurrence for User B with title/message mentioning the task
	// Verify second sweep deduplicates via ddcore_notification_due
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine -run TestAssignment_DueDateReminder`
Expected: FAIL

- [ ] **Step 3: Implement ToDo due date notification**

Register the date rule for `ToDo`:
```ts
import { defineNotification, _ } from "@ddcore/sdk";

export default defineNotification({
  name: "core.todo_due",
  doctype: "ToDo",
  date: { field: "date", days: 0 },
  condition: (doc) => doc.status === "Open",
  recipients: (doc) => [doc.allocated_to],
  desk: {
    title: (doc) => _("Assignment due today: {0}", [doc.description || doc.name]),
    message: (doc) => _("Task assigned by {0} is due today", [doc.assigned_by]),
  },
});
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine -run TestAssignment_DueDateReminder`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/ notifications/ internal/engine/assignments_test.go
git commit -m "feat(assignments): configure persistent notification for due date reminders"
```

---

### Task 6: Desk SDK & Client API Integration

**Files:**
- Modify: `packages/desk-sdk/src/index.ts`
- Modify: `desk/src/lib/api.ts`
- Create: `desk/src/lib/assignments.svelte.ts`
- Test: `desk/src/lib/assignments.svelte.test.ts`

**Interfaces:**
- `api.assignments.assign(doctype, name, args)`
- `api.assignments.complete(todoName)`
- `api.assignments.revoke(todoName)`
- `api.assignments.forDoc(doctype, name)`
- `api.assignments.pending(options)`

- [ ] **Step 1: Write failing desk unit test**

In `desk/src/lib/assignments.svelte.test.ts`:
- Test `AssignmentCenter` / helper class manages state, loads document assignments, handles toggle completion, and handles error states.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd desk && npm run test`
Expected: FAIL

- [ ] **Step 3: Implement client API and store**

In `packages/desk-sdk/src/index.ts` and `desk/src/lib/api.ts`:
Add assignments API methods.
In `desk/src/lib/assignments.svelte.ts`:
Implement reactive state manager for assignments.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd desk && npm run test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add packages/desk-sdk/ desk/src/lib/api.ts desk/src/lib/assignments.svelte.ts desk/src/lib/assignments.svelte.test.ts
git commit -m "feat(assignments): add Desk SDK client methods and reactive assignments state"
```

---

### Task 7: Desk UI — Document Sidebar Assignment Widget

**Files:**
- Modify: `desk/src/lib/components/DocSidebar.svelte`
- Create: `desk/src/lib/components/AssignModal.svelte` (or inline assign modal)
- Test: `desk/src/lib/components/sidebar.test.ts` (or new component test)

**Interfaces:**
- `DocSidebar` displays active assignments for `frm.doc`, complete and revoke buttons, and "+ Assign" button.

- [ ] **Step 1: Write component test or update sidebar tests**

In `desk/src/lib/components/sidebar.test.ts`:
Test that assignments block renders when loaded and triggers action callbacks.

- [ ] **Step 2: Implement "Assigned To" section in `DocSidebar.svelte`**

Add assignments section:
- Lists assignments with assignee, due date, priority pill.
- Complete button (checkmark icon) calling `api.assignments.complete`.
- Revoke button (X icon) calling `api.assignments.revoke`.
- "+ Assign" button opening modal:
  - User select dropdown (fetched from active users).
  - Due date picker.
  - Priority selector (`Low`, `Medium`, `High`, `Urgent`).
  - Description textarea.
  - Submit action calling `api.assignments.assign`.
- Refreshes comments and assignments on completion.

- [ ] **Step 3: Run desk tests and verify build**

Run: `cd desk && npm run test && npm run build`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add desk/src/lib/components/DocSidebar.svelte desk/src/lib/components/AssignModal.svelte
git commit -m "feat(desk): add document sidebar assignment widget and assign modal"
```

---

### Task 8: Desk UI — "My Pending Work" Page (`/app/todo`) & Navigation

**Files:**
- Create: `desk/src/routes/app/todo/+page.svelte`
- Modify: `desk/src/lib/components/Sidebar.svelte` (add To-Do nav link with badge)
- Test: `desk/src/lib/components/sidebar.test.ts`

**Interfaces:**
- Route: `/app/todo` displays user's pending work with filters (Pending, Assigned by me, Completed), checkbox to complete, and document link.
- Sidebar: adds "To-Do" navigation link with count badge.

- [ ] **Step 1: Write test for sidebar link and todo page loading**

Update `desk/src/lib/components/sidebar.test.ts` to assert To-Do link presence.

- [ ] **Step 2: Implement `/app/todo/+page.svelte` and update `Sidebar.svelte`**

Create `/app/todo/+page.svelte`:
- Filter buttons: "Pending", "Assigned by me", "Completed".
- Task list items:
  - Checkbox for completion toggle.
  - Task description.
  - Reference document link (`doctypeLabel` + name) opening the document form.
  - Due date pill (colored red if overdue).
  - Priority badge.
- Empty state: icon + "No pending tasks".
- "+ New Task" modal for standalone tasks.

Update `desk/src/lib/components/Sidebar.svelte`:
- Add To-Do link with check-square icon and open tasks count badge.

- [ ] **Step 3: Run desk tests and verify build**

Run: `cd desk && npm run test && npm run build`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add desk/src/routes/app/todo/+page.svelte desk/src/lib/components/Sidebar.svelte
git commit -m "feat(desk): create My Pending Work page at /app/todo and sidebar navigation"
```

---

### Task 9: Documentation, Translations & Validation

**Files:**
- Create: `docs/agent/assignments.md`
- Modify: `docs/agent/index.md`
- Modify: `docs/frappe-implemented-features.md`
- Modify: `ROADMAP.md`
- Modify: `core/translations/pt-BR.csv`

- [ ] **Step 1: Write `docs/agent/assignments.md`**

Document:
- ToDo DocType schema.
- Assigning, revoking, completing via API and SDK.
- Authorization model: how reference permissions are checked and why assignment does not grant document access.
- Due date reminders.
- Desk pending work view.

- [ ] **Step 2: Update documentation indices and roadmap**

- In `docs/agent/index.md`: add link to `assignments.md`.
- In `docs/frappe-implemented-features.md`: mark OPS-05 as implemented.
- In `ROADMAP.md`: update OPS-05 status.

- [ ] **Step 3: Extract and translate strings**

Run:
```bash
./bin/ddcore i18n extract --all --lang pt-BR
```
Ensure all newly added English strings have Portuguese translations in `core/translations/pt-BR.csv`.

- [ ] **Step 4: Run full verification suite**

Run:
```bash
make check
make test
```
Expected: All checks, Go tests, desk tests, and acceptance tests PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/ core/translations/ ROADMAP.md
git commit -m "docs: document assignments and pending work (OPS-05) and complete translations"
```
