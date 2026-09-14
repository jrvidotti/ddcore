package engine

import (
	"context"
	"testing"
)

func TestAssignment_ToDoDocTypeLoaded(t *testing.T) {
	e := setupWith(t, nil)
	dt, err := e.DocType("ToDo")
	if err != nil {
		t.Fatalf("expected ToDo doctype to be loaded: %v", err)
	}
	if dt.Module != "Core" {
		t.Fatalf("expected module Core, got %s", dt.Module)
	}
	expectedFields := []string{"status", "priority", "date", "allocated_to", "assigned_by", "description", "reference_type", "reference_name"}
	for _, f := range expectedFields {
		if dt.Field(f) == nil {
			t.Errorf("missing expected field %s on ToDo", f)
		}
	}
}

func TestAssignment_ControllerLifecycle(t *testing.T) {
	e := setupWith(t, nil)
	ctx := context.Background()

	// 1. Insert a ToDo as Administrator without assigned_by -> beforeInsert sets assigned_by to Administrator
	var todoName string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.NewDoc("ToDo", Doc{
			"allocated_to": "Administrator",
			"description":  "Personal task",
			"status":       "Open",
		})
		if err != nil {
			return err
		}
		inserted, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		todoName = inserted.Name()
		if inserted.Str("assigned_by") != "Administrator" {
			t.Fatalf("expected assigned_by to be Administrator, got %v", inserted.Str("assigned_by"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to insert ToDo: %v", err)
	}
	if todoName == "" {
		t.Fatal("expected todoName to be set")
	}
}
