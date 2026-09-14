package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
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

func TestAssignment_RenameAndDeletionCascade(t *testing.T) {
	e := setupWith(t, nil)
	ctx := context.Background()

	var todoName string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		p, err := c.NewDoc("Pessoa", Doc{"nome": "Original Cascade"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			return err
		}

		todo, err := c.NewDoc("ToDo", Doc{
			"allocated_to":   "Administrator",
			"reference_type": "Pessoa",
			"reference_name": "Original Cascade",
			"description":    "Review person",
		})
		if err != nil {
			return err
		}
		ins, err := c.Insert(todo, SaveOpts{})
		if err != nil {
			return err
		}
		todoName = ins.Name()
		return nil
	})
	if err != nil {
		t.Fatalf("failed setup: %v", err)
	}

	// 1. Rename Pessoa
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		if _, err := c.Rename("Pessoa", "Original Cascade", "Renamed Cascade"); err != nil {
			return err
		}
		tDoc, err := c.GetDoc("ToDo", todoName)
		if err != nil {
			return err
		}
		if tDoc.Str("reference_name") != "Renamed Cascade" {
			t.Fatalf("expected reference_name to be Renamed Cascade, got %s", tDoc.Str("reference_name"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("rename cascade failed: %v", err)
	}

	// 2. Delete Pessoa
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		if err := c.Delete("Pessoa", "Renamed Cascade", false, false); err != nil {
			return err
		}
		_, err := c.GetDoc("ToDo", todoName)
		if err == nil {
			t.Fatal("expected ToDo to be deleted when reference document was deleted")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("delete cascade failed: %v", err)
	}
}

func TestAssignment_DoesNotGrantDocumentAccess(t *testing.T) {
	e := setupWith(t, nil)
	ctx := context.Background()

	// Create user 'ze' with only 'Atendente' role (cannot read Pessoa)
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{
			"email":      "ze@x.com",
			"first_name": "Ze",
			"full_name":  "Ze Atendente",
			"enabled":    true,
		})
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}

		// Create a Pessoa that Ze cannot read
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Confidencial"})
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			return err
		}

		// Assign it to Ze
		todo, _ := c.NewDoc("ToDo", Doc{
			"allocated_to":   "ze@x.com",
			"reference_type": "Pessoa",
			"reference_name": "Confidencial",
			"description":    "Confidential task",
		})
		if _, err := c.Insert(todo, SaveOpts{}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Now run as 'ze@x.com':
	// Ze tries to read Pessoa "Confidencial" -> must fail!
	err = e.Run(ctx, "ze@x.com", func(c *Ctx) error {
		_, err := c.GetDoc("Pessoa", "Confidencial")
		if err == nil {
			t.Fatal("expected GetDoc to fail for user ze without permission")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error running as ze: %v", err)
	}

	// Ze tries to read ToDo -> must fail because ze cannot read Pessoa Confidencial!
	err = e.Run(ctx, "ze@x.com", func(c *Ctx) error {
		todos, err := c.GetList("ToDo", ListArgs{
			Filters: map[string]any{
				"reference_type": "Pessoa",
				"reference_name": "Confidencial",
			},
		})
		if err != nil {
			return nil // Permission error or similar is also acceptable
		}
		// If returned, each item must be checked by hasPermission
		for _, row := range todos {
			doc, err := c.GetDoc("ToDo", fmt.Sprint(row["name"]))
			if err == nil {
				t.Fatalf("expected GetDoc on ToDo to fail for unauthorized referenced doc, got %v", doc)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssignment_DueDateReminder(t *testing.T) {
	e := setupWith(t, nil)
	ctx := context.Background()

	// 1. Create a user 'ana@x.com'
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{
			"email":      "ana@x.com",
			"first_name": "Ana",
			"full_name":  "Ana Gestora",
			"enabled":    true,
		})
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}

		today := time.Now().Format("2006-01-02")
		todo, err := c.NewDoc("ToDo", Doc{
			"allocated_to": "ana@x.com",
			"status":       "Open",
			"date":         today,
			"description":  "Tax filing deadline",
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(todo, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 2. Run notification sweep
	if err := e.SweepNotifications(ctx, time.Now()); err != nil {
		t.Fatalf("sweep failed: %v", err)
	}

	// 3. Verify ddcore_notification has the reminder for ana@x.com
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT title, message FROM ddcore_notification WHERE recipient='ana@x.com'`)
	if err != nil {
		t.Fatalf("failed to query notifications: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 notification for ana, got %d", len(rows))
	}

	// 4. Second sweep must not duplicate
	if err := e.SweepNotifications(ctx, time.Now()); err != nil {
		t.Fatalf("second sweep failed: %v", err)
	}
	rows, _ = db.Select(ctx, e.DB.Pool, `SELECT title, message FROM ddcore_notification WHERE recipient='ana@x.com'`)
	if len(rows) != 1 {
		t.Fatalf("expected still 1 notification after second sweep, got %d", len(rows))
	}
}


