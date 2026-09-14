package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
		comment, err := c.NewDoc("Comment", engine.Doc{
			"comment_type":      "Workflow",
			"reference_doctype": body.Doctype,
			"reference_name":    body.Name,
			"content":           commentContent,
		})
		if err == nil {
			_, _ = c.Insert(comment, engine.SaveOpts{})
		}

		// 3. Dispatch persistent notification
		title := fmt.Sprintf("Assigned: %s %s", body.Doctype, body.Name)
		msg := body.Description
		if msg == "" {
			msg = fmt.Sprintf("%s assigned %s %s to you", c.User, body.Doctype, body.Name)
		}
		_ = c.NotifyUser(body.AllocatedTo, body.Doctype, body.Name, title, msg)

		return inserted, nil
	})
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
		isMgr := c.User == "Administrator" || contains(roles, "System Manager")
		if !isMgr && c.User != doc.Str("allocated_to") && c.User != doc.Str("assigned_by") {
			return nil, cerr.Permission("No permission to complete this task")
		}

		doc["status"] = "Closed"
		saved, err := c.Save(doc, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}

		refType := doc.Str("reference_type")
		refName := doc.Str("reference_name")
		if refType != "" && refName != "" {
			comment, err := c.NewDoc("Comment", engine.Doc{
				"comment_type":      "Workflow",
				"reference_doctype": refType,
				"reference_name":    refName,
				"content":           fmt.Sprintf("%s completed assignment", c.User),
			})
			if err == nil {
				_, _ = c.Insert(comment, engine.SaveOpts{})
			}
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
		isMgr := c.User == "Administrator" || contains(roles, "System Manager")
		if !isMgr && c.User != doc.Str("assigned_by") && c.User != doc.Str("allocated_to") {
			return nil, cerr.Permission("No permission to revoke this task")
		}

		doc["status"] = "Cancelled"
		_, err = c.Save(doc, engine.SaveOpts{})
		if err != nil {
			return nil, err
		}

		refType := doc.Str("reference_type")
		refName := doc.Str("reference_name")
		if refType != "" && refName != "" {
			comment, err := c.NewDoc("Comment", engine.Doc{
				"comment_type":      "Workflow",
				"reference_doctype": refType,
				"reference_name":    refName,
				"content":           fmt.Sprintf("%s revoked assignment for %s", c.User, doc.Str("allocated_to")),
			})
			if err == nil {
				_, _ = c.Insert(comment, engine.SaveOpts{})
			}
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
		return c.GetList("ToDo", engine.ListArgs{
			Filters: map[string]any{
				"reference_type": doctype,
				"reference_name": name,
				"status":         []any{"!=", "Cancelled"},
			},
			Fields:   []string{"name", "status", "priority", "date", "allocated_to", "assigned_by", "description", "creation", "modified"},
			OrderBy:  "creation desc",
			Limit:    100,
		})
	})
}
