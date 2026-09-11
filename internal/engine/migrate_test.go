package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// DAT-04 — a rename declared in the meta moves the column, a conversion that
// could lose data is refused rather than guessed, and the drops wait for the
// patches.

// migrar runs a migration and insists it left nothing behind: a plan that is
// not empty afterwards is a plan that will run again.
func migrar(t *testing.T, e *Engine, prune bool, step string) *MigrateResult {
	t.Helper()
	ctx := context.Background()
	res, err := e.Migrate(ctx, prune)
	if err != nil {
		t.Fatalf("%s: migrate: %v", step, err)
	}
	plan, err := e.Plan(ctx, prune)
	if err != nil {
		t.Fatalf("%s: plan after migrate: %v", step, err)
	}
	if len(plan) != 0 {
		t.Fatalf("%s: migrate not idempotent, %d left: %v", step, len(plan), db.SQL(plan))
	}
	return res
}

// sql is a read-only query straight at the pool, for asserting on the catalog.
func sqlRows(t *testing.T, e *Engine, query string, args ...any) []map[string]any {
	t.Helper()
	rows, err := db.Select(context.Background(), e.DB.Pool, query, args...)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return rows
}

func columnType(t *testing.T, e *Engine, table, col string) string {
	t.Helper()
	rows := sqlRows(t, e, `SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`, table, col)
	if len(rows) == 0 {
		return ""
	}
	return db.Str(rows[0]["data_type"])
}

func indexDef(t *testing.T, e *Engine, name string) string {
	t.Helper()
	rows := sqlRows(t, e, `SELECT indexdef FROM pg_indexes WHERE indexname = $1`, name)
	if len(rows) == 0 {
		return ""
	}
	return db.Str(rows[0]["indexdef"])
}

func insertPessoa(t *testing.T, e *Engine, values Doc) string {
	t.Helper()
	var name string
	err := e.Run(context.Background(), "Administrator", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		doc, err := c.NewDoc("Pessoa", values)
		if err != nil {
			return err
		}
		n, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
		name = n.Str("name")
		return err
	})
	if err != nil {
		t.Fatalf("insert Pessoa: %v", err)
	}
	return name
}

// TestRenameFieldKeepsTheData is the defect DAT-04 names: before this, renaming
// a field produced an empty new column beside the old one, and --prune dropped
// the old one in the same transaction, before any patch could copy it.
func TestRenameFieldKeepsTheData(t *testing.T) {
	e := setup(t)
	insertPessoa(t, e, Doc{"nome": "Ana", "cpf": "12345678901"})

	pessoa, _ := e.Meta.Get("Pessoa")
	cpf := pessoa.Field("cpf")
	cpf.Fieldname = "documento"
	cpf.RenamedFrom = meta.Names{"cpf"}
	pessoa.ResetFieldIndex()

	res := migrar(t, e, true, "rename cpf → documento")

	rows := sqlRows(t, e, `SELECT documento FROM tab_pessoa WHERE nome = 'Ana'`)
	if len(rows) != 1 || db.Str(rows[0]["documento"]) != "12345678901" {
		t.Fatalf("the value did not survive the rename: %v", rows)
	}
	if columnType(t, e, "tab_pessoa", "cpf") != "" {
		t.Fatal("the old column is still there")
	}
	// The rename must be a rename, not an add-and-drop: prune was on, so a
	// plan that missed it would have emptied the column instead.
	var renamed bool
	for _, st := range res.DDL {
		if st.Kind == db.KindRenameColumn && st.OldName == "cpf" && st.Column == "documento" {
			renamed = true
		}
		if st.Destructive {
			t.Fatalf("a rename should need no drop, got %q", st.SQL)
		}
	}
	if !renamed {
		t.Fatalf("no RENAME COLUMN in the plan: %v", db.SQL(res.DDL))
	}
}

// TestRenameFieldRenamesItsIndex pins the subtle half: Postgres reports the
// renamed index under its old definition, so comparing it against the wanted
// one would drop and rebuild an index that did not change — expensive, and on a
// unique index a window with no constraint.
func TestRenameFieldRenamesItsIndex(t *testing.T) {
	e := setup(t)
	before := indexDef(t, e, "tab_pessoa_cpf")
	if !strings.Contains(before, "UNIQUE") {
		t.Fatalf("expected a unique index to start from, got %q", before)
	}

	pessoa, _ := e.Meta.Get("Pessoa")
	cpf := pessoa.Field("cpf")
	cpf.Fieldname = "documento"
	cpf.RenamedFrom = meta.Names{"cpf"}
	pessoa.ResetFieldIndex()

	res := migrar(t, e, false, "rename with index")

	if indexDef(t, e, "tab_pessoa_cpf") != "" {
		t.Fatal("the old index name survived")
	}
	after := indexDef(t, e, "tab_pessoa_documento")
	if !strings.Contains(after, "UNIQUE") || !strings.Contains(after, "documento") {
		t.Fatalf("the renamed index is not the unique index on documento: %q", after)
	}
	for _, st := range res.DDL {
		if st.Kind == db.KindDropIndex {
			t.Fatalf("the index was rebuilt instead of renamed: %q", st.SQL)
		}
	}
}

// TestRenameFieldChain covers a database that skipped a release: the field is
// on its second name in the meta and its first in the catalog.
func TestRenameFieldChain(t *testing.T) {
	e := setup(t)
	insertPessoa(t, e, Doc{"nome": "Bia", "codigo": "X1"})

	pessoa, _ := e.Meta.Get("Pessoa")
	f := pessoa.Field("codigo")
	f.Fieldname = "referencia"
	f.RenamedFrom = meta.Names{"codigo", "cod_interno"}
	pessoa.ResetFieldIndex()

	migrar(t, e, true, "chain")

	rows := sqlRows(t, e, `SELECT referencia FROM tab_pessoa WHERE nome = 'Bia'`)
	if len(rows) != 1 || db.Str(rows[0]["referencia"]) != "X1" {
		t.Fatalf("the chained rename lost the value: %v", rows)
	}
}

// TestRenameFieldAndReuseTheOldName is the ordering case: the rename has to run
// before the ADD COLUMN, or the new column occupies the name the old one needs.
func TestRenameFieldAndReuseTheOldName(t *testing.T) {
	e := setup(t)
	insertPessoa(t, e, Doc{"nome": "Caio", "codigo": "antigo"})

	pessoa, _ := e.Meta.Get("Pessoa")
	f := pessoa.Field("codigo")
	f.Fieldname = "codigo_legado"
	f.RenamedFrom = meta.Names{"codigo"}
	pessoa.Fields = append(pessoa.Fields, &meta.Field{
		Fieldname: "codigo", Fieldtype: "Data", Label: "Código",
	})
	pessoa.ResetFieldIndex()

	migrar(t, e, true, "rename and reuse")

	rows := sqlRows(t, e, `SELECT codigo, codigo_legado FROM tab_pessoa WHERE nome = 'Caio'`)
	if len(rows) != 1 {
		t.Fatal("row gone")
	}
	if got := db.Str(rows[0]["codigo_legado"]); got != "antigo" {
		t.Fatalf("the old value should have moved to codigo_legado, got %q", got)
	}
	if got := db.Str(rows[0]["codigo"]); got != "" {
		t.Fatalf("the reused name should start empty, got %q", got)
	}
}

// TestRefusesUndeclaredConversion — text to numeric cannot be done without
// knowing the data: one unparseable row aborts the whole ALTER, and Postgres
// gives no route forward. The refusal replaces that with something actionable.
func TestRefusesUndeclaredConversion(t *testing.T) {
	e := setup(t)
	insertPessoa(t, e, Doc{"nome": "Dora", "codigo": "not a number"})

	pessoa, _ := e.Meta.Get("Pessoa")
	pessoa.Field("codigo").Fieldtype = "Currency"

	_, err := e.Migrate(context.Background(), false)
	if err == nil {
		t.Fatal("migrate ran a conversion nobody authorised")
	}
	if !strings.Contains(err.Error(), "codigo") || !strings.Contains(err.Error(), "convert") {
		t.Fatalf("the refusal should name the column and the way out, got: %v", err)
	}
	if got := columnType(t, e, "tab_pessoa", "codigo"); got != "text" {
		t.Fatalf("nothing should have been applied, column is %q", got)
	}
}

// TestDeclaredConversionRuns — the same change, authorised, on data the cast
// can take.
func TestDeclaredConversionRuns(t *testing.T) {
	e := setup(t)
	insertPessoa(t, e, Doc{"nome": "Eva", "codigo": "42"})

	pessoa, _ := e.Meta.Get("Pessoa")
	f := pessoa.Field("codigo")
	f.Fieldtype = "Int"
	f.Convert = &meta.Convert{From: "Data"}

	migrar(t, e, false, "declared conversion")

	if got := columnType(t, e, "tab_pessoa", "codigo"); got != "bigint" {
		t.Fatalf("column is %q, wanted bigint", got)
	}
	rows := sqlRows(t, e, `SELECT codigo FROM tab_pessoa WHERE nome = 'Eva'`)
	if len(rows) != 1 || db.Str(rows[0]["codigo"]) != "42" {
		t.Fatalf("the value did not convert: %v", rows)
	}
}

// TestSafeWideningNeedsNoDeclaration — Int to Float and anything to text hold
// every value they had, so they are not the author's problem.
func TestSafeWideningNeedsNoDeclaration(t *testing.T) {
	e := setup(t)
	item, _ := e.Meta.Get("Item Pedido")
	item.Field("qtd").Fieldtype = "Float"
	pessoa, _ := e.Meta.Get("Pessoa")
	pessoa.Field("limite").Fieldtype = "Data" // Currency → text

	migrar(t, e, false, "widening")

	if got := columnType(t, e, "tab_item_pedido", "qtd"); got != "double precision" {
		t.Fatalf("qtd is %q", got)
	}
	if got := columnType(t, e, "tab_pessoa", "limite"); got != "text" {
		t.Fatalf("limite is %q", got)
	}
}

// TestRefusesDroppingDataUnderPrune is what makes a renamedFrom declaration
// safe to delete: get the timing wrong and the migration stops and names the
// column, instead of emptying it.
func TestRefusesDroppingDataUnderPrune(t *testing.T) {
	e := setup(t)
	insertPessoa(t, e, Doc{"nome": "File", "codigo": "keep me"})

	pessoa, _ := e.Meta.Get("Pessoa")
	var kept []*meta.Field
	for _, f := range pessoa.Fields {
		if f.Fieldname != "codigo" {
			kept = append(kept, f)
		}
	}
	pessoa.Fields = kept
	pessoa.ResetFieldIndex()

	_, err := e.Migrate(context.Background(), true)
	if err == nil {
		t.Fatal("prune dropped a column that still held data")
	}
	if !strings.Contains(err.Error(), "codigo") || !strings.Contains(err.Error(), "renamedFrom") {
		t.Fatalf("the refusal should name the column and suggest renamedFrom, got: %v", err)
	}
	if columnType(t, e, "tab_pessoa", "codigo") == "" {
		t.Fatal("the column was dropped anyway")
	}

	// Emptied, the same drop is unremarkable.
	if _, err := e.DB.Pool.Exec(context.Background(), `UPDATE tab_pessoa SET codigo = NULL`); err != nil {
		t.Fatal(err)
	}
	migrar(t, e, true, "drop once empty")
	if columnType(t, e, "tab_pessoa", "codigo") != "" {
		t.Fatal("the empty column should have been dropped")
	}
}

// A whole table works the same way: a DocType that leaves the meta while its
// rows are still there is a rename nobody declared until proven otherwise.
func TestRefusesDroppingATableWithRows(t *testing.T) {
	e := setup(t)
	insertPessoa(t, e, Doc{"nome": "Kai", "tipo": "PF"})

	// Pedido links to Pessoa, so it has to go first or the meta will not load.
	delete(e.Meta.DocTypes, "Pedido")
	delete(e.Meta.DocTypes, "Pessoa")

	_, err := e.Migrate(context.Background(), true)
	if err == nil {
		t.Fatal("prune dropped a table that still held rows")
	}
	if !strings.Contains(err.Error(), "tab_pessoa") || !strings.Contains(err.Error(), "renamedFrom") {
		t.Fatalf("the refusal should name the table and suggest renamedFrom, got: %v", err)
	}
	if columnType(t, e, "tab_pessoa", "nome") == "" {
		t.Fatal("the table was dropped anyway")
	}
}

// TestRenameDocType — the table moves, and so does every row that *names* the
// DocType rather than linking to it.
func TestRenameDocType(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	insertPessoa(t, e, Doc{"nome": "Gil", "tipo": "PF"})

	var pedido string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		doc, err := c.NewDoc("Pedido", Doc{"cliente": "Gil", "itens": []any{
			map[string]any{"descricao": "a", "qtd": 2, "valor": 10},
		}})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		pedido = saved.Str("name")
		// Pedido has trackChanges, and a Version row is written on update, not
		// on insert — so there is something for the sweep to move.
		saved["obs"] = "toca"
		if _, err := c.Save(saved, SaveOpts{IgnorePermissions: true}); err != nil {
			return err
		}
		f, err := c.NewDoc("File", Doc{"file_name": "a.pdf", "file_url": "/files/a.pdf",
			"attached_to_doctype": "Pedido", "attached_to_name": pedido})
		if err != nil {
			return err
		}
		_, err = c.Insert(f, SaveOpts{IgnorePermissions: true})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Rename Pedido → Ordem. amended_from points at its own DocType, so it has
	// to follow or Registry.Validate refuses to load.
	ped, _ := e.Meta.Get("Pedido")
	ped.Name = "Ordem"
	ped.RenamedFrom = meta.Names{"Pedido"}
	ped.Field("amended_from").Options = "Ordem"
	delete(e.Meta.DocTypes, "Pedido")
	e.Meta.DocTypes["Ordem"] = ped

	migrar(t, e, true, "rename doctype")

	if len(sqlRows(t, e, `SELECT 1 FROM tab_ordem WHERE name = $1`, pedido)) != 1 {
		t.Fatal("the row did not move to tab_ordem")
	}
	if columnType(t, e, "tab_pedido", "name") != "" {
		t.Fatal("tab_pedido is still there")
	}
	// The primary key is an index wantedIndexes never names; if the rename left
	// it behind, the next rename would collide with it.
	if indexDef(t, e, "tab_pedido_pkey") != "" {
		t.Fatal("the old primary key name survived")
	}
	for _, q := range []struct{ what, sql string }{
		{"child parenttype", `SELECT 1 FROM tab_item_pedido WHERE parenttype = 'Ordem' AND parent = $1`},
		{"file", `SELECT 1 FROM tab_file WHERE attached_to_doctype = 'Ordem' AND attached_to_name = $1`},
		{"version", `SELECT 1 FROM tab_version WHERE ref_doctype = 'Ordem' AND docname = $1`},
	} {
		if len(sqlRows(t, e, q.sql, pedido)) == 0 {
			t.Fatalf("%s was not repointed at the new DocType name", q.what)
		}
	}
	renames, err := e.AppliedRenames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range renames {
		if r.Kind == "doctype" && r.OldName == "Pedido" && r.NewName == "Ordem" {
			found = true
			if !r.Retirable {
				t.Fatal("the old table is gone, so the declaration is retirable here")
			}
		}
	}
	if !found {
		t.Fatalf("the rename was not recorded: %v", renames)
	}
}

// TestDocumentRenameKeepsAttachments — a defect this work turned up rather than
// introduced: Ctx.Rename swept Version and Comment but not File, so a renamed
// document lost its attachments, and with them the permission check on a
// private file, which reads attached_to_name to decide who may download it.
func TestDocumentRenameKeepsAttachments(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	name := insertPessoa(t, e, Doc{"nome": "Hugo", "tipo": "PF"})

	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		f, err := c.NewDoc("File", Doc{"file_name": "h.pdf", "file_url": "/files/h.pdf",
			"attached_to_doctype": "Pessoa", "attached_to_name": name})
		if err != nil {
			return err
		}
		if _, err := c.Insert(f, SaveOpts{IgnorePermissions: true}); err != nil {
			return err
		}
		_, err = c.Rename("Pessoa", name, "Hugo Renomeado")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sqlRows(t, e, `SELECT 1 FROM tab_file WHERE attached_to_name = 'Hugo Renomeado'`)) != 1 {
		t.Fatal("the attachment did not follow the rename")
	}
}

// TestPatchPhases — the defect the inventory names as "DDL comes before
// patches": before this a patch could only ever see the schema after the change,
// so there was no way to make the data fit what the DDL was about to do.
func TestPatchPhases(t *testing.T) {
	e := setupWith(t, map[string]string{
		"patches/0001_antes.ts": `import { definePatch } from "@ddcore/sdk";
export default definePatch({
  phase: "beforeSchema",
  execute(ctx) {
    const cols = ctx.sql("SELECT column_name FROM information_schema.columns WHERE table_name = 'tab_pessoa' AND column_name = 'apelido'");
    ctx.sql("INSERT INTO tab_pessoa (name, docstatus, nome, codigo) VALUES ($1, 0, $2, $3)",
      ["marca-antes", "antes", String(cols.length)]);
  },
});`,
		"patches/0002_depois.ts": `import { definePatch } from "@ddcore/sdk";
export default definePatch({
  execute(ctx) {
    const cols = ctx.sql("SELECT column_name FROM information_schema.columns WHERE table_name = 'tab_pessoa' AND column_name = 'apelido'");
    ctx.sql("INSERT INTO tab_pessoa (name, docstatus, nome, codigo) VALUES ($1, 0, $2, $3)",
      ["marca-depois", "depois", String(cols.length)]);
  },
});`,
	})

	// The app installs fresh, so both patches were recorded rather than run:
	// a patch describes a change to data a new database does not have, and a
	// beforeSchema one would run against tables that do not exist yet.
	if len(sqlRows(t, e, `SELECT 1 FROM tab_pessoa WHERE name LIKE 'marca-%'`)) != 0 {
		t.Fatal("a fresh install ran its patches instead of recording them")
	}
	ran := sqlRows(t, e, `SELECT name FROM ddcore_patch WHERE app = 'demo'`)
	if len(ran) != 2 {
		t.Fatalf("a fresh install should record both patches, got %v", ran)
	}

	// Make them pending again, then add a column so the two phases see a
	// different schema.
	if _, err := e.DB.Pool.Exec(context.Background(), `DELETE FROM ddcore_patch WHERE app = 'demo'`); err != nil {
		t.Fatal(err)
	}
	pessoa, _ := e.Meta.Get("Pessoa")
	pessoa.Fields = append(pessoa.Fields, &meta.Field{Fieldname: "apelido", Fieldtype: "Data", Label: "Apelido"})
	pessoa.ResetFieldIndex()

	res := migrar(t, e, false, "phases")
	if len(res.Patches) != 2 {
		t.Fatalf("both patches should have run, got %v", res.Patches)
	}

	rows := sqlRows(t, e, `SELECT name, codigo FROM tab_pessoa WHERE name LIKE 'marca-%' ORDER BY name`)
	if len(rows) != 2 {
		t.Fatalf("expected both marks, got %v", rows)
	}
	if got := db.Str(rows[0]["codigo"]); got != "0" {
		t.Fatalf("the beforeSchema patch saw the new column already: %q", got)
	}
	if got := db.Str(rows[1]["codigo"]); got != "1" {
		t.Fatalf("the afterSchema patch did not see the new column: %q", got)
	}

	// A patch runs once. The second migration is the idempotence check migrar
	// already made; this asserts nothing ran again.
	if res := migrar(t, e, false, "phases again"); len(res.Patches) != 0 {
		t.Fatalf("patches re-ran: %v", res.Patches)
	}
}

// TestBareExecutePatchStillWorks — docs/agent/conventions.md documented a plain
// `export function execute`, and that contract does not break.
func TestBareExecutePatchStillWorks(t *testing.T) {
	e := setupWith(t, map[string]string{
		"patches/0001_nu.ts": `export function execute(ctx: any) {
  ctx.sql("INSERT INTO tab_pessoa (name, docstatus, nome) VALUES ('nu', 0, 'nu')");
}`,
	})
	if _, err := e.DB.Pool.Exec(context.Background(), `DELETE FROM ddcore_patch`); err != nil {
		t.Fatal(err)
	}
	res := migrar(t, e, false, "bare execute")
	if len(res.Patches) != 1 {
		t.Fatalf("the bare form was not discovered: %v", res.Patches)
	}
	for _, p := range e.Current().Snap.Patches {
		if p.BeforeSchema() {
			t.Fatal("a patch with no phase must default to afterSchema")
		}
	}
	if len(sqlRows(t, e, `SELECT 1 FROM tab_pessoa WHERE name = 'nu'`)) != 1 {
		t.Fatal("the bare patch did not run")
	}
}

// TestPatchSQLIsOnlyForPatches — ctx.sql is the one write-SQL in the framework,
// and it must stay unreachable from a controller, a service or a report.
func TestPatchSQLIsOnlyForPatches(t *testing.T) {
	e := setup(t)
	err := e.Run(context.Background(), "Administrator", func(c *Ctx) error {
		_, err := c.PatchSQL(`UPDATE tab_pessoa SET nome = 'x'`, nil)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "patch") {
		t.Fatalf("write SQL outside a patch should be refused, got %v", err)
	}
	// And ddcore.db.sql stays read-only.
	err = e.Run(context.Background(), "Administrator", func(c *Ctx) error {
		_, err := c.SQL(`UPDATE tab_pessoa SET nome = 'x'`, nil)
		return err
	})
	if err == nil {
		t.Fatal("ddcore.db.sql accepted a write")
	}
}

// TestExpandBackfillValidateContract is DAT-04 end to end, in two releases: the
// route that preserves data when a conversion cannot be a cast.
//
// It also proves the property the single transaction buys — a validation that
// fails rolls the contraction back with everything else, so a half-migrated
// database is not a state this can reach.
func TestExpandBackfillValidateContract(t *testing.T) {
	// Release 1: valor_texto stays, valor_num arrives beside it, a patch fills it.
	e := setupWith(t, map[string]string{
		"patches/0001_backfill.ts": `import { definePatch } from "@ddcore/sdk";
export default definePatch({
  description: "valor_texto (Data) → valor_num (Currency)",
  execute(ctx) {
    ctx.sql("UPDATE tab_pessoa SET valor_num = NULLIF(regexp_replace(valor_texto, '[^0-9.-]', '', 'g'), '')::numeric" +
            " WHERE valor_texto IS NOT NULL AND valor_num IS NULL");
  },
});`,
		"patches/0002_valida.ts": `import { definePatch } from "@ddcore/sdk";
export default definePatch({
  phase: "beforeSchema",
  description: "validate the backfill, then retire valor_texto",
  execute(ctx) {
    const left = ctx.sql("SELECT count(*)::int AS n FROM tab_pessoa WHERE valor_texto IS NOT NULL AND valor_num IS NULL");
    if (left[0].n > 0) ddcore.throw(left[0].n + " row(s) still have no valor_num");
    ctx.sql("ALTER TABLE tab_pessoa DROP COLUMN valor_texto");
  },
});`,
	})
	ctx := context.Background()
	pessoa, _ := e.Meta.Get("Pessoa")
	pessoa.Fields = append(pessoa.Fields, &meta.Field{Fieldname: "valor_texto", Fieldtype: "Data", Label: "Valor"})
	pessoa.ResetFieldIndex()
	migrar(t, e, false, "release 0: the old field")

	insertPessoa(t, e, Doc{"nome": "Ivo", "valor_texto": "R$ 1.250,00"})
	insertPessoa(t, e, Doc{"nome": "Joana", "valor_texto": "990"})

	// Release 1 — expand and backfill. The patches were recorded on install,
	// so make them pending; a real deployment adds the files in this release.
	if _, err := e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_patch`); err != nil {
		t.Fatal(err)
	}
	pessoa.Fields = append(pessoa.Fields, &meta.Field{Fieldname: "valor_num", Fieldtype: "Currency", Label: "Valor"})
	pessoa.ResetFieldIndex()

	// Only the backfill, not the validation: run one phase at a time by
	// pretending the validating patch has already gone.
	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_patch (app, name) VALUES ('demo', '0002_valida')`); err != nil {
		t.Fatal(err)
	}
	migrar(t, e, false, "release 1: expand and backfill")

	rows := sqlRows(t, e, `SELECT nome, valor_num FROM tab_pessoa ORDER BY nome`)
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %v", rows)
	}
	if got := db.Str(rows[1]["valor_num"]); !strings.HasPrefix(got, "990") {
		t.Fatalf("Joana was not backfilled: %v", rows[1])
	}

	// Leave one row unconverted, then try release 2 — validate and contract.
	//
	// The patch drops the column itself rather than leaving it to --prune, and
	// that is the contract: discarding data a backfill has already copied is a
	// deliberate act with the author's name on it in ddcore_patch, not a side
	// effect of a CLI flag. --prune stays for the leftovers that are empty.
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE tab_pessoa SET valor_num = NULL WHERE nome = 'Ivo'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_patch WHERE name = '0002_valida'`); err != nil {
		t.Fatal(err)
	}
	var kept []*meta.Field
	for _, f := range pessoa.Fields {
		if f.Fieldname != "valor_texto" {
			kept = append(kept, f)
		}
	}
	pessoa.Fields = kept
	pessoa.ResetFieldIndex()

	if _, err := e.Migrate(ctx, false); err == nil {
		t.Fatal("the contraction ran with a row still unconverted")
	}
	if columnType(t, e, "tab_pessoa", "valor_texto") == "" {
		t.Fatal("the failed validation should have rolled the drop back")
	}
	if len(sqlRows(t, e, `SELECT 1 FROM ddcore_patch WHERE name = '0002_valida'`)) != 0 {
		t.Fatal("a patch that threw was recorded as applied")
	}

	// Fix the row and the same release goes through.
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE tab_pessoa SET valor_num = 1250 WHERE nome = 'Ivo'`); err != nil {
		t.Fatal(err)
	}
	migrar(t, e, false, "release 2: validate and contract")
	if columnType(t, e, "tab_pessoa", "valor_texto") != "" {
		t.Fatal("the old column should be gone once every row converted")
	}
	rows = sqlRows(t, e, `SELECT nome, valor_num FROM tab_pessoa ORDER BY nome`)
	if len(rows) != 2 || !strings.HasPrefix(db.Str(rows[0]["valor_num"]), "1250") {
		t.Fatalf("the converted values did not survive the contraction: %v", rows)
	}
}

// DAT-05 — a compound business key is a partial unique index the planner owns
// end to end: it creates it, it leaves it alone when nothing changed, and it
// takes it away when the declaration goes.

func uniqueKeyOn(t *testing.T, e *Engine, doctype string, keys ...meta.UniqueKey) {
	t.Helper()
	d, ok := e.Meta.Get(doctype)
	if !ok {
		t.Fatalf("meta has no %s", doctype)
	}
	d.UniqueKeys = keys
}

func TestAUniqueKeyBecomesAPartialUniqueIndex(t *testing.T) {
	e := setup(t)
	uniqueKeyOn(t, e, "Pessoa", meta.UniqueKey{Name: "tipo_codigo", Fields: []string{"tipo", "codigo"}})
	migrar(t, e, false, "create key")

	def := indexDef(t, e, "tab_pessoa_uk_tipo_codigo")
	for _, want := range []string{"UNIQUE", "tipo", "codigo", "WHERE"} {
		if !strings.Contains(def, want) {
			t.Fatalf("index is missing %q: %q", want, def)
		}
	}
	// Every component has to be in the predicate, or a row with one empty
	// column would be constrained where the pre-check says it is not.
	if !strings.Contains(def, "codigo IS NOT NULL") || !strings.Contains(def, "tipo IS NOT NULL") {
		t.Fatalf("the predicate does not name both components: %q", def)
	}
}

// Removing the declaration has to remove the constraint. The index diff only
// ever visits the names the meta wants, and prune only handles columns and
// tables — so without the sweep this key would go on being enforced by an
// index no one could read a declaration for.
func TestDeletingAUniqueKeyDropsItsIndex(t *testing.T) {
	e := setup(t)
	uniqueKeyOn(t, e, "Pessoa", meta.UniqueKey{Name: "tipo_codigo", Fields: []string{"tipo", "codigo"}})
	migrar(t, e, false, "create key")
	if indexDef(t, e, "tab_pessoa_uk_tipo_codigo") == "" {
		t.Fatal("the index was never created")
	}

	uniqueKeyOn(t, e, "Pessoa")
	// Deliberately without prune: dropping an index loses no row, so it must
	// not need the flag that guards losing data.
	res := migrar(t, e, false, "drop key")

	if def := indexDef(t, e, "tab_pessoa_uk_tipo_codigo"); def != "" {
		t.Fatalf("the index outlived its declaration: %q", def)
	}
	var dropped bool
	for _, st := range res.DDL {
		if st.Kind == db.KindDropIndex {
			dropped = true
			if st.Destructive {
				t.Fatal("dropping an index loses no data and must not be marked destructive")
			}
		}
	}
	if !dropped {
		t.Fatalf("no drop was planned: %v", db.SQL(res.DDL))
	}
}

// The counterpart of TestRenameFieldRenamesItsIndex, for an index no single
// field owns. Postgres carries a unique index across a RENAME COLUMN on its
// own; rebuilding it would be the expensive way to change nothing, and on a
// unique index it is a window with no constraint.
func TestRenamingAFieldInsideAUniqueKeyDoesNotRebuildTheIndex(t *testing.T) {
	e := setup(t)
	uniqueKeyOn(t, e, "Pessoa", meta.UniqueKey{Name: "tipo_codigo", Fields: []string{"tipo", "codigo"}})
	migrar(t, e, false, "create key")

	pessoa, _ := e.Meta.Get("Pessoa")
	codigo := pessoa.Field("codigo")
	codigo.Fieldname = "codigo_interno"
	codigo.RenamedFrom = meta.Names{"codigo"}
	pessoa.ResetFieldIndex()
	// The half the declaration cannot do for you, exactly as for searchFields.
	pessoa.UniqueKeys = []meta.UniqueKey{{Name: "tipo_codigo", Fields: []string{"tipo", "codigo_interno"}}}

	res := migrar(t, e, false, "rename inside a key")

	for _, st := range res.DDL {
		if st.Kind == db.KindDropIndex {
			t.Fatalf("the key's index was rebuilt instead of followed: %q", st.SQL)
		}
	}
	if def := indexDef(t, e, "tab_pessoa_uk_tipo_codigo"); !strings.Contains(def, "codigo_interno") {
		t.Fatalf("the index did not follow the column: %q", def)
	}
}

// The sweep is scoped by the owning table, not by a name prefix. A DocType
// called "Pedido Uk Foo" gives its table indexes named "tab_pedido_uk_foo_*",
// which carry the prefix "tab_pedido_uk_" that Pedido's own sweep looks for —
// and which belong to another DocType entirely.
func TestTheSweepDoesNotReachAnotherDocTypesTable(t *testing.T) {
	e := setupWith(t, map[string]string{
		"doctypes/pedido_uk_foo/pedido_uk_foo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pedido Uk Foo", fields: [
  { fieldname: "rotulo", fieldtype: "Data", label: "Rotulo" } ] });`,
	})
	neighbour := indexDef(t, e, "tab_pedido_uk_foo_modified")
	if neighbour == "" {
		t.Fatal("the neighbouring table has no index to protect")
	}
	// Pedido declares no key at all, so its sweep sees every candidate name.
	migrar(t, e, false, "sweep with a neighbour in the namespace")
	if got := indexDef(t, e, "tab_pedido_uk_foo_modified"); got != neighbour {
		t.Fatalf("the sweep reached another DocType's index: %q", got)
	}
}
