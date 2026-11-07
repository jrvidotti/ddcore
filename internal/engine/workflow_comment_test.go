package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/richtext"
)

// workflowCommentFiles is the Artigo workflow with a "Return" that requires a
// comment, a controller recording the transition its save sees in doc.flags,
// and a notification built from the comment (#151).
func workflowCommentFiles() map[string]string {
	files := workflowTestFiles()
	files["workflows/artigo.workflow.ts"] = strings.Replace(files["workflows/artigo.workflow.ts"],
		`{ state: "Pending Approval", action: "Reject",`,
		`{ state: "Pending Approval", action: "Return", nextState: "Draft", allowed: "Editor", requireComment: true },
    { state: "Pending Approval", action: "Reject",`, 1)
	files["doctypes/artigo/artigo.controller.ts"] = `import { defineController } from "@ddcore/sdk";
export default defineController("Artigo", {
  beforeSave(doc) {
    const wf = doc.flags.workflow;
    if (wf) doc.status = [wf.action, wf.from, wf.to, wf.comment ?? "<none>"].join("|");
  },
});`
	files["notifications/artigo_returned.notification.ts"] = `import { defineNotification } from "@ddcore/sdk";
export default defineNotification({
  name: "demo.artigo_returned",
  doctype: "Artigo",
  event: "on_update",
  condition: (doc, before, context) => context.workflow?.action === "Return",
  recipients: (doc) => [doc.owner],
  desk: {
    title: (doc, before, context) => "Returned: " + context.workflow.comment,
    message: (doc, before, context) => context.workflow.from + " > " + context.workflow.to,
  },
});`
	return files
}

// pendingArtigo inserts an Artigo as the author and submits it for approval.
func pendingArtigo(t *testing.T, e *Engine) string {
	t.Helper()
	var id string
	err := e.Run(context.Background(), "autor@x.com", func(c *Ctx) error {
		// the timeline comment is written in the actor's language
		c.Lang = "en"
		doc, err := c.NewDoc("Artigo", Doc{"titulo": "Returned Article", "conteudo": "Detailed content"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		id = saved.ID()
		_, err = c.ApplyWorkflowTransition("Artigo", id, "Submit for Approval")
		return err
	})
	if err != nil {
		t.Fatalf("pending artigo: %v", err)
	}
	return id
}

// A transition with requireComment refuses an empty or blank comment, and the
// document stays where it was (#151).
func TestWorkflowComment_RequiredCommentRefused(t *testing.T) {
	e := setupWith(t, workflowCommentFiles())
	setupWorkflowTestUsers(t, e)
	id := pendingArtigo(t, e)
	for _, comment := range []string{"", "   \n "} {
		err := e.Run(context.Background(), "editor@x.com", func(c *Ctx) error {
			c.Lang = "en"
			_, err := c.ApplyWorkflow("Artigo", id, "Return", WorkflowApplyOpts{Comment: comment})
			return err
		})
		if err == nil || cerr.From(err).Type != "ValidationError" || !strings.Contains(err.Error(), "requires a comment") {
			t.Fatalf("Return with comment %q: want a validation error asking for a comment, got %v", comment, err)
		}
	}
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		if v, _ := c.GetValue("Artigo", id, "workflow_state"); db.Str(v) != "Pending Approval" {
			t.Fatalf("state = %v, want Pending Approval", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The comment lands in the timeline next to the transition and in the audit
// event, the controller sees the transition in doc.flags.workflow, and a
// notification reads it from its context argument (#151).
func TestWorkflowComment_CommentRecordedAndShared(t *testing.T) {
	e := setupWith(t, workflowCommentFiles())
	setupWorkflowTestUsers(t, e)
	id := pendingArtigo(t, e)
	ctx := context.Background()
	err := e.Run(ctx, "editor@x.com", func(c *Ctx) error {
		c.Lang = "en"
		saved, err := c.ApplyWorkflow("Artigo", id, "Return", WorkflowApplyOpts{Comment: "  Markup too low  "})
		if err != nil {
			return err
		}
		if saved.Str("workflow_state") != "Draft" {
			t.Fatalf("state = %s, want Draft", saved.Str("workflow_state"))
		}
		if got := saved.Str("status"); got != "Return|Pending Approval|Draft|Markup too low" {
			t.Fatalf("doc.flags.workflow seen by beforeSave = %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Return: %v", err)
	}
	crows, err := db.Select(ctx, e.DB.Pool, `SELECT content FROM tab_comment WHERE reference_doctype = 'Artigo' AND reference_id = $1 AND comment_type = 'Workflow' ORDER BY creation`, id)
	if err != nil || len(crows) != 2 {
		t.Fatalf("timeline comments = %d (%v), want 2", len(crows), err)
	}
	want := "editor@x.com applied action 'Return' (Pending Approval → Draft): Markup too low"
	if got := richtext.ToText(db.Str(crows[1]["content"])); got != want {
		t.Fatalf("timeline comment = %q, want %q", got, want)
	}
	arows, err := db.Select(ctx, e.DB.Pool, `SELECT detail FROM tab_audit_event WHERE target_doctype = 'Artigo' AND target_id = $1 AND action = 'workflow.transition' ORDER BY creation`, id)
	if err != nil || len(arows) != 2 {
		t.Fatalf("audit events = %d (%v), want 2", len(arows), err)
	}
	if detail := db.Str(arows[1]["detail"]); !strings.Contains(detail, "Markup too low") {
		t.Fatalf("audit detail = %s, want the comment", detail)
	}
	nrows, err := db.Select(ctx, e.DB.Pool, `SELECT recipient, title, message FROM ddcore_notification WHERE rule = 'demo.artigo_returned' AND reference_id = $1`, id)
	if err != nil || len(nrows) != 1 {
		t.Fatalf("notifications = %d (%v), want 1", len(nrows), err)
	}
	if db.Str(nrows[0]["recipient"]) != "autor@x.com" || db.Str(nrows[0]["title"]) != "Returned: Markup too low" ||
		db.Str(nrows[0]["message"]) != "Pending Approval > Draft" {
		t.Fatalf("notification = %v", nrows[0])
	}
}

// A comment is optional on a transition that does not require one, and is
// recorded all the same; without one the timeline reads as before and the
// controller sees a null comment (#151).
func TestWorkflowComment_OptionalComment(t *testing.T) {
	e := setupWith(t, workflowCommentFiles())
	setupWorkflowTestUsers(t, e)
	id := pendingArtigo(t, e)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		if v, _ := c.GetValue("Artigo", id, "status"); db.Str(v) != "Submit for Approval|Draft|Pending Approval|<none>" {
			t.Fatalf("status after a transition without a comment = %v", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(context.Background(), "editor@x.com", func(c *Ctx) error {
		c.Lang = "en"
		_, err := c.ApplyWorkflow("Artigo", id, "Reject", WorkflowApplyOpts{Comment: "Off topic"})
		return err
	})
	if err != nil {
		t.Fatalf("Reject with a comment: %v", err)
	}
	crows, err := db.Select(context.Background(), e.DB.Pool, `SELECT content FROM tab_comment WHERE reference_doctype = 'Artigo' AND reference_id = $1 AND comment_type = 'Workflow' ORDER BY creation`, id)
	if err != nil || len(crows) != 2 {
		t.Fatalf("timeline comments = %d (%v), want 2", len(crows), err)
	}
	if got := richtext.ToText(db.Str(crows[0]["content"])); got != "autor@x.com applied action 'Submit for Approval' (Draft → Pending Approval)" {
		t.Fatalf("comment without text = %q", got)
	}
	if got := richtext.ToText(db.Str(crows[1]["content"])); got != "editor@x.com applied action 'Reject' (Pending Approval → Rejected): Off topic" {
		t.Fatalf("comment with text = %q", got)
	}
}

// The actions offered say which ones need a comment, so the desk can ask for
// it before applying (#151).
func TestWorkflowComment_AvailableActionsSayRequireComment(t *testing.T) {
	e := setupWith(t, workflowCommentFiles())
	setupWorkflowTestUsers(t, e)
	id := pendingArtigo(t, e)
	err := e.Run(context.Background(), "editor@x.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Artigo", id)
		if err != nil {
			return err
		}
		actions, err := c.AvailableWorkflowActions("Artigo", doc)
		if err != nil {
			return err
		}
		got := map[string]bool{}
		for _, a := range actions {
			got[a.Action] = a.RequireComment
		}
		if req, ok := got["Return"]; !ok || !req {
			t.Fatalf("Return: requireComment = %v (offered %v), want true", req, ok)
		}
		if req, ok := got["Reject"]; !ok || req {
			t.Fatalf("Reject: requireComment = %v (offered %v), want false", req, ok)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// doc.applyWorkflow takes the comment from app code (#151).
func TestWorkflowComment_FromTS(t *testing.T) {
	e := setupWith(t, workflowCommentFiles())
	setupWorkflowTestUsers(t, e)
	id := pendingArtigo(t, e)
	eval := func(code string) (json.RawMessage, error) {
		var out json.RawMessage
		err := e.Run(context.Background(), "editor@x.com", func(c *Ctx) error {
			c.Lang = "en"
			rt, err := c.RT()
			if err != nil {
				return err
			}
			out, err = rt.Eval(code)
			return err
		})
		return out, err
	}
	if _, err := eval(`ddcore.getDoc("Artigo", "` + id + `").applyWorkflow("Return")`); err == nil || !strings.Contains(err.Error(), "requires a comment") {
		t.Fatalf("applyWorkflow without a comment: want refusal, got %v", err)
	}
	raw, err := eval(`ddcore.getDoc("Artigo", "` + id + `").applyWorkflow("Return", { comment: "From TS" }).status`)
	if err != nil {
		t.Fatalf("applyWorkflow with a comment: %v", err)
	}
	var status string
	if err := json.Unmarshal(raw, &status); err != nil || status != "Return|Pending Approval|Draft|From TS" {
		t.Fatalf("status = %s (%v)", raw, err)
	}
}
