package engine

import (
	"context"
	"testing"
)

func reconcile(t *testing.T, e *Engine, dir string, args ImportArgs) *ImportReconciliation {
	t.Helper()
	args.Dir = dir
	rec, err := e.ImportReconcile(context.Background(), args)
	if err != nil {
		t.Fatalf("ImportReconcile: %v", err)
	}
	return rec
}

func faturas(t *testing.T) map[string][]Doc {
	return map[string][]Doc{
		"Cliente": clientes(2),
		"Fatura": {
			{"doctype": "Fatura", "id": "FAT-0001", "cliente": "C-001", "total": 10.5, "docstatus": 1,
				"itens": []any{map[string]any{"doctype": "Item Fatura", "id": "r1", "descricao": "a", "preco": 10.5, "idx": 1}}},
			{"doctype": "Fatura", "id": "FAT-0002", "cliente": "C-002", "total": 20.25, "docstatus": 0},
		},
	}
}

func TestImportReconcileAgreesWithACleanLoad(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, faturas(t))
	runImport(t, e, dir, ImportArgs{})
	rec := reconcile(t, e, dir, ImportArgs{})
	if !rec.OK {
		t.Fatalf("mismatches on a clean load: %+v", rec.Mismatches)
	}
	if rec.Doctypes["Fatura"].SourceRows != 2 || rec.Doctypes["Fatura"].TargetRows != 2 {
		t.Fatalf("Fatura = %+v", rec.Doctypes["Fatura"])
	}
	if rec.Doctypes["Fatura"].ChildRows["Item Fatura"] != 1 {
		t.Fatalf("child rows = %+v", rec.Doctypes["Fatura"].ChildRows)
	}
}

// Money is the reason a reconciliation exists: the totals have to agree, per
// docstatus, or the load moved an amount.
func TestImportReconcileCatchesAChangedAmount(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, faturas(t))
	runImport(t, e, dir, ImportArgs{})
	if _, err := e.DB.Pool.Exec(context.Background(), "UPDATE tab_fatura SET total = total + 1 WHERE id = $1", "FAT-0001"); err != nil {
		t.Fatal(err)
	}
	rec := reconcile(t, e, dir, ImportArgs{})
	if rec.OK {
		t.Fatal("a changed total must be a mismatch")
	}
	found := false
	for _, m := range rec.Mismatches {
		if m.Kind == "total" && m.Doctype == "Fatura" {
			found = true
		}
	}
	if !found {
		t.Fatalf("mismatches = %+v", rec.Mismatches)
	}
}

func TestImportReconcileCatchesMissingRowsAndChildren(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, faturas(t))
	runImport(t, e, dir, ImportArgs{})
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, "DELETE FROM tab_item_fatura WHERE parent = $1", "FAT-0001"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Pool.Exec(ctx, "DELETE FROM tab_fatura WHERE id = $1", "FAT-0002"); err != nil {
		t.Fatal(err)
	}
	rec := reconcile(t, e, dir, ImportArgs{})
	kinds := map[string]bool{}
	for _, m := range rec.Mismatches {
		kinds[m.Kind] = true
	}
	if !kinds["rows"] || !kinds["childRows"] {
		t.Fatalf("mismatches = %+v", rec.Mismatches)
	}
}

// docstatus is history too: a cancelled document that came back as a draft is
// a different document.
func TestImportReconcileComparesDocstatus(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, faturas(t))
	runImport(t, e, dir, ImportArgs{})
	if _, err := e.DB.Pool.Exec(context.Background(), "UPDATE tab_fatura SET docstatus = 0 WHERE id = $1", "FAT-0001"); err != nil {
		t.Fatal(err)
	}
	rec := reconcile(t, e, dir, ImportArgs{})
	found := false
	for _, m := range rec.Mismatches {
		if m.Kind == "docstatus" {
			found = true
		}
	}
	if !found {
		t.Fatalf("mismatches = %+v", rec.Mismatches)
	}
}

// Reconciling before anything is loaded says so plainly, rather than counting
// zero against zero and calling it agreement.
func TestImportReconcileNoticesNothingWasLoaded(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, faturas(t))
	rec := reconcile(t, e, dir, ImportArgs{})
	if rec.OK {
		t.Fatalf("an empty site cannot reconcile with an export: %+v", rec)
	}
}
