package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// FeedbackDocType is the core DocType a user's feedback is written to.
const FeedbackDocType = "Feedback"

// The bounds of one feedback: what a person types into a dialog, not a dump.
const (
	FeedbackMaxFiles    = 10
	feedbackPerHour     = 20
	feedbackMaxTitle    = 200
	feedbackMaxText     = 20000
	feedbackMaxURL      = 2048
	feedbackMaxContext  = 64 << 10
	feedbackMineLimit   = 50
	feedbackNotifyRule  = "feedback"
	feedbackMailKeyHead = "feedback:"
)

// feedbackFields are the fields each type adds to the title and description.
// Anything else in a submission is refused, so the dialog and the DocType
// cannot drift apart without a test noticing.
var feedbackFields = map[string][]string{
	"Bug":             {"severity", "steps_to_reproduce", "expected_result", "actual_result"},
	"Improvement":     {"current_behavior", "suggested_improvement"},
	"Feature Request": {"problem", "expected_benefit"},
}

var feedbackSeverities = []string{"Low", "Medium", "High", "Critical"}

// FeedbackAuthor is who sent a feedback and from which space, read in the
// author's own space before the write moves to the platform's.
type FeedbackAuthor struct {
	User, Name, Email, Tenant string
}

// FeedbackFile is one attachment of a submission: a file the user picked, a
// pasted screenshot, the recorded audio note.
type FeedbackFile struct {
	Name, ContentType string
	Size              int64
	Body              io.Reader
}

// FeedbackSummary is what the author sees of their own feedback.
type FeedbackSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Type     string `json:"feedback_type"`
	Status   string `json:"status"`
	Response string `json:"response"`
	Creation string `json:"creation"` // RFC 3339, UTC
}

// FeedbackAuthorOf reads the author of a submission in the ctx's space.
func (c *Ctx) FeedbackAuthorOf() (FeedbackAuthor, error) {
	if c.User == "" || c.User == "Guest" {
		return FeedbackAuthor{}, cerr.Auth("Sign in to continue")
	}
	if c.IsWebsiteUser() {
		return FeedbackAuthor{}, cerr.Permission("Feedback is sent from the desk")
	}
	a := FeedbackAuthor{User: c.User, Name: c.User, Tenant: c.Tenant}
	// a platform user inside a tenant has no User row there: the id is all
	// there is to show, as the boot does
	if u, err := c.GetValues("User", c.User, []string{"full_name", "email"}); err == nil && u != nil {
		if n := db.Str(u["full_name"]); n != "" {
			a.Name = n
		}
		a.Email = db.Str(u["email"])
	}
	return a, nil
}

// feedbackDoc checks a submission and turns it into the Feedback document,
// before any file is read or anything written.
func feedbackDoc(data map[string]any) (Doc, error) {
	str := func(k string, max int) (string, error) {
		v, ok := data[k]
		if !ok || v == nil {
			return "", nil
		}
		s, ok := v.(string)
		if !ok {
			return "", cerr.Validation("Invalid {0}: {1}", k, "expected text")
		}
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) > max {
			return "", cerr.Validation("Invalid {0}: {1}", k, fmt.Sprintf("longer than %d characters", max))
		}
		return s, nil
	}
	kind, err := str("feedback_type", 40)
	if err != nil {
		return nil, err
	}
	own, ok := feedbackFields[kind]
	if !ok {
		return nil, cerr.Validation("Invalid {0}: {1}", "feedback_type", "expected Bug, Improvement or Feature Request")
	}
	allowed := map[string]bool{"feedback_type": true, "title": true, "description": true, "page_url": true, "context": true}
	for _, f := range own {
		allowed[f] = true
	}
	for k := range data {
		if !allowed[k] {
			return nil, cerr.Validation("Invalid {0}: {1}", k, "not a field of "+kind)
		}
	}
	doc := Doc{"doctype": FeedbackDocType, "feedback_type": kind, "status": "New"}
	for _, k := range append([]string{"title", "description", "page_url"}, own...) {
		max := feedbackMaxText
		switch k {
		case "title":
			max = feedbackMaxTitle
		case "page_url":
			max = feedbackMaxURL
		}
		s, err := str(k, max)
		if err != nil {
			return nil, err
		}
		if s != "" {
			doc[k] = s
		}
	}
	for _, k := range []string{"title", "description"} {
		if doc[k] == nil {
			return nil, cerr.Validation("Fill in the required fields: {0}", k).WithTitleKey("Required fields")
		}
	}
	if sev, ok := doc["severity"].(string); ok && !slices.Contains(feedbackSeverities, sev) {
		return nil, cerr.Validation("Invalid {0}: {1}", "severity", "expected Low, Medium, High or Critical")
	}
	if ctx, ok := data["context"]; ok && ctx != nil {
		if _, isObj := ctx.(map[string]any); !isObj {
			return nil, cerr.Validation("Invalid {0}: {1}", "context", "expected an object")
		}
		b, _ := json.Marshal(ctx)
		if len(b) > feedbackMaxContext {
			return nil, cerr.Validation("Invalid {0}: {1}", "context", "too large")
		}
		doc["context"] = string(b)
	}
	return doc, nil
}

// platformCtx is ctx naming no tenant: a ctx run as Admin under it works in
// the platform space, whichever space the request came from.
func platformCtx(ctx context.Context) context.Context { return WithTenant(ctx, "") }

// SubmitFeedback writes a user's feedback, with its files, in the platform
// space, where the developers who read it work, and tells them: the desk
// inbox of every System Manager there, and a mail to the site's feedback
// addresses. It returns the new document's id.
//
// It runs as Admin because the author may belong to a tenant, and a tenant's
// user cannot write in the platform space; who wrote it is kept in
// reported_by and source_tenant instead.
func (e *Engine) SubmitFeedback(ctx context.Context, author FeedbackAuthor, data map[string]any, files []FeedbackFile) (string, error) {
	if !e.Cfg.Feedback.On() {
		return "", cerr.NotFound("Feedback is turned off on this site")
	}
	doc, err := feedbackDoc(data)
	if err != nil {
		return "", err
	}
	if len(files) > FeedbackMaxFiles {
		return "", cerr.Validation("At most {0} files can be attached", FeedbackMaxFiles).WithTitleKey("Too many files")
	}
	doc["reported_by"] = author.User
	doc["reporter_name"] = author.Name
	doc["reporter_email"] = author.Email
	doc["source_tenant"] = author.Tenant
	doc["app_version"] = Version

	var id string
	err = e.Run(platformCtx(ctx), "Admin", func(c *Ctx) error {
		var recent int64
		if err := c.Q().QueryRow(c.Ctx, `SELECT count(*) FROM tab_feedback
			WHERE reported_by = $1 AND coalesce(source_tenant, '') = $2 AND creation > now() - interval '1 hour'`,
			author.User, author.Tenant).Scan(&recent); err != nil {
			return err
		}
		if recent >= feedbackPerHour {
			return cerr.Validation("You sent {0} feedbacks in the last hour; try again later", recent).WithTitleKey("Too many feedbacks")
		}
		saved, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		id = db.Str(saved["id"])
		var stored []FeedbackFile
		var fileIDs []string
		for _, f := range files {
			row, err := c.StoreFile(NewFile{Doctype: FeedbackDocType, ID: id, Name: f.Name, ContentType: f.ContentType, Size: f.Size, Private: true}, f.Body)
			if err != nil {
				return err
			}
			stored = append(stored, f)
			fileIDs = append(fileIDs, db.Str(row["id"]))
		}
		if err := c.notifyFeedback(saved, author); err != nil {
			return err
		}
		c.mailFeedback(saved, stored, fileIDs)
		return nil
	})
	return id, err
}

// notifyFeedback tells every System Manager of the platform but the author.
func (c *Ctx) notifyFeedback(doc Doc, author FeedbackAuthor) error {
	rows, err := db.Select(c.Ctx, c.Q(), `SELECT DISTINCT parent FROM tab_has_role
		WHERE parenttype = 'User' AND role = 'System Manager'`)
	if err != nil {
		return err
	}
	title := c.T("New feedback: {0}", db.Str(doc["title"]))
	message := c.T(db.Str(doc["feedback_type"])) + " · " + author.Name
	for _, r := range rows {
		user := db.Str(r["parent"])
		if user == author.User && author.Tenant == "" {
			continue
		}
		if err := c.notifyRecipient(feedbackNotifyRule, user, FeedbackDocType, db.Str(doc["id"]), title, message); err != nil {
			return err
		}
	}
	return nil
}

// mailFeedback queues the copy for the site's feedback addresses. It is a
// courtesy, like a notification's mail: a failure is logged, never the reason
// a user's feedback is lost. Files that would push the message past the
// site's attachment limit stay on the document, which the mail links to.
func (c *Ctx) mailFeedback(doc Doc, files []FeedbackFile, fileIDs []string) {
	to := c.E.Cfg.Feedback.To
	if len(to) == 0 {
		return
	}
	var attach []string
	var total int64
	limit := c.E.Cfg.Mail.MaxAttachment
	for i, f := range files {
		if limit > 0 && total+f.Size > limit {
			continue
		}
		total += f.Size
		attach = append(attach, fileIDs[i])
	}
	args := map[string]any{"doctype": FeedbackDocType, "id": doc["id"], "files": len(files), "attached": len(attach)}
	for _, k := range []string{"feedback_type", "title", "description", "severity", "steps_to_reproduce", "expected_result",
		"actual_result", "current_behavior", "suggested_improvement", "problem", "expected_benefit",
		"reporter_name", "reporter_email", "source_tenant", "page_url"} {
		if v, ok := doc[k]; ok && v != nil && v != "" {
			args[k] = v
		}
	}
	err := c.WithSavepoint(func() error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"template": MailTemplateFeedback, "to": to, "args": args,
			"lang": c.RecipientLang(to), "attach": attach, "reference": MailReference{Doctype: FeedbackDocType, ID: db.Str(doc["id"])},
			"key": feedbackMailKeyHead + db.Str(doc["id"])})
		_, err = rt.CallFunction("core.services.mail.queue", payload)
		return err
	})
	if err != nil {
		c.E.Log.Warn("feedback email failed", "id", doc["id"], "err", err)
	}
}

// ListMyFeedback is the author's own feedback, newest first, read in the
// platform space where it was written.
func (e *Engine) ListMyFeedback(ctx context.Context, author FeedbackAuthor) ([]FeedbackSummary, error) {
	if !e.Cfg.Feedback.On() {
		return nil, cerr.NotFound("Feedback is turned off on this site")
	}
	out := []FeedbackSummary{}
	err := e.Run(platformCtx(ctx), "Admin", func(c *Ctx) error {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, title, feedback_type, status, response,
			to_char(creation AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS creation FROM tab_feedback
			WHERE reported_by = $1 AND coalesce(source_tenant, '') = $2 ORDER BY creation DESC LIMIT $3`,
			author.User, author.Tenant, feedbackMineLimit)
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, FeedbackSummary{ID: db.Str(r["id"]), Title: db.Str(r["title"]), Type: db.Str(r["feedback_type"]),
				Status: db.Str(r["status"]), Response: db.Str(r["response"]), Creation: db.Str(r["creation"])})
		}
		return nil
	})
	return out, err
}
