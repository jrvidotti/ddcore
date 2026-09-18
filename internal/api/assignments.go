package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

type assignRequest struct {
	Doctype     string `json:"doctype"`
	Name        string `json:"name"`
	AllocatedTo string `json:"allocated_to"`
	Description string `json:"description"`
	Date        string `json:"date"`
	Priority    string `json:"priority"`
}

type completeOrRevokeRequest struct {
	Name string `json:"name"`
}

func (s *Server) assignDoc(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		var body assignRequest
		dec := json.NewDecoder(io.LimitReader(r.Body, 16384))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			return nil, cerr.Validation("Invalid JSON: {0}", err)
		}
		if body.Doctype == "" || body.Name == "" || body.AllocatedTo == "" {
			return nil, cerr.Validation("doctype, name and allocated_to are required")
		}
		if err := s.requireDocRead(c, body.Doctype, body.Name); err != nil {
			return nil, err
		}
		userRows, err := db.Select(c.Ctx, c.Q(), `SELECT enabled FROM tab_user WHERE name=$1`, body.AllocatedTo)
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
			"reference_name": body.Name,
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
		addTimelineComment(c, body.Doctype, body.Name, commentContent)

		// 3. Dispatch persistent notification, written in the assignee's language
		lang := c.RecipientLang([]string{body.AllocatedTo})
		label := body.Doctype
		if d, err := c.St.DocType(body.Doctype); err == nil {
			label = c.St.I18n.T(lang, d.Label)
		}
		title := c.St.I18n.T(lang, "Assigned: {0} {1}", label, body.Name)
		msg := body.Description
		if msg == "" {
			msg = c.St.I18n.T(lang, "{0} assigned {1} {2} to you", c.User, label, body.Name)
		}
		// Best effort, like the comment: a failure is rolled back to its
		// savepoint and logged, and the assignment still commits.
		if err := c.WithSavepoint(func() error {
			return c.NotifyUser(body.AllocatedTo, body.Doctype, body.Name, title, msg)
		}); err != nil {
			c.E.Log.Warn("assignment notification failed", "doctype", body.Doctype, "name", body.Name, "user", body.AllocatedTo, "err", err)
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
			"reference_name":    name,
			"content":           content,
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(comment, engine.SaveOpts{})
		return err
	})
	if err != nil {
		c.E.Log.Warn("assignment timeline comment failed", "doctype", doctype, "name", name, "err", err)
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
		if body.Name == "" {
			return nil, cerr.Validation("name is required")
		}

		doc, err := c.GetDoc("ToDo", body.Name)
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

		if refType, refName := doc.Str("reference_type"), doc.Str("reference_name"); refType != "" && refName != "" {
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
		if body.Name == "" {
			return nil, cerr.Validation("name is required")
		}

		doc, err := c.GetDoc("ToDo", body.Name)
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

		if refType, refName := doc.Str("reference_type"), doc.Str("reference_name"); refType != "" && refName != "" {
			addTimelineComment(c, refType, refName, fmt.Sprintf("%s revoked assignment for %s", c.User, doc.Str("allocated_to")))
		}

		return map[string]any{"success": true}, nil
	})
}

func (s *Server) listDocAssignments(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		doctype := urlParam(r, "doctype")
		name := urlParam(r, "name")
		if err := s.requireDocRead(c, doctype, name); err != nil {
			return nil, err
		}
		// Anyone who can read the document sees every assignee, not only the
		// rows ToDo's permissionQuery would leave them (allocated_to = user).
		return c.GetList("ToDo", engine.ListArgs{
			IgnorePermissions: true,
			Filters: map[string]any{
				"reference_type": doctype,
				"reference_name": name,
				"status":         []any{"!=", "Cancelled"},
			},
			Fields:  []string{"name", "status", "priority", "date", "allocated_to", "assigned_by", "description", "creation", "modified"},
			OrderBy: "creation desc",
			Limit:   100,
		})
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
			if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}
		if q.Has("offset") {
			if n, err := strconv.Atoi(q.Get("offset")); err == nil && n >= 0 {
				offset = n
			}
		}

		filters := map[string]any{}
		if !strings.EqualFold(status, "all") {
			filters["status"] = status
		}
		if scope == "assigned_by_me" {
			filters["assigned_by"] = c.User
		} else {
			filters["allocated_to"] = c.User
		}

		// The participant filter above replaces ToDo's permissionQuery, which
		// would hide assigned_by_me tasks allocated to someone else; the
		// referenced document is still rechecked per row below.
		allCandidates, err := c.GetList("ToDo", engine.ListArgs{
			IgnorePermissions: true,
			Filters:           filters,
			Fields:            []string{"name", "status", "priority", "date", "allocated_to", "assigned_by", "description", "reference_type", "reference_name", "creation", "modified"},
			OrderBy:           "creation desc",
			Limit:             1000,
		})
		if err != nil {
			return nil, err
		}

		var filtered []map[string]any
		for _, item := range allCandidates {
			refType := db.Str(item["reference_type"])
			refName := db.Str(item["reference_name"])
			if refType != "" && refName != "" {
				if err := s.requireDocRead(c, refType, refName); err != nil {
					continue // skip items whose reference doc the user cannot read
				}
			}
			filtered = append(filtered, item)
		}

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

		return map[string]any{
			"data":  page,
			"total": total,
		}, nil
	})
}
