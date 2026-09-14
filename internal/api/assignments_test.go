package api

import (
	"fmt"
	"testing"
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

	// 8. Revoke assignment by Ana
	r = x.call("POST", "/api/assignments/revoke", map[string]any{"name": todoName}, ana)
	x.expect(r, 200, "")
}
