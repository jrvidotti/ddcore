package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeExport lays out a directory the way `ddcore export` does: one NDJSON
// file per DocType, each closed by its _manifest line, plus manifest.json.
func writeExport(t *testing.T, files map[string][]Doc) string {
	t.Helper()
	dir := t.TempDir()
	m := &ExportManifest{DDCore: "v0.17.0", Layout: ExportLayout, Format: "ndjson", User: "Admin"}
	for doctype, docs := range files {
		name := strings.ReplaceAll(doctype, " ", "-") + ".ndjson"
		var b strings.Builder
		for _, d := range docs {
			line, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			b.Write(line)
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "{\"_manifest\":{\"doctype\":%q,\"rows\":%d}}\n", doctype, len(docs))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		m.Exports = append(m.Exports, &ExportResult{
			Summary: &ExportSummary{Doctype: doctype, Rows: int64(len(docs))},
			Outputs: []ExportOutput{{File: name}},
		})
	}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func clientes(n int) []Doc {
	var out []Doc
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("C-%03d", i)
		out = append(out, Doc{"doctype": "Cliente", "id": id, "code": id, "nome": "Cliente " + id, "owner": "op@x.com"})
	}
	return out
}

func runImport(t *testing.T, e *Engine, dir string, args ImportArgs) *ImportRun {
	t.Helper()
	args.Dir = dir
	if args.Actor == "" {
		args.Actor = "test"
	}
	run, err := e.Import(context.Background(), args)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return run
}

func TestImportLoadsAnExport(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(7)})
	run := runImport(t, e, dir, ImportArgs{Batch: 3})
	if run.Status != ImportCompleted {
		t.Fatalf("status = %s (%s)", run.Status, run.Message)
	}
	if got := run.Counts["Cliente"].Loaded; got != 7 {
		t.Fatalf("loaded = %d", got)
	}
	if got := scalar(t, e, "SELECT count(*) FROM tab_cliente"); got != int64(7) {
		t.Fatalf("rows = %v", got)
	}
}

// Running the same export twice is the ordinary case: a rehearsal, then the
// cutover. The second run must write nothing.
func TestImportIsIdempotent(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(4)})
	runImport(t, e, dir, ImportArgs{Batch: 2})
	second := runImport(t, e, dir, ImportArgs{Batch: 2})
	if second.Counts["Cliente"].Loaded != 0 || second.Counts["Cliente"].Skipped != 4 {
		t.Fatalf("second run = %+v", second.Counts["Cliente"])
	}
	if got := scalar(t, e, "SELECT count(*) FROM tab_cliente"); got != int64(4) {
		t.Fatalf("rows = %v", got)
	}
}

// A run that stops halfway leaves a cursor, and resuming finishes the job
// without loading anything twice.
func TestImportResumesWhereItStopped(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(10)})
	part := runImport(t, e, dir, ImportArgs{Batch: 2, MaxBatches: 2})
	if part.Status != ImportPaused {
		t.Fatalf("status = %s", part.Status)
	}
	if got := scalar(t, e, "SELECT count(*) FROM tab_cliente"); got != int64(4) {
		t.Fatalf("after two batches = %v", got)
	}
	done := runImport(t, e, dir, ImportArgs{Batch: 2, Resume: part.ID})
	if done.Status != ImportCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Message)
	}
	if got := scalar(t, e, "SELECT count(*) FROM tab_cliente"); got != int64(10) {
		t.Fatalf("rows = %v", got)
	}
	if got := scalar(t, e, "SELECT count(*) FROM ddcore_import_record"); got != int64(10) {
		t.Fatalf("ledger rows = %v", got)
	}
}

// One unloadable record must not cost the batch it is in.
func TestImportRecordsPerRecordErrors(t *testing.T) {
	e := setupImport(t)
	docs := clientes(3)
	docs[1] = Doc{"doctype": "Cliente", "id": "C-BAD", "nome": "no code"} // code is required
	dir := writeExport(t, map[string][]Doc{"Cliente": docs})
	run := runImport(t, e, dir, ImportArgs{Batch: 5})
	if run.Status != ImportCompletedWithErrors {
		t.Fatalf("status = %s", run.Status)
	}
	if run.Counts["Cliente"].Errors != 1 || run.Counts["Cliente"].Loaded != 2 {
		t.Fatalf("counts = %+v", run.Counts["Cliente"])
	}
	if got := scalar(t, e, "SELECT count(*) FROM ddcore_import_error WHERE run_id = $1", run.ID); got != int64(1) {
		t.Fatalf("error rows = %v", got)
	}
}

// A dry run answers the same questions and leaves nothing behind.
func TestImportDryRunWritesNothing(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(5)})
	run := runImport(t, e, dir, ImportArgs{DryRun: true})
	if run.Counts["Cliente"].Loaded != 5 {
		t.Fatalf("dry run counts = %+v", run.Counts["Cliente"])
	}
	for _, q := range []string{"SELECT count(*) FROM tab_cliente", "SELECT count(*) FROM ddcore_import_record"} {
		if got := scalar(t, e, q); got != int64(0) {
			t.Fatalf("%s = %v", q, got)
		}
	}
}

// A document the site already holds is not a failure by itself; identical
// content is the common case, because migrate seeds Roles and Admin before
// any import runs.
func TestImportOnExistingPolicies(t *testing.T) {
	e := setupImport(t)
	if err := importAs(t, e, "Admin", Doc{"doctype": "Cliente", "id": "C-001", "code": "C-001", "nome": "Cliente C-001", "owner": "op@x.com"}); err != nil {
		t.Fatal(err)
	}
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(2)})
	run := runImport(t, e, dir, ImportArgs{})
	if run.Counts["Cliente"].Skipped != 1 || run.Counts["Cliente"].Loaded != 1 {
		t.Fatalf("identical should skip: %+v", run.Counts["Cliente"])
	}

	e2 := setupImport(t)
	if err := importAs(t, e2, "Admin", Doc{"doctype": "Cliente", "id": "C-001", "code": "C-001", "nome": "SOMETHING ELSE"}); err != nil {
		t.Fatal(err)
	}
	run2 := runImport(t, e2, dir, ImportArgs{})
	if run2.Counts["Cliente"].Errors != 1 {
		t.Fatalf("a different document is a conflict: %+v", run2.Counts["Cliente"])
	}
}

// The order is not the manifest's: a link's target has to be there first.
func TestImportLoadsInDependencyOrder(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{
		"Fatura":  {{"doctype": "Fatura", "id": "FAT-0001", "cliente": "C-001", "total": 9}},
		"Cliente": clientes(1),
	})
	run := runImport(t, e, dir, ImportArgs{})
	if run.Status != ImportCompleted {
		t.Fatalf("status = %s (%s); errors %+v", run.Status, run.Message, run.Errors)
	}
	if got := scalar(t, e, "SELECT cliente FROM tab_fatura WHERE id = $1", "FAT-0001"); got != "C-001" {
		t.Fatalf("cliente = %v", got)
	}
}

// A self link is deferred past the load and then verified; one that never
// resolves is a finding, not a silent gap.
func TestImportVerifiesDeferredLinks(t *testing.T) {
	e := setupImport(t)
	docs := clientes(2)
	docs[0]["indicado_por"] = "C-002" // forward, only satisfiable after the load
	dir := writeExport(t, map[string][]Doc{"Cliente": docs})
	run := runImport(t, e, dir, ImportArgs{})
	if run.Status != ImportCompleted || len(run.Dangling) != 0 {
		t.Fatalf("status = %s dangling = %+v", run.Status, run.Dangling)
	}

	e2 := setupImport(t)
	bad := clientes(1)
	bad[0]["indicado_por"] = "C-999"
	dir2 := writeExport(t, map[string][]Doc{"Cliente": bad})
	run2 := runImport(t, e2, dir2, ImportArgs{})
	if run2.Status != ImportCompletedWithErrors || len(run2.Dangling) != 1 {
		t.Fatalf("status = %s dangling = %+v", run2.Status, run2.Dangling)
	}
}

// The mapping is what makes somebody else's export loadable here.
func TestImportAppliesTheMapping(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Customer": {
		{"doctype": "Customer", "id": "OLD-1", "customer_code": "C-900", "customer_name": "Remapped"},
	}})
	m, err := ParseImportMap([]byte(`{"doctypes":{"Customer":{"to":"Cliente",
	  "fields":{"customer_code":"code","customer_name":"nome"},"ids":{"OLD-1":"C-900"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	run := runImport(t, e, dir, ImportArgs{Map: m})
	if run.Status != ImportCompleted {
		t.Fatalf("status = %s (%s) %+v", run.Status, run.Message, run.Errors)
	}
	if got := scalar(t, e, "SELECT nome FROM tab_cliente WHERE id = $1", "C-900"); got != "Remapped" {
		t.Fatalf("nome = %v", got)
	}
}

// Some DocTypes are records of effects that already happened; loading them
// would either fail or start firing.
func TestImportExcludesTheLedgersByDefault(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{
		"Cliente":   clientes(1),
		"Error Log": {{"doctype": "Error Log", "id": "E-1", "message": "boom"}},
	})
	run := runImport(t, e, dir, ImportArgs{})
	if _, loaded := run.Counts["Error Log"]; loaded {
		t.Fatalf("Error Log should be skipped by default: %+v", run.Counts)
	}
	if len(run.Excluded) == 0 {
		t.Fatal("the run should say what it left out and why")
	}
}
