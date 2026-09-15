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
		"name":         "Cliente A",
		"allocated_to": "bia@x.com",
		"description":  "Please contact client",
		"date":         "2026-09-25",
		"priority":     "High",
	}
	r = x.call("POST", "/api/assignments/assign", assignPayload, ana)
	x.expect(r, 200, "")
	todo := r.Body["data"].(map[string]any)
	todoName := fmt.Sprint(todo["name"])
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
	r = x.call("POST", "/api/assignments/complete", map[string]any{"name": todoName}, bia)
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
	r = x.call("POST", "/api/assignments/revoke", map[string]any{"name": todoName}, ana)
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
		"name":         "Pessoa Secreta",
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
		_, err := c.Q().Exec(c.Ctx, `INSERT INTO tab_has_role (name, parent, parenttype, parentfield, role) VALUES ($1, 'ze@x.com', 'User', 'roles', 'Gestor')`, engine.RandomToken())
		return err
	})
	x.e.Cache.Del("roles:ze@x.com")

	// Ana assigns Compartilhada to Ze
	r = x.call("POST", "/api/assignments/assign", map[string]any{
		"doctype":      "Pessoa",
		"name":         "Compartilhada",
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

