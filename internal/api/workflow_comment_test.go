package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// POST /api/workflow/apply takes the comment a transition with requireComment
// asks for, and the actions say which ones ask for it (#151).
func TestWorkflowAPI_Comment(t *testing.T) {
	dir := workflowTestApp(t)
	p := filepath.Join(dir, "workflows/artigo.workflow.ts")
	src, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(string(src), `action: "Reject", nextState: "Rejected", allowed: "Editor" }`,
		`action: "Reject", nextState: "Rejected", allowed: "Editor", requireComment: true }`, 1)
	if patched == string(src) {
		t.Fatal("fixture: the Reject transition was not found")
	}
	if err := os.WriteFile(p, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	wf := setupWorkflowEnvIn(t, dir)
	x, autor, editor := wf.x, wf.autor, wf.editor

	r := x.call("POST", "/api/resource/Artigo", map[string]any{"titulo": "Commented", "conteudo": "Long enough content"}, autor)
	x.expect(r, 200, "")
	id := fmt.Sprint(r.Body["data"].(map[string]any)["id"])
	r = x.call("POST", "/api/workflow/apply", map[string]any{"doctype": "Artigo", "id": id, "action": "Submit for Approval"}, autor)
	x.expect(r, 200, "")

	r = x.call("GET", "/api/workflow/actions?doctype=Artigo&id="+id, nil, editor)
	x.expect(r, 200, "")
	required := map[string]any{}
	for _, a := range r.Body["data"].(map[string]any)["actions"].([]any) {
		m := a.(map[string]any)
		required[fmt.Sprint(m["action"])] = m["requireComment"]
	}
	if required["Reject"] != true || required["Approve"] != false {
		t.Fatalf("requireComment by action = %v, want Reject true and Approve false", required)
	}

	r = x.call("POST", "/api/workflow/apply", map[string]any{"doctype": "Artigo", "id": id, "action": "Reject", "comment": "  "}, editor)
	x.expect(r, 417, "ValidationError")

	r = x.call("POST", "/api/workflow/apply", map[string]any{"doctype": "Artigo", "id": id, "action": "Reject", "comment": "Not ours"}, editor)
	x.expect(r, 200, "")
	if got := r.Body["data"].(map[string]any)["workflow_state"]; got != "Rejected" {
		t.Fatalf("workflow_state = %v, want Rejected", got)
	}
	r = x.call("GET", "/api/comments/Artigo/"+id, nil, editor)
	x.expect(r, 200, "")
	if body := fmt.Sprint(r.Body["data"]); !strings.Contains(body, "Not ours") {
		t.Fatalf("timeline = %s, want the comment", body)
	}
}
