package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/richtext"
)

type assignRequest struct {
	Doctype     string `json:"doctype"`
	ID          string `json:"id"`
	AllocatedTo string `json:"allocated_to"`
	Description string `json:"description"`
	Date        string `json:"date"`
	Priority    string `json:"priority"`
}

type completeOrRevokeRequest struct {
	ID string `json:"id"`
}

func (s *Server) assignDoc(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body assignRequest
		dec := json.NewDecoder(io.LimitReader(r.Body, 16384))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			return nil, cerr.Validation("Invalid JSON: {0}", err)
		}
		if body.Doctype == "" || body.ID == "" || body.AllocatedTo == "" {
			return nil, cerr.Validation("doctype, id and allocated_to are required")
		}
		if err := s.requireDocRead(c, body.Doctype, body.ID); err != nil {
			return nil, err
		}
		userRows, err := db.Select(c.Ctx, c.Q(), `SELECT enabled FROM tab_user WHERE id=$1`, body.AllocatedTo)
		if err != nil || len(userRows) == 0 || userRows[0]["enabled"] != true {
			return nil, cerr.Validation("User {0} does not exist or is disabled", body.AllocatedTo)
		}

		if body.Priority == "" {
			body.Priority = "Medium"
		}

		// 1. Create ToDo
		todo, err := c.NewDoc("ToDo", engine.Doc{
			"status":         "Open",
			"priority":       body.Priority,
			"date":           body.Date,
			"allocated_to":   body.AllocatedTo,
			"assigned_by":    c.User,
			"description":    body.Description,
			"reference_type": body.Doctype,
			"reference_id":   body.ID,
		})
		if err != nil {
			return nil, err
		}
		inserted, err := c.Insert(todo, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}

		// 2. Insert timeline Comment
		commentContent := fmt.Sprintf("%s assigned this document to %s", c.User, body.AllocatedTo)
		if body.Description != "" {
			commentContent += fmt.Sprintf(": %s", body.Description)
		}
		addTimelineComment(c, body.Doctype, body.ID, commentContent)

		// 3. Dispatch persistent notification, written in the assignee's language
		lang := c.RecipientLang([]string{body.AllocatedTo})
		label := body.Doctype
		if d, err := c.St.DocType(body.Doctype); err == nil {
			label = c.St.I18n.T(lang, d.Label)
		}
		title := c.St.I18n.T(lang, "Assigned: {0} {1}", label, body.ID)
		msg := body.Description
		if msg == "" {
			msg = c.St.I18n.T(lang, "{0} assigned {1} {2} to you", c.User, label, body.ID)
		}
		// Best effort, like the comment: a failure is rolled back to its
		// savepoint and logged, and the assignment still commits.
		if err := c.WithSavepoint(func() error {
			return c.NotifyUser(body.AllocatedTo, body.Doctype, body.ID, title, msg)
		}); err != nil {
			c.E.Log.Warn("assignment notification failed", "doctype", body.Doctype, "id", body.ID, "user", body.AllocatedTo, "err", err)
		}

		return inserted, nil
	})
}

// addTimelineComment records a Workflow comment on the referenced document. It
// is a side effect of the assignment action: a failure is rolled back to a
// savepoint and logged instead of aborting the request transaction.
func addTimelineComment(c *engine.Ctx, doctype, name, content string) {
	err := c.WithSavepoint(func() error {
		comment, err := c.NewDoc("Comment", engine.Doc{
			"comment_type":      "Workflow",
			"reference_doctype": doctype,
			"reference_id":      name,
			// server text, and the assignment's own description, are plain
			// text: marking them as such is what keeps a `<` a `<` instead of
			// the start of markup
			"content": richtext.FromPlainText(content),
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(comment, engine.SaveOpts{})
		return err
	})
	if err != nil {
		c.E.Log.Warn("assignment timeline comment failed", "doctype", doctype, "id", name, "err", err)
	}
}

func (s *Server) completeAssignment(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body completeOrRevokeRequest
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			return nil, cerr.Validation("Invalid JSON: {0}", err)
		}
		if body.ID == "" {
			return nil, cerr.Validation("id is required")
		}

		doc, err := c.GetDoc("ToDo", body.ID)
		if err != nil {
			return nil, err
		}

		roles, _ := c.Roles()
		isMgr := c.User == "Admin" || contains(roles, "System Manager")
		if !isMgr && c.User != doc.Str("allocated_to") && c.User != doc.Str("assigned_by") {
			return nil, cerr.Permission("No permission to complete this task")
		}

		doc["status"] = "Closed"
		saved, err := c.Save(doc, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}

		if refType, refName := doc.Str("reference_type"), doc.Str("reference_id"); refType != "" && refName != "" {
			addTimelineComment(c, refType, refName, fmt.Sprintf("%s completed assignment", c.User))
		}

		return saved, nil
	})
}

func (s *Server) revokeAssignment(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body completeOrRevokeRequest
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			return nil, cerr.Validation("Invalid JSON: {0}", err)
		}
		if body.ID == "" {
			return nil, cerr.Validation("id is required")
		}

		doc, err := c.GetDoc("ToDo", body.ID)
		if err != nil {
			return nil, err
		}

		roles, _ := c.Roles()
		isMgr := c.User == "Admin" || contains(roles, "System Manager")
		if !isMgr && c.User != doc.Str("assigned_by") && c.User != doc.Str("allocated_to") {
			return nil, cerr.Permission("No permission to revoke this task")
		}

		doc["status"] = "Cancelled"
		_, err = c.Save(doc, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}

		if refType, refName := doc.Str("reference_type"), doc.Str("reference_id"); refType != "" && refName != "" {
			addTimelineComment(c, refType, refName, fmt.Sprintf("%s revoked assignment for %s", c.User, doc.Str("allocated_to")))
		}

		return map[string]any{"success": true}, nil
	})
}

func (s *Server) reopenAssignment(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body completeOrRevokeRequest
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			return nil, cerr.Validation("Invalid JSON: {0}", err)
		}
		if body.ID == "" {
			return nil, cerr.Validation("id is required")
		}

		doc, err := c.GetDoc("ToDo", body.ID)
		if err != nil {
			return nil, err
		}

		roles, _ := c.Roles()
		isMgr := c.User == "Admin" || contains(roles, "System Manager")
		if !isMgr && c.User != doc.Str("allocated_to") && c.User != doc.Str("assigned_by") {
			return nil, cerr.Permission("No permission to reopen this task")
		}

		doc["status"] = "Open"
		saved, err := c.Save(doc, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}

		if refType, refName := doc.Str("reference_type"), doc.Str("reference_id"); refType != "" && refName != "" {
			addTimelineComment(c, refType, refName, fmt.Sprintf("%s reopened assignment", c.User))
		}

		return saved, nil
	})
}

func (s *Server) listDocAssignments(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		doctype := urlParam(r, "doctype")
		name := urlParam(r, "id")
		if err := s.requireDocRead(c, doctype, name); err != nil {
			return nil, err
		}
		// Anyone who can read the document sees every assignee, not only the
		// rows ToDo's permissionQuery would leave them (allocated_to = user).
		return c.GetList("ToDo", engine.ListArgs{
			IgnorePermissions: true,
			Filters: map[string]any{
				"reference_type": doctype,
				"reference_id":   name,
				"status":         []any{"!=", "Cancelled"},
			},
			Fields:  []string{"id", "status", "priority", "date", "allocated_to", "assigned_by", "description", "creation", "modified"},
			OrderBy: "creation desc",
			Limit:   100,
		})
	})
}

// pendingPriorityRank orders ToDo priorities by urgency rather than by name.
var pendingPriorityRank = map[string]int{"Low": 0, "Medium": 1, "High": 2, "Urgent": 3}

// pendingSortFields are the columns /api/todo/pending may sort on.
var pendingSortFields = map[string]bool{
	"date": true, "priority": true, "status": true, "creation": true, "modified": true,
	"allocated_to": true, "assigned_by": true, "description": true,
}

// parsePendingOrder reads "field asc|desc"; anything else sorts newest first.
func parsePendingOrder(s string) (field string, desc bool) {
	parts := strings.Fields(strings.ToLower(s))
	if len(parts) == 0 || len(parts) > 2 || !pendingSortFields[parts[0]] {
		return "creation", true
	}
	if len(parts) == 2 && parts[1] != "asc" && parts[1] != "desc" {
		return "creation", true
	}
	return parts[0], len(parts) == 2 && parts[1] == "desc"
}

// sortPending sorts in memory: the rows are already loaded for the reference
// check, and priority must follow urgency, not the alphabet. Empty values go
// last in either direction, so undated tasks never lead a due-date sort.
func sortPending(rows []map[string]any, field string, desc bool) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := db.Str(rows[i][field]), db.Str(rows[j][field])
		if (a == "") != (b == "") {
			return b == ""
		}
		var cmp int
		if field == "priority" {
			cmp = pendingPriorityRank[a] - pendingPriorityRank[b]
		} else {
			cmp = strings.Compare(a, b)
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func (s *Server) pendingWork(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		q := r.URL.Query()
		status := q.Get("status")
		if status == "" {
			status = "Open"
		}
		scope := q.Get("scope")
		if scope == "" {
			scope = "mine"
		}
		limit := 20
		offset := 0
		if q.Has("limit") {
			if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		if q.Has("offset") {
			if n, err := strconv.Atoi(q.Get("offset")); err == nil && n >= 0 {
				offset = n
			}
		}

		filters := []any{}
		if !strings.EqualFold(status, "all") {
			filters = append(filters, []any{"status", "=", status})
		}
		// the counterpart: who assigned my tasks, or whom I assigned them to
		counterpart := "assigned_by"
		if scope == "assigned_by_me" {
			filters = append(filters, []any{"assigned_by", "=", c.User})
			counterpart = "allocated_to"
		} else {
			filters = append(filters, []any{"allocated_to", "=", c.User})
		}
		if v := q.Get("user"); v != "" {
			filters = append(filters, []any{counterpart, "=", v})
		}
		if v := q.Get("priority"); v != "" {
			filters = append(filters, []any{"priority", "=", v})
		}
		if v := q.Get("date_from"); v != "" {
			filters = append(filters, []any{"date", ">=", v})
		}
		if v := q.Get("date_to"); v != "" {
			filters = append(filters, []any{"date", "<=", v})
		}
		if q.Get("no_date") == "1" {
			filters = append(filters, []any{"date", "not set", nil})
		}
		var orFilters any
		if v := strings.TrimSpace(q.Get("q")); v != "" {
			like := "%" + v + "%"
			orFilters = []any{
				[]any{"description", "like", like},
				[]any{"reference_type", "like", like},
				[]any{"reference_id", "like", like},
			}
		}

		// The participant filter above replaces ToDo's permissionQuery, which
		// would hide assigned_by_me tasks allocated to someone else; the
		// referenced document is still rechecked per row below.
		allCandidates, err := c.GetList("ToDo", engine.ListArgs{
			IgnorePermissions: true,
			Filters:           filters,
			OrFilters:         orFilters,
			Fields:            []string{"id", "status", "priority", "date", "allocated_to", "assigned_by", "description", "reference_type", "reference_id", "creation", "modified"},
			OrderBy:           "creation desc",
			Limit:             1000,
		})
		if err != nil {
			return nil, err
		}

		var filtered []map[string]any
		for _, item := range allCandidates {
			refType := db.Str(item["reference_type"])
			refName := db.Str(item["reference_id"])
			if refType != "" && refName != "" {
				if err := s.requireDocRead(c, refType, refName); err != nil {
					continue // skip items whose reference doc the user cannot read
				}
			}
			filtered = append(filtered, item)
		}
		sortField, sortDesc := parsePendingOrder(q.Get("order_by"))
		sortPending(filtered, sortField, sortDesc)

		total := len(filtered)
		start := offset
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}
		page := filtered[start:end]
		if page == nil {
			page = []map[string]any{}
		}
		docs := make([]engine.Doc, len(page))
		for i, row := range page {
			docs[i] = engine.Doc(row)
		}

		return map[string]any{
			"data":   page,
			"total":  total,
			"titles": c.ResolveLinkTitles("ToDo", docs...),
		}, nil
	})
}
