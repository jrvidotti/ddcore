package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/testdb"
)

// setupTenantImport is setupImport on a site with tenancy: the import
// fixture plus a shared DocType, and the tenants alfa and beta, empty. Its own
// database, like setupImport's.
func setupTenantImport(t *testing.T) *Engine {
	t.Helper()
	dir := importApp(t)
	src := `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Moeda", shared: true, idGeneration: { field: "sigla" },
  fields: [{ fieldname: "sigla", fieldtype: "Data", label: "Code", reqd: true }],
  permissions: [{ role: "Operador", read: true, write: true, create: true }] });`
	// and one that lives inside a tenant (#105)
	contrato := `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Contrato", space: "tenant", idGeneration: { field: "numero" },
  fields: [{ fieldname: "numero", fieldtype: "Data", label: "Number", reqd: true }],
  permissions: [{ role: "Operador", read: true, write: true, create: true }] });`
	for rel, src := range map[string]string{"doctypes/moeda/moeda.doctype.ts": src, "doctypes/contrato/contrato.doctype.ts": contrato} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dsn := testdb.WithDatabase(testDSN, testdb.Database(testDSN)+"_impt")
	e := migratedEngine(t, Config{DSN: dsn, Apps: []js.App{{Name: "imp", Dir: dir}}, Test: true, Tenancy: true, DataDir: t.TempDir()})
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, id := range []string{tenantA, tenantB} {
			if err := insertDoc(c, "Site Tenant", Doc{"slug": id, "title": strings.ToUpper(id)}); err != nil {
				return err
			}
		}
		return nil
	})
	return e
}

// sysScalar is scalar seen from outside every space.
func sysScalar(t *testing.T, e *Engine, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := e.DB.Sys.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func faturasDe(ids ...string) []Doc {
	var out []Doc
	for _, id := range ids {
		out = append(out, Doc{"doctype": "Fatura", "id": id, "cliente": "C-001", "total": 10})
	}
	return out
}

// A load into a tenant writes the tenant's rows, its ledger and its audit
// events, and moves its series; the platform space gets none of it.
func TestImportIntoATenantLandsInTheTenant(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(3), "Fatura": faturasDe("FAT-0001", "FAT-0002")})
	if dry := runImport(t, e, dir, ImportArgs{Tenant: tenantA, DryRun: true}); dry.Status != ImportCompleted || dry.Counts["Fatura"].Loaded != 2 {
		t.Fatalf("dry run: %s %+v", dry.Status, dry.Errors)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_cliente`); got != 0 {
		t.Fatalf("the dry run left %d Cliente", got)
	}
	run := runImport(t, e, dir, ImportArgs{Tenant: tenantA, Batch: 2})
	if run.Status != ImportCompleted || run.Tenant != tenantA {
		t.Fatalf("status = %s tenant = %q (%s) %+v", run.Status, run.Tenant, run.Message, run.Errors)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_cliente WHERE tenant = $1`, tenantA); got != 3 {
		t.Fatalf("alfa has %d Cliente", got)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_cliente WHERE tenant = ''`); got != 0 {
		t.Fatalf("the platform space has %d Cliente", got)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM ddcore_import_record WHERE tenant = $1`, tenantA); got != 5 {
		t.Fatalf("alfa's ledger has %d rows", got)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_audit_event WHERE action = 'import.run' AND tenant = $1`, tenantA); got != 2 {
		t.Fatalf("alfa has %d import.run audit events", got)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM ddcore_import_run WHERE id = $1 AND tenant = $2`, run.ID, tenantA); got != 1 {
		t.Fatal("the run does not record its tenant")
	}
	// the series continues in the tenant, and the platform's starts at one
	next := func(c *Ctx) string {
		d, err := c.NewDoc("Fatura", Doc{"cliente": "C-001"})
		if err != nil {
			t.Fatal(err)
		}
		if d, err = c.Insert(d, SaveOpts{IgnorePermissions: true, IgnoreLinks: true}); err != nil {
			t.Fatal(err)
		}
		return d.ID()
	}
	inTenant(t, e, tenantA, func(c *Ctx) error {
		if id := next(c); id != "FAT-0003" {
			t.Fatalf("alfa's next Fatura is %s", id)
		}
		return nil
	})
	inTenant(t, e, "", func(c *Ctx) error {
		if id := next(c); id != "FAT-0001" {
			t.Fatalf("the platform's next Fatura is %s", id)
		}
		return nil
	})
}

// The ledger is per tenant: one export loads into two tenants, and a second
// run into either writes nothing.
func TestImportTheSameExportIntoTwoTenants(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(3)})
	for _, tenant := range []string{tenantA, tenantB} {
		run := runImport(t, e, dir, ImportArgs{Tenant: tenant})
		if run.Status != ImportCompleted || run.Counts["Cliente"].Loaded != 3 {
			t.Fatalf("%s: %s %+v %+v", tenant, run.Status, run.Counts["Cliente"], run.Errors)
		}
	}
	again := runImport(t, e, dir, ImportArgs{Tenant: tenantA})
	if c := again.Counts["Cliente"]; c.Loaded != 0 || c.Skipped != 3 {
		t.Fatalf("second run into alfa = %+v", c)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM ddcore_import_record`); got != 6 {
		t.Fatalf("ledger rows = %d", got)
	}
}

// What a tenant cannot hold is left out: a shared DocType is the platform's,
// and so are Admin and Guest.
func TestImportIntoATenantLeavesOutWhatIsNotItsOwn(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{
		"Cliente": clientes(1),
		"Moeda":   {{"doctype": "Moeda", "id": "BRL", "sigla": "BRL"}},
		"User": {
			{"doctype": "User", "id": "Admin", "email": "admin@example.com", "full_name": "Administrator"},
			{"doctype": "User", "id": "ana@alfa.test", "email": "ana@alfa.test", "full_name": "Ana"},
		},
	})
	run := runImport(t, e, dir, ImportArgs{Tenant: tenantA})
	if run.Status != ImportCompleted {
		t.Fatalf("status = %s (%s) %+v", run.Status, run.Message, run.Errors)
	}
	excluded := false
	for _, x := range run.Excluded {
		if x.Doctype == "Moeda" && strings.HasPrefix(x.Reason, "shared:") {
			excluded = true
		}
	}
	if !excluded {
		t.Fatalf("the shared DocType was not left out: %+v", run.Excluded)
	}
	if c := run.Counts["User"]; c.Loaded != 1 || c.Skipped != 1 {
		t.Fatalf("users = %+v", c)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_user WHERE tenant = $1`, tenantA); got != 1 {
		t.Fatalf("alfa has %d users", got)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_moeda`); got != 0 {
		t.Fatalf("%d Moeda were loaded", got)
	}
	// the write path refuses a shared DocType inside a tenant on its own
	err := e.Run(WithTenant(context.Background(), tenantA), "Admin", func(c *Ctx) error {
		_, err := c.ImportDoc(Doc{"doctype": "Moeda", "id": "USD", "sigla": "USD"}, ImportOpts{})
		return err
	})
	wantStatus(t, err, 403)
	// what the tenant was given reconciles, Admin aside
	rep, err := e.ImportReconcile(context.Background(), ImportArgs{Dir: dir, Tenant: tenantA})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("mismatches: %+v", rep.Mismatches)
	}
}

// A load into the platform space leaves out what lives inside a tenant, and
// says so; the same export loads it into a tenant.
func TestImportIntoThePlatformLeavesOutTenantOnlyDocTypes(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{
		"Cliente":  clientes(1),
		"Moeda":    {{"doctype": "Moeda", "id": "BRL", "sigla": "BRL"}},
		"Contrato": {{"doctype": "Contrato", "id": "CT-1", "numero": "CT-1"}},
	})
	run := runImport(t, e, dir, ImportArgs{})
	if run.Status != ImportCompleted {
		t.Fatalf("status = %s (%s) %+v", run.Status, run.Message, run.Errors)
	}
	excluded := false
	for _, x := range run.Excluded {
		if x.Doctype == "Contrato" && strings.Contains(x.Reason, "--tenant") {
			excluded = true
		}
	}
	if !excluded {
		t.Fatalf("the tenant-only DocType was not left out: %+v", run.Excluded)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_moeda`); got != 1 {
		t.Fatalf("%d Moeda were loaded into the platform", got)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_contrato`); got != 0 {
		t.Fatalf("%d Contrato were loaded into the platform", got)
	}
	into := runImport(t, e, dir, ImportArgs{Tenant: tenantA})
	if into.Status != ImportCompleted || into.Counts["Contrato"].Loaded != 1 {
		t.Fatalf("into alfa: %s %+v %+v", into.Status, into.Counts["Contrato"], into.Errors)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_contrato WHERE tenant = $1`, tenantA); got != 1 {
		t.Fatalf("alfa has %d Contrato", got)
	}
}

// Reconcile and the list of runs look at the space they are asked about.
func TestImportReconcileAndStatusAreScopedToTheTenant(t *testing.T) {
	e := setupTenantImport(t)
	ctx := context.Background()
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(2)})
	run := runImport(t, e, dir, ImportArgs{Tenant: tenantA})

	rep, err := e.ImportReconcile(ctx, ImportArgs{Dir: dir, Tenant: tenantA})
	if err != nil || !rep.OK || rep.Doctypes["Cliente"].TargetRows != 2 {
		t.Fatalf("alfa: %+v %v", rep, err)
	}
	for _, tenant := range []string{"", tenantB} {
		rep, err := e.ImportReconcile(ctx, ImportArgs{Dir: dir, Tenant: tenant})
		if err != nil || rep.OK {
			t.Fatalf("%q reconciles alfa's load: %+v %v", tenant, rep, err)
		}
	}

	runs, err := e.ImportRuns(WithTenant(ctx, tenantA), 10)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID || runs[0].Tenant != tenantA {
		t.Fatalf("alfa's runs: %+v %v", runs, err)
	}
	if runs, err := e.ImportRuns(ctx, 10); err != nil || len(runs) != 0 {
		t.Fatalf("the platform space lists %+v %v", runs, err)
	}
	if got, err := e.ImportRunByID(ctx, run.ID); err != nil || got.Tenant != tenantA {
		t.Fatalf("by id: %+v %v", got, err)
	}
}

// A run resumes in the space it started in, and nowhere else.
func TestImportResumeRefusesAnotherTenant(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(5)})
	part := runImport(t, e, dir, ImportArgs{Tenant: tenantA, Batch: 2, MaxBatches: 1})
	if part.Status != ImportPaused {
		t.Fatalf("status = %s", part.Status)
	}
	for _, tenant := range []string{tenantB, ""} {
		_, err := e.Import(context.Background(), ImportArgs{Dir: dir, Tenant: tenant, Resume: part.ID, Actor: "test"})
		wantStatus(t, err, 417)
		if !strings.Contains(err.Error(), "--tenant") {
			t.Fatalf("the refusal does not say how to resume: %v", err)
		}
	}
	done := runImport(t, e, dir, ImportArgs{Tenant: tenantA, Batch: 2, Resume: part.ID})
	if done.Status != ImportCompleted {
		t.Fatalf("status = %s (%s)", done.Status, done.Message)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_cliente WHERE tenant = $1`, tenantA); got != 5 {
		t.Fatalf("alfa has %d Cliente", got)
	}
}

// A named tenant has to exist, and be enabled, before anything is written.
func TestImportRefusesAnUnknownTenant(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(1)})
	_, err := e.Import(context.Background(), ImportArgs{Dir: dir, Tenant: "nobody", Actor: "test"})
	wantStatus(t, err, 404)
	if got := sysScalar(t, e, `SELECT count(*) FROM ddcore_import_run`); got != 0 {
		t.Fatalf("%d runs were recorded", got)
	}
}

// An attachment's storage key is site-wide: a url another tenant's File holds
// is not overwritten by a load into this one.
func TestImportRefusesAFileOfAnotherTenant(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(1)})
	f := attach(t, dir, "/private/files/nota.txt", "hello")
	rewriteLine(t, dir, "Cliente.ndjson", 0, func(d Doc) { d["_files"] = []any{f} })

	if run := runImport(t, e, dir, ImportArgs{Tenant: tenantA}); run.Status != ImportCompleted {
		t.Fatalf("alfa: %s %+v", run.Status, run.Errors)
	}
	run := runImport(t, e, dir, ImportArgs{Tenant: tenantB})
	if run.Status != ImportCompletedWithErrors || len(run.Errors) != 1 || !strings.Contains(run.Errors[0].Message, "another tenant") {
		t.Fatalf("beta: %s %+v", run.Status, run.Errors)
	}
	if got := sysScalar(t, e, `SELECT count(*) FROM tab_cliente WHERE tenant = $1`, tenantB); got != 0 {
		t.Fatalf("beta kept the record whose file it could not load: %d", got)
	}
}

// An adopt takes the platform space's ledger along, so loading the same
// export into the tenant afterwards writes nothing.
func TestTenantAdoptMovesTheImportLedger(t *testing.T) {
	e := setupTenantImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(3)})
	if run := runImport(t, e, dir, ImportArgs{}); run.Status != ImportCompleted || run.Tenant != "" {
		t.Fatalf("platform load: %s %q", run.Status, run.Tenant)
	}
	moved, err := e.AdoptPlatformRows(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if moved["ddcore_import_record"] != 3 {
		t.Fatalf("moved = %v", moved)
	}
	again := runImport(t, e, dir, ImportArgs{Tenant: tenantA})
	if c := again.Counts["Cliente"]; c.Loaded != 0 || c.Skipped != 3 {
		t.Fatalf("after the adopt = %+v", c)
	}
}

// Without tenancy there is no tenant to name.
func TestImportRefusesATenantWithoutTenancy(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(1)})
	_, err := e.Import(context.Background(), ImportArgs{Dir: dir, Tenant: tenantA, Actor: "test"})
	wantStatus(t, err, 417)
	_, err = e.ImportReconcile(context.Background(), ImportArgs{Dir: dir, Tenant: tenantA})
	wantStatus(t, err, 417)
}
