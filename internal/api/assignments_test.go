package api

import (
	"fmt"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func TestAssignments_AssignCompleteRevokeHTTP(t *testing.T) {
	x := setup(t)
	ana, bia, ze := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com"), "sid:"+x.sid("ze@x.com")

	// 1. Ana creates a Pessoa "Cliente A"
	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Cliente A"}, ana)
	x.expect(r, 200, "")

	// 2. Ana assigns "Cliente A" to Bia
	assignPayload := map[string]any{
		"doctype":      "Pessoa",
		"id":           "Cliente A",
		"allocated_to": "bia@x.com",
		"description":  "Please contact client",
		"date":         "2026-09-25",
		"priority":     "High",
	}
	r = x.call("POST", "/api/assignments/assign", assignPayload, ana)
	x.expect(r, 200, "")
	todo := r.Body["data"].(map[string]any)
	todoName := fmt.Sprint(todo["id"])
	if todo["status"] != "Open" || todo["allocated_to"] != "bia@x.com" || todo["assigned_by"] != "ana@x.com" {
		t.Fatalf("unexpected todo data: %v", todo)
	}

	// 3. Verify timeline Comment was created on Cliente A
	r = x.call("GET", "/api/comments/Pessoa/Cliente A", nil, ana)
	x.expect(r, 200, "")
	comments := r.Body["data"].([]any)
	if len(comments) == 0 {
		t.Fatal("expected timeline comment on assignment")
	}

	// 4. Verify notification was created for Bia
	r = x.call("GET", "/api/notifications", nil, bia)
	x.expect(r, 200, "")
	notifData := r.Body["data"].(map[string]any)["data"].([]any)
	if len(notifData) == 0 {
		t.Fatal("expected notification for bia")
	}

	// 5. List assignments on document
	r = x.call("GET", "/api/assignments/Pessoa/Cliente A", nil, bia)
	x.expect(r, 200, "")
	docAssignments := r.Body["data"].([]any)
	if len(docAssignments) != 1 {
		t.Fatalf("expected 1 assignment on doc, got %d", len(docAssignments))
	}

	// 6. Ze (who cannot read Pessoa Cliente A) tries to list assignments -> 403
	r = x.call("GET", "/api/assignments/Pessoa/Cliente A", nil, ze)
	x.expect(r, 403, "")

	// 7. Complete assignment by Bia
	r = x.call("POST", "/api/assignments/complete", map[string]any{"id": todoName}, bia)
	x.expect(r, 200, "")
	completedTodo := r.Body["data"].(map[string]any)
	if completedTodo["status"] != "Closed" {
		t.Fatalf("expected status Closed, got %v", completedTodo["status"])
	}

	// Bia checks pending work: default (Open) returns 0, Closed returns 1, all returns 1
	r = x.call("GET", "/api/todo/pending", nil, bia)
	x.expect(r, 200, "")
	if len(r.Body["data"].(map[string]any)["data"].([]any)) != 0 {
		t.Fatalf("expected 0 open tasks for bia, got %v", r.Body["data"])
	}

	r = x.call("GET", "/api/todo/pending?status=Closed", nil, bia)
	x.expect(r, 200, "")
	if len(r.Body["data"].(map[string]any)["data"].([]any)) != 1 {
		t.Fatalf("expected 1 closed task for bia, got %v", r.Body["data"])
	}

	r = x.call("GET", "/api/todo/pending?status=all", nil, bia)
	x.expect(r, 200, "")
	if len(r.Body["data"].(map[string]any)["data"].([]any)) != 1 {
		t.Fatalf("expected 1 task with status=all for bia, got %v", r.Body["data"])
	}

	r = x.call("GET", "/api/todo/pending?status=All", nil, bia)
	x.expect(r, 200, "")
	if len(r.Body["data"].(map[string]any)["data"].([]any)) != 1 {
		t.Fatalf("expected 1 task with status=All for bia, got %v", r.Body["data"])
	}

	// 8. Revoke assignment by Ana
	r = x.call("POST", "/api/assignments/revoke", map[string]any{"id": todoName}, ana)
	x.expect(r, 200, "")
}

func TestPendingWork_FiltersUnauthorizedDocuments(t *testing.T) {
	x := setup(t)
	ana, ze := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("ze@x.com")

	// Ana creates Pessoa Secreta (Ze cannot read)
	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Pessoa Secreta"}, ana)
	x.expect(r, 200, "")

	// Ana assigns Pessoa Secreta to Ze
	r = x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype":      "Pessoa",
		"id":           "Pessoa Secreta",
		"allocated_to": "ze@x.com",
		"description":  "Secret review",
	}, ana)
	x.expect(r, 200, "")

	// Ze creates a personal ToDo
	r = x.call("POST", "/api/resource/ToDo", map[string]any{
		"description":  "Personal note",
		"allocated_to": "ze@x.com",
		"status":       "Open",
	}, ze)
	x.expect(r, 200, "")

	// Ze calls GET /api/todo/pending
	r = x.call("GET", "/api/todo/pending", nil, ze)
	x.expect(r, 200, "")
	body := r.Body["data"].(map[string]any)
	tasks := body["data"].([]any)
	// Must only contain the personal note! Pessoa Secreta must be hidden!
	if len(tasks) != 1 {
		t.Fatalf("expected exactly 1 task (personal), got %d: %v", len(tasks), tasks)
	}
	if tasks[0].(map[string]any)["description"] != "Personal note" {
		t.Fatalf("expected Personal note, got %v", tasks[0])
	}

	// Ana creates Pessoa Compartilhada
	r = x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Compartilhada"}, ana)
	x.expect(r, 200, "")

	// Grant role Gestor to Ze so Ze CAN read Pessoa
	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.Q().Exec(c.Ctx, `INSERT INTO tab_has_role (id, parent, parenttype, parentfield, role) VALUES ($1, 'ze@x.com', 'User', 'roles', 'Gestor')`, engine.RandomToken())
		return err
	})
	x.e.Cache.Del("roles:ze@x.com")

	// Ana assigns Compartilhada to Ze
	r = x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype":      "Pessoa",
		"id":           "Compartilhada",
		"allocated_to": "ze@x.com",
		"description":  "Shared task",
	}, ana)
	x.expect(r, 200, "")

	// Ze calls GET /api/todo/pending -> now sees 3 tasks (Personal + Secreta + Compartilhada)
	r = x.call("GET", "/api/todo/pending", nil, ze)
	x.expect(r, 200, "")
	body = r.Body["data"].(map[string]any)
	tasks = body["data"].([]any)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Revoke role Gestor from Ze
	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.Q().Exec(c.Ctx, `DELETE FROM tab_has_role WHERE parent='ze@x.com' AND role='Gestor'`)
		return err
	})
	x.e.Cache.Del("roles:ze@x.com")

	// Ze calls GET /api/todo/pending -> task disappeared, back to 1 task!
	r = x.call("GET", "/api/todo/pending", nil, ze)
	x.expect(r, 200, "")
	body = r.Body["data"].(map[string]any)
	tasks = body["data"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task after revocation, got %d", len(tasks))
	}

	// Ze attempts to read Compartilhada directly -> 403 PermissionError!
	r = x.call("GET", "/api/resource/Pessoa/Compartilhada", nil, ze)
	x.expect(r, 403, "")
}

// ToDo's permissionQuery narrows every listing to allocated_to = user. The
// assignment endpoints must show the assigner their tasks and every assignee
// the other assignees of a document they can read.
func TestAssignments_ParticipantsSeeEachOther(t *testing.T) {
	x := setup(t)
	ana, bia, ze := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com"), "sid:"+x.sid("ze@x.com")

	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Shared Doc"}, ana)
	x.expect(r, 200, "")
	for _, who := range []string{"bia@x.com", "root@x.com"} {
		r = x.call("POST", "/api/assignments/assign", map[string]any{
			"doctype": "Pessoa", "id": "Shared Doc", "allocated_to": who,
		}, ana)
		x.expect(r, 200, "")
	}

	// Ana (Gestor, not System Manager) sees both tasks she assigned.
	r = x.call("GET", "/api/todo/pending?scope=assigned_by_me", nil, ana)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["data"].([]any); len(got) != 2 {
		t.Fatalf("assigner expected 2 tasks under assigned_by_me, got %d: %v", len(got), got)
	}
	// Assigned to Ana: nothing.
	r = x.call("GET", "/api/todo/pending", nil, ana)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["data"].([]any); len(got) != 0 {
		t.Fatalf("assigner expected 0 tasks assigned to her, got %v", got)
	}

	// Bia, one of the assignees, sees every assignee in the document sidebar.
	r = x.call("GET", "/api/assignments/Pessoa/Shared Doc", nil, bia)
	x.expect(r, 200, "")
	if got := r.Body["data"].([]any); len(got) != 2 {
		t.Fatalf("assignee expected 2 assignments on the document, got %d: %v", len(got), got)
	}
	// The generic listing stays allocated-to-only.
	r = x.call("GET", "/api/resource/ToDo", nil, bia)
	x.expect(r, 200, "")
	if got := r.Body["data"].([]any); len(got) != 1 {
		t.Fatalf("generic ToDo listing expected 1 row for bia, got %d: %v", len(got), got)
	}

	// Ze cannot read the document: 403 on the sidebar.
	r = x.call("GET", "/api/assignments/Pessoa/Shared Doc", nil, ze)
	x.expect(r, 403, "")

	// Bia assigns too; once she loses read access (she is not the owner),
	// her assigned_by_me list hides the task.
	r = x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype": "Pessoa", "id": "Shared Doc", "allocated_to": "ze@x.com",
	}, bia)
	x.expect(r, 200, "")
	r = x.call("GET", "/api/todo/pending?scope=assigned_by_me", nil, bia)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["data"].([]any); len(got) != 1 {
		t.Fatalf("bia expected 1 task under assigned_by_me, got %v", got)
	}
	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.Q().Exec(c.Ctx, `DELETE FROM tab_has_role WHERE parent='bia@x.com' AND role='Gestor'`)
		return err
	})
	x.e.Cache.Del("roles:bia@x.com")
	r = x.call("GET", "/api/todo/pending?scope=assigned_by_me", nil, bia)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["data"].([]any); len(got) != 0 {
		t.Fatalf("assigner without read access expected 0 tasks, got %v", got)
	}
	r = x.call("GET", "/api/assignments/Pessoa/Shared Doc", nil, bia)
	x.expect(r, 403, "")
}

func TestToDo_AssignedByCannotBeSpoofed(t *testing.T) {
	x := setup(t)
	bia, root := "sid:"+x.sid("bia@x.com"), "sid:"+x.sid("root@x.com")

	r := x.call("POST", "/api/resource/ToDo", map[string]any{
		"description": "Spoofed", "allocated_to": "bia@x.com", "assigned_by": "ana@x.com",
	}, bia)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["assigned_by"]; got != "bia@x.com" {
		t.Fatalf("assigned_by = %v, want bia@x.com", got)
	}

	r = x.call("POST", "/api/resource/ToDo", map[string]any{
		"description": "On behalf", "allocated_to": "bia@x.com", "assigned_by": "ana@x.com",
	}, root)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["assigned_by"]; got != "ana@x.com" {
		t.Fatalf("System Manager assigned_by = %v, want ana@x.com", got)
	}
}

func TestAssignments_NotificationInRecipientLanguage(t *testing.T) {
	x := setup(t)
	ana, bia := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com")
	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.Q().Exec(c.Ctx, `UPDATE tab_user SET language='pt-BR' WHERE id='bia@x.com'`)
		return err
	})

	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Lang Doc"}, ana)
	x.expect(r, 200, "")
	r = x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype": "Pessoa", "id": "Lang Doc", "allocated_to": "bia@x.com",
	}, ana)
	x.expect(r, 200, "")

	r = x.call("GET", "/api/notifications", nil, bia)
	x.expect(r, 200, "")
	items := r.Body["data"].(map[string]any)["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 notification, got %v", items)
	}
	n := items[0].(map[string]any)
	if n["title"] != "Atribuído: Pessoa Lang Doc" || n["message"] != "ana@x.com atribuiu Pessoa Lang Doc a você" {
		t.Fatalf("notification not in recipient language: %v", n)
	}
}

func TestToDo_UpdateCannotRewriteAssignment(t *testing.T) {
	x := setup(t)
	ana, bia, root := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com"), "sid:"+x.sid("root@x.com")

	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Update Doc"}, ana)
	x.expect(r, 200, "")
	r = x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype": "Pessoa", "id": "Update Doc", "allocated_to": "bia@x.com",
	}, ana)
	x.expect(r, 200, "")
	name := fmt.Sprint(r.Body["data"].(map[string]any)["id"])

	for _, change := range []map[string]any{
		{"assigned_by": "root@x.com"},
		{"allocated_to": "ana@x.com"},
		{"reference_id": "Other Doc"},
	} {
		r = x.call("PUT", "/api/resource/ToDo/"+name, change, bia)
		x.expect(r, 417, "ValidationError")
	}
	r = x.call("GET", "/api/resource/ToDo/"+name, nil, bia)
	x.expect(r, 200, "")
	got := r.Body["data"].(map[string]any)
	if got["assigned_by"] != "ana@x.com" || got["allocated_to"] != "bia@x.com" || got["reference_id"] != "Update Doc" {
		t.Fatalf("assignment rewritten: %v", got)
	}

	// Other fields stay editable by a participant.
	r = x.call("PUT", "/api/resource/ToDo/"+name, map[string]any{"description": "Edited"}, bia)
	x.expect(r, 200, "")
	// A System Manager may still rewrite the assignment.
	r = x.call("PUT", "/api/resource/ToDo/"+name, map[string]any{"assigned_by": "root@x.com"}, root)
	x.expect(r, 200, "")
}

// A database failure in a side effect (timeline comment, notification) must
// not abort the request transaction: the assignment itself still commits.
func TestAssignments_SideEffectFailureStillCommits(t *testing.T) {
	x := setup(t)
	ana, bia := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com")
	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Faulty Doc"}, ana)
	x.expect(r, 200, "")

	x.asAdmin(func(c *engine.Ctx) error {
		_, err := c.Q().Exec(c.Ctx, `
CREATE OR REPLACE FUNCTION ddcore_test_fail() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'induced failure'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER ddcore_test_fail BEFORE INSERT ON tab_comment FOR EACH ROW EXECUTE FUNCTION ddcore_test_fail();
CREATE TRIGGER ddcore_test_fail BEFORE INSERT ON ddcore_notification FOR EACH ROW EXECUTE FUNCTION ddcore_test_fail();`)
		return err
	})
	t.Cleanup(func() {
		x.asAdmin(func(c *engine.Ctx) error {
			_, err := c.Q().Exec(c.Ctx, `
DROP TRIGGER IF EXISTS ddcore_test_fail ON tab_comment;
DROP TRIGGER IF EXISTS ddcore_test_fail ON ddcore_notification;
DROP FUNCTION IF EXISTS ddcore_test_fail();`)
			return err
		})
	})

	r = x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype": "Pessoa", "id": "Faulty Doc", "allocated_to": "bia@x.com",
	}, ana)
	x.expect(r, 200, "")
	name := fmt.Sprint(r.Body["data"].(map[string]any)["id"])

	r = x.call("POST", "/api/assignments/complete", map[string]any{"id": name}, bia)
	x.expect(r, 200, "")
	r = x.call("POST", "/api/assignments/revoke", map[string]any{"id": name}, ana)
	x.expect(r, 200, "")

	r = x.call("GET", "/api/resource/ToDo/"+name, nil, ana)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["status"]; got != "Cancelled" {
		t.Fatalf("status = %v, want Cancelled", got)
	}
}

func pendingDescriptions(t *testing.T, x *env, url, sid string) []string {
	t.Helper()
	r := x.call("GET", url, nil, sid)
	x.expect(r, 200, "")
	var out []string
	for _, row := range r.Body["data"].(map[string]any)["data"].([]any) {
		out = append(out, fmt.Sprint(row.(map[string]any)["description"]))
	}
	return out
}

func TestPendingWork_FiltersAndOrder(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	x.sid("bia@x.com")
	for _, td := range []map[string]any{
		{"description": "Alpha report", "priority": "Low", "date": "2026-10-05"},
		{"description": "Beta call", "priority": "Urgent", "date": "2026-10-01"},
		{"description": "Gamma", "priority": "Medium"},
	} {
		td["allocated_to"] = "ana@x.com"
		x.expect(x.call("POST", "/api/resource/ToDo", td, ana), 200, "")
	}
	x.expect(x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Cliente F"}, ana), 200, "")
	x.expect(x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype": "Pessoa", "id": "Cliente F", "allocated_to": "bia@x.com", "description": "For Bia",
	}, ana), 200, "")

	cases := []struct {
		url  string
		want string
	}{
		{"/api/todo/pending?priority=Urgent", "[Beta call]"},
		{"/api/todo/pending?date_from=2026-10-01&date_to=2026-10-03", "[Beta call]"},
		{"/api/todo/pending?no_date=1", "[Gamma]"},
		// undated=1 adds the tasks without a due date to the window
		{"/api/todo/pending?date_from=2026-10-01&date_to=2026-10-03&undated=1&order_by=date+asc", "[Beta call Gamma]"},
		{"/api/todo/pending?q=ALPHA", "[Alpha report]"},
		// priority follows urgency, not the alphabet
		{"/api/todo/pending?order_by=priority+desc", "[Beta call Gamma Alpha report]"},
		// undated tasks come last in either direction
		{"/api/todo/pending?order_by=date+asc", "[Beta call Alpha report Gamma]"},
		{"/api/todo/pending?order_by=date+desc", "[Alpha report Beta call Gamma]"},
		{"/api/todo/pending?order_by=description+asc&limit=2&offset=1", "[Beta call Gamma]"},
		{"/api/todo/pending?scope=assigned_by_me&user=bia@x.com", "[For Bia]"},
		{"/api/todo/pending?scope=assigned_by_me&user=ze@x.com", "[]"},
	}
	for _, tc := range cases {
		if got := fmt.Sprint(pendingDescriptions(t, x, tc.url, ana)); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.url, got, tc.want)
		}
	}
	// an unknown sort column falls back to the default instead of reaching SQL
	if got := len(pendingDescriptions(t, x, "/api/todo/pending?order_by=secret;drop", ana)); got != 3 {
		t.Fatalf("bogus order_by: got %d rows", got)
	}
}

func TestAssignments_Reopen(t *testing.T) {
	x := setup(t)
	ana, bia, ze := "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("bia@x.com"), "sid:"+x.sid("ze@x.com")
	x.expect(x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Cliente R"}, ana), 200, "")
	r := x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype": "Pessoa", "id": "Cliente R", "allocated_to": "bia@x.com", "description": "Reopen me",
	}, ana)
	x.expect(r, 200, "")
	id := fmt.Sprint(r.Body["data"].(map[string]any)["id"])
	x.expect(x.call("POST", "/api/assignments/complete", map[string]any{"id": id}, bia), 200, "")

	x.expect(x.call("POST", "/api/assignments/reopen", map[string]any{"id": id}, ze), 403, "")
	r = x.call("POST", "/api/assignments/reopen", map[string]any{"id": id}, bia)
	x.expect(r, 200, "")
	if st := r.Body["data"].(map[string]any)["status"]; st != "Open" {
		t.Fatalf("expected Open after reopen, got %v", st)
	}
	if got := pendingDescriptions(t, x, "/api/todo/pending", bia); fmt.Sprint(got) != "[Reopen me]" {
		t.Fatalf("reopened task missing from pending: %v", got)
	}
}
