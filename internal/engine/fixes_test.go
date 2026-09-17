package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// B06 — ddcore.db.sql must be truly read-only: a write CTE
// disguised as a SELECT must be rejected by the database and nothing
// can be saved.
func TestB06_SQLReadonlyRejectsWrites(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.SQL(`WITH changed AS (
			UPDATE tab_role SET modified_by = 'audit' WHERE name = $1 RETURNING name
		) SELECT * FROM changed`, []any{"Gestor"})
		if err == nil {
			t.Fatalf("expected write CTE rejection")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "read-only") {
			t.Fatalf("expected read-only transaction error, got: %v", err)
		}
		// the transaction remains usable and SET LOCAL was rolled back
		rows, err := c.SQL(`SELECT modified_by FROM tab_role WHERE name = $1`, []any{"Gestor"})
		if err != nil {
			t.Fatalf("SELECT after rejection failed: %v", err)
		}
		if len(rows) != 1 || db.Str(rows[0]["modified_by"]) == "audit" {
			t.Fatalf("the write leaked: %v", rows)
		}
		// legitimate write through the normal path continues to work
		_, err = c.DBSet("Role", "Gestor", Doc{"role_name": "Gestor"}, true)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		v, _ := c.GetValue("Role", "Gestor", "modified_by")
		if db.Str(v) == "audit" {
			t.Fatalf("modified_by was saved by the CTE")
		}
		return nil
	})
}

// B07 — two concurrent transactions writing to the same document: T1 modifies
// full_name and holds the transaction open; T2 read the previous version and saves only
// language. T1's edit must not be lost.
func TestB07_LostUpdate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "co@x.com", "full_name": "Original"})
		_, err := c.Insert(u, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// T2 reads the stale version before T1 writes
	var stale Doc
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		var err error
		stale, err = c.GetDoc("User", "co@x.com")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	t1Saved, t1Commit, t1Done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		t1Done <- e.Run(ctx, "Administrator", func(c *Ctx) error {
			d, err := c.GetDoc("User", "co@x.com")
			if err != nil {
				return err
			}
			d["full_name"] = "Primeiro"
			if _, err := c.Save(d, SaveOpts{}); err != nil {
				return err
			}
			close(t1Saved)
			<-t1Commit // hold the transaction open
			return nil
		})
	}()
	<-t1Saved

	t2Done := make(chan error, 1)
	go func() {
		t2Done <- e.Run(ctx, "Administrator", func(c *Ctx) error {
			d := stale.Clone()
			d["language"] = "en"
			_, err := c.Save(d, SaveOpts{})
			return err
		})
	}()
	select {
	case err := <-t2Done:
		t.Fatalf("T2 did not wait for T1's lock: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	close(t1Commit)
	if err := <-t1Done; err != nil {
		t.Fatalf("T1: %v", err)
	}
	err := <-t2Done
	if err == nil {
		t.Fatalf("T2 overwrote the stale version without error")
	}
	if got := cerr.From(err).Type; got != "TimestampMismatchError" {
		t.Fatalf("expected TimestampMismatchError, got %s (%v)", got, err)
	}
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		d, err := c.GetDoc("User", "co@x.com")
		if err != nil {
			return err
		}
		if d.Str("full_name") != "Primeiro" {
			t.Fatalf("T1's edit was lost: %v", d.Str("full_name"))
		}
		return nil
	})
}

// B07 — invalid timestamp cannot pass as "equal".
func TestB07_InvalidTimestampRejected(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "ts@x.com", "full_name": "TS"})
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		d, err := c.GetDoc("User", "ts@x.com")
		if err != nil {
			return err
		}
		d["modified"] = "not a valid date"
		d["full_name"] = "Other"
		if _, err := c.Save(d, SaveOpts{}); err == nil || cerr.From(err).Type != "TimestampMismatchError" {
			t.Fatalf("expected TimestampMismatchError, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B03 — saving a parent with a child row belonging to another document cannot
// transfer the row: it becomes a copy.
func TestB03_ChildDoesNotSwitchParent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		a, _ := c.NewDoc("User", Doc{"email": "a@x.com", "full_name": "A"})
		a["roles"] = []any{map[string]any{"role": "Gestor"}}
		a, err := c.Insert(a, SaveOpts{})
		if err != nil {
			return err
		}
		b, _ := c.NewDoc("User", Doc{"email": "b@x.com", "full_name": "B"})
		b, err = c.Insert(b, SaveOpts{})
		if err != nil {
			return err
		}
		childOfA := a.Children("roles")[0]
		origName := childOfA.Str("name")
		if origName == "" {
			t.Fatalf("child row without name: %v", childOfA)
		}
		// copy A's child row into B keeping the name
		b["roles"] = []any{map[string]any(childOfA.Clone())}
		b, err = c.Save(b, SaveOpts{})
		if err != nil {
			return err
		}
		a, err = c.GetDoc("User", "a@x.com")
		if err != nil {
			return err
		}
		if len(a.Children("roles")) != 1 {
			t.Fatalf("A lost its child row: %v", a["roles"])
		}
		if a.Children("roles")[0].Str("name") != origName {
			t.Fatalf("A's child row changed its name")
		}
		if len(b.Children("roles")) != 1 {
			t.Fatalf("B should have a copy: %v", b["roles"])
		}
		if b.Children("roles")[0].Str("name") == origName {
			t.Fatalf("B kept the exact same row as A (%s)", origName)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B21 — after a dbSet, the in-memory document must load the new
// `modified`; otherwise a save() on the same instance fails with TimestampMismatch
// without anyone having edited the document. And runMethod returns the
// persisted version.
func TestB21_DbSetUpdatesModified(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Cli", "tipo": "PF"})
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			return err
		}
		ped, _ := c.NewDoc("Pedido", Doc{"cliente": "Cli"})
		ped, err := c.Insert(ped, SaveOpts{})
		if err != nil {
			return err
		}
		rt, err := c.RT()
		if err != nil {
			return err
		}
		res, err := rt.RunMethod("Pedido", "tocar", ped.JSON(), []byte(`{}`))
		if err != nil {
			t.Fatalf("dbSet followed by save failed: %v", err)
		}
		var out Doc
		if err := json.Unmarshal(res.Doc, &out); err != nil {
			return err
		}
		current, err := c.GetDoc("Pedido", ped.Name())
		if err != nil {
			return err
		}
		if out.Str("obs") != "tocado" || current.Str("obs") != "tocado" {
			t.Fatalf("obs was not saved: %v / %v", out["obs"], current["obs"])
		}
		if !sameTime(out["modified"], current["modified"], time.UTC) {
			t.Fatalf("runMethod returned stale modified: %v != %v", out["modified"], current["modified"])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B14 — unique indexes: the predicate must match the column type
// (`<> ''` only on text) and changing searchIndex ↔ unique must recreate the index,
// since the name is the same.
func TestB14_UniqueIndexes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	indexdef := func(name string) string {
		var out string
		e.Run(ctx, "Administrator", func(c *Ctx) error {
			rows, err := c.SQL(`SELECT indexdef FROM pg_indexes WHERE indexname = $1`, []any{name})
			if err != nil {
				return err
			}
			if len(rows) > 0 {
				out = db.Str(rows[0]["indexdef"])
			}
			return nil
		})
		return out
	}
	migrateStep := func(step string) {
		if _, err := e.Migrate(ctx, false); err != nil {
			t.Fatalf("%s: migrate failed: %v", step, err)
		}
		plan, err := e.Plan(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan) != 0 {
			t.Fatalf("%s: migrate was not idempotent: %v", step, plan)
		}
	}

	// Currency unique: the predicate cannot compare numeric with ''
	pessoa, _ := e.Meta.Get("Pessoa")
	pessoa.Field("limite").Unique = true
	migrateStep("currency unique")
	if def := indexdef("tab_pessoa_limite"); !strings.Contains(def, "UNIQUE") || strings.Contains(def, "''") {
		t.Fatalf("unexpected Currency index: %q", def)
	}

	// Link with search index becomes unique and then reverts
	pedido, _ := e.Meta.Get("Pedido")
	before := indexdef("tab_pedido_cliente")
	if before == "" || strings.Contains(before, "UNIQUE") {
		t.Fatalf("expected non-unique index on cliente: %q", before)
	}
	pedido.Field("cliente").Unique = true
	migrateStep("link → unique")
	if def := indexdef("tab_pedido_cliente"); !strings.Contains(def, "UNIQUE") {
		t.Fatalf("Link index did not become unique: %q", def)
	}
	pedido.Field("cliente").Unique = false
	migrateStep("unique → link")
	if def := indexdef("tab_pedido_cliente"); def == "" || strings.Contains(def, "UNIQUE") {
		t.Fatalf("unique index did not revert to search index: %q", def)
	}
}

// ddcore.db.lock serializes two transactions requesting the same key.
func TestDBLockSerializes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var mu sync.Mutex
	var order []string
	firstReleased := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.Run(ctx, "Administrator", func(c *Ctx) error {
			if err := c.Lock("contrato:1"); err != nil {
				t.Error(err)
				return err
			}
			close(firstReleased)
			time.Sleep(400 * time.Millisecond)
			mu.Lock()
			order = append(order, "t1")
			mu.Unlock()
			return nil
		})
	}()
	<-firstReleased
	time.Sleep(50 * time.Millisecond)
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		if err := c.Lock("contrato:1"); err != nil {
			return err
		}
		mu.Lock()
		order = append(order, "t2")
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "t1" || order[1] != "t2" {
		t.Fatalf("lock did not serialize: %v", order)
	}
}

// B19 — a filter on a child field must not duplicate the parent in list queries or
// counts, and Count must observe the same orFilters as the listing.
func TestB19_FilterOnChildDoesNotDuplicate(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		if _, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Cli", "cpf": "900"}), SaveOpts{}); err != nil {
			return err
		}
		ped := mustDoc(t, c, "Pedido", Doc{"cliente": "Cli"})
		// two child rows match the same filter
		ped["itens"] = []any{
			map[string]any{"descricao": "Cadeira", "qtd": 1, "valor": 10},
			map[string]any{"descricao": "Cadeira", "qtd": 2, "valor": 20},
		}
		_, err := c.Insert(ped, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		f := []any{[]any{"Item Pedido", "descricao", "=", "Cadeira"}}
		rows, err := c.GetList("Pedido", ListArgs{Filters: f, Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 order, got %d: %v", len(rows), rows)
		}
		n, err := c.Count("Pedido", f)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("count duplicated the parent: %d", n)
		}
		// two conditions on the same child require the same row
		n, err = c.Count("Pedido", []any{
			[]any{"Item Pedido", "descricao", "=", "Cadeira"},
			[]any{"Item Pedido", "qtd", "=", 5},
		})
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("child conditions should apply to the same row: %d", n)
		}
		// orFilters reach count
		n, err = c.Count("Pedido", nil, []any{[]any{"cliente", "=", "ninguém"}})
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("Count ignored orFilters: %d", n)
		}
		n, err = c.Count("Pedido", nil, []any{[]any{"cliente", "=", "Cli"}})
		if err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("Count with orFilters: %d", n)
		}
		// child that is not a child table of the parent is rejected
		if _, err := c.Count("Pessoa", []any{[]any{"Item Pedido", "descricao", "=", "x"}}); err == nil {
			t.Fatalf("expected rejection of unbound child")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustDoc(t *testing.T, c *Ctx, dt string, values Doc) Doc {
	t.Helper()
	d, err := c.NewDoc(dt, values)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// B08 — concurrent requests during e.Load(): each ctx must see meta,
// pool, and translations of the same generation, without data races and without
// returning an old runtime to the new pool.
func TestB08_ReloadKeepsPoolConsistent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Reload", "cpf": "808"}), SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	errs := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				err := e.Run(ctx, "Administrator", func(c *Ctx) error {
					if c.St != c.E.Current() && c.St == nil {
						t.Error("ctx without state")
					}
					rows, err := c.GetList("Pessoa", ListArgs{Filters: map[string]any{"nome": "Reload"}})
					if err != nil {
						return err
					}
					if len(rows) != 1 {
						t.Errorf("inconsistent listing during reload: %v", rows)
					}
					rt, err := c.RT()
					if err != nil {
						return err
					}
					if _, err := rt.Meta(); err != nil {
						return err
					}
					_ = c.T("Nome")
					return nil
				})
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		if err := e.Load(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	select {
	case err := <-errs:
		t.Fatalf("request failed during reload: %v", err)
	default:
	}

	// after reload the new ctx uses the new pool and the old one was decommissioned
	st := e.Current()
	c := e.NewCtx(ctx, "Administrator")
	if c.St != st {
		t.Fatalf("NewCtx did not capture the current state")
	}
	if e.Meta != st.Meta || e.Pool != st.Pool {
		t.Fatalf("legacy fields diverged from current state")
	}
}

// B15 — a job that does not return must be interrupted by timeout, and a
// "running" job whose worker died returns to the queue when its lease expires.
func TestB15_JobTimeout(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	start := time.Now()
	_, err := e.RunJob(withTimeout(ctx, 2*time.Second), "Administrator", "demo.services.loop.travar", nil)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("job only stopped after %s", d)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("unexpected error: %v", err)
	}

	// the VM remains usable after interruption
	res, err := e.RunJob(ctx, "Administrator", "demo.services.loop.ok", map[string]any{"x": 7})
	if err != nil || !strings.Contains(string(res), `"x":7`) {
		t.Fatalf("runtime did not survive interrupt: %s %v", res, err)
	}
}

func withTimeout(ctx context.Context, d time.Duration) context.Context {
	c, cancel := context.WithTimeout(ctx, d)
	_ = cancel
	return c
}

// B15 — dead worker: job remains running with expired lease and returns to the queue.
func TestB15_ExpiredLeaseReturnsToQueue(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var id int64
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		var err error
		id, err = c.Enqueue("demo.services.loop.ok", nil, map[string]any{"timeout": 5})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// simulate claim by a worker that died right after
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'running', attempts = 1, lease_until = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := e.requeueStale(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := e.DB.Pool.QueryRow(ctx, `SELECT status FROM ddcore_job WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("abandoned job did not return to queue: %s", status)
	}
	// when attempts are exhausted, it becomes failed instead of running forever
	if _, err := e.DB.Pool.Exec(ctx, `UPDATE ddcore_job SET status = 'running', attempts = max_attempts, lease_until = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := e.requeueStale(ctx); err != nil {
		t.Fatal(err)
	}
	e.DB.Pool.QueryRow(ctx, `SELECT status FROM ddcore_job WHERE id = $1`, id).Scan(&status)
	if status != "failed" {
		t.Fatalf("expected failed after exhausting attempts: %s", status)
	}
	// timeout_seconds saved during enqueue
	var to int
	e.DB.Pool.QueryRow(ctx, `SELECT timeout_seconds FROM ddcore_job WHERE id = $1`, id).Scan(&to)
	if to != 5 {
		t.Fatalf("job timeout was not saved: %d", to)
	}
}

// Gap: checkAllowOnSubmit ran before validate/beforeSave, so a
// hook could still modify a protected field after the check.
func TestAllowOnSubmitAfterHooks(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		if _, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Sub", "cpf": "777"}), SaveOpts{}); err != nil {
			return err
		}
		ped := mustDoc(t, c, "Pedido", Doc{"cliente": "Sub", "desconto": 1})
		ped["itens"] = []any{map[string]any{"descricao": "a", "qtd": 1, "valor": 10}}
		ped, err := c.Insert(ped, SaveOpts{})
		if err != nil {
			return err
		}
		if ped, err = c.Submit(ped); err != nil {
			return err
		}
		// obs is allowOnSubmit, but the hook touches desconto, which is not
		ped["obs"] = "bagunca"
		if _, err := c.Save(ped, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "cannot be changed after submission") {
			t.Fatalf("hook modified protected field after submit: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Gap: readOnlyDependsOn must be enforced on the server — hiding the field
// in the UI is not authorization.
func TestReadOnlyDependsOnServerSide(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		p, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "RO", "cpf": "555", "tipo": "PF", "codigo": "A"}), SaveOpts{})
		if err != nil {
			return err
		}
		// PF: the field is editable
		p["codigo"] = "B"
		if p, err = c.Save(p, SaveOpts{}); err != nil {
			return err
		}
		p["tipo"] = "PJ"
		if p, err = c.Save(p, SaveOpts{}); err != nil {
			return err
		}
		// PJ: expression evaluates to true, the field cannot change
		p["codigo"] = "C"
		if _, err := c.Save(p, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "is read-only") {
			t.Fatalf("readOnlyDependsOn was not enforced: %v", err)
		}
		// saving without modifying the field continues to work
		p, _ = c.GetDoc("Pessoa", "RO")
		p["limite"] = 10
		_, err = c.Save(p, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// B04 — Version diff cannot record passwords or password hashes.
func TestB04_VersionDoesNotStoreSecret(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		p, err := c.Insert(mustDoc(t, c, "Pessoa", Doc{"nome": "Seg", "cpf": "444", "segredo": "abc"}), SaveOpts{})
		if err != nil {
			return err
		}
		p["segredo"], p["limite"] = "xyz", 5
		if _, err := c.Save(p, SaveOpts{}); err != nil {
			return err
		}
		rows, err := c.GetList("Version", ListArgs{Filters: map[string]any{"ref_doctype": "Pessoa", "docname": "Seg"}, Fields: []string{"data"}, IgnorePermissions: true})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 version, got %d", len(rows))
		}
		data := db.Str(rows[0]["data"])
		if strings.Contains(data, "segredo") || strings.Contains(data, "xyz") {
			t.Fatalf("the version stored the secret: %s", data)
		}
		if !strings.Contains(data, "limite") {
			t.Fatalf("the version should record limite: %s", data)
		}

		// User: password_hash is Data, but remains excluded from history
		u, err := c.Insert(mustDoc(t, c, "User", Doc{"email": "seg@x.com", "full_name": "Seg", "new_password": "segredo1"}), SaveOpts{})
		if err != nil {
			return err
		}
		u["new_password"] = "segredo2"
		if _, err := c.Save(u, SaveOpts{}); err != nil {
			return err
		}
		vs, err := c.GetList("Version", ListArgs{Filters: map[string]any{"ref_doctype": "User", "docname": "seg@x.com"}, Fields: []string{"data"}, IgnorePermissions: true})
		if err != nil {
			return err
		}
		for _, v := range vs {
			if strings.Contains(db.Str(v["data"]), "password") {
				t.Fatalf("User version leaked password: %s", db.Str(v["data"]))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Gap: `requires` must be validated and sorted during Load.
func TestRequiresSortsAndValidates(t *testing.T) {
	metas := map[string]*AppMeta{
		"core": {Name: "core"},
		"a":    {Name: "a", Requires: []string{"b"}},
		"b":    {Name: "b", Requires: []string{"core"}},
	}
	in := []js.App{{Name: "core"}, {Name: "a"}, {Name: "b"}}
	out, err := orderApps(in, metas)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range out {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "core,b,a" {
		t.Fatalf("unexpected order: %v", names)
	}
	// missing dependency
	if _, err := orderApps([]js.App{{Name: "core"}, {Name: "a"}}, metas); err == nil || !strings.Contains(err.Error(), "is not installed") {
		t.Fatalf("expected missing dependency error, got %v", err)
	}
	// cycle
	cycle := map[string]*AppMeta{"x": {Name: "x", Requires: []string{"y"}}, "y": {Name: "y", Requires: []string{"x"}}}
	if _, err := orderApps([]js.App{{Name: "x"}, {Name: "y"}}, cycle); err == nil || !strings.Contains(err.Error(), "circular") {
		t.Fatalf("expected circular dependency error, got %v", err)
	}
}

// An app loaded under a namespace other than the one it declares is a rename
// nobody asked for: the mismatch has to stop the load, not be papered over.
func TestAppNameMismatchRefusesTheLoad(t *testing.T) {
	snap := &Snapshot{Apps: map[string]*AppMeta{
		"core": {Name: "core"},
		"app":  {Name: "alugueis", Dir: "/app"},
	}}
	apps := []js.App{{Name: "core"}, {Name: "app", Dir: "/app"}}
	err := checkAppNames(apps, snap)
	if err == nil {
		t.Fatal("expected an error for an app loaded under the wrong namespace")
	}
	for _, want := range []string{"/app", `"alugueis"`, `"app"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %s", err, want)
		}
	}
	// An app whose manifest agrees, and one with no manifest at all, both load.
	ok := &Snapshot{Apps: map[string]*AppMeta{"core": {Name: "core"}, "demo": {Name: "demo"}}}
	if err := checkAppNames([]js.App{{Name: "core"}, {Name: "demo"}, {Name: "nometa"}}, ok); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveLinkTitles(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		titles, err := c.LinkTitles("User", []string{"Administrator", "inexistente"})
		if err != nil {
			t.Fatalf("LinkTitles error: %v", err)
		}
		if titles["Administrator"] != "Administrator" {
			t.Fatalf("expected 'Administrator', got %q", titles["Administrator"])
		}
		if titles["inexistente"] != "inexistente" {
			t.Fatalf("expected fallback to name, got %q", titles["inexistente"])
		}
		return nil
	})
}

func TestLinkFieldSearchByTitleAndSearchFields(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		p, err := c.NewDoc("Pessoa", Doc{"nome": "Maria Comércio", "cpf": "999.888.777-66"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			t.Fatal(err)
		}

		ped, err := c.NewDoc("Pedido", Doc{"cliente": "Maria Comércio"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Insert(ped, SaveOpts{}); err != nil {
			t.Fatal(err)
		}

		// 1. Search by name directly on the cliente Link field
		rows, err := c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%Maria%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 order searching for 'Maria', got %d", len(rows))
		}

		// Search without accents should match accented Link title.
		rows, err = c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%comercio%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 order searching for 'comercio', got %d", len(rows))
		}

		// 2. Search by CPF (searchFields of Pessoa, not the value stored in Pedido's cliente column)
		rows, err = c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%888.777%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 order searching for CPF '888.777', got %d", len(rows))
		}

		// 3. Count with OrFilters
		count, err := c.Count("Pedido", nil, []any{[]any{"cliente", "like", "%888.777%"}})
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected count 1 searching for CPF, got %d", count)
		}

		// 4. Search for non-existent value
		rows, err = c.GetList("Pedido", ListArgs{
			OrFilters: []any{[]any{"cliente", "like", "%Inexistente%"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 orders, got %d", len(rows))
		}

		return nil
	})
}

// PRD-07: an app declares the ddcore releases it supports, and a binary outside
// that range refuses to load it — every offending app named at once.
func TestCoreCompatRanges(t *testing.T) {
	cases := []struct {
		rng, core string
		ok        bool
	}{
		{">=0.14.0 <1.0.0", "v0.14.0", true},
		{">=0.14.0 <1.0.0", "0.13.9", false},
		{">=0.14.0 <1.0.0", "v1.0.0", false},
		{">=0.14 <1", "v0.20.3-4-gabc123-dirty", true},
		{"^0.14.0", "v0.14.9", true},
		{"^0.14.0", "v0.15.0", false},
		{"^0.0.3", "v0.0.3", true},
		{"^0.0.3", "v0.0.4", false},
		{"^1.2.0", "v1.9.0", true},
		{"^1.2.0", "v2.0.0", false},
		{"~1.2.3", "v1.2.9", true},
		{"~1.2.3", "v1.3.0", false},
		{"0.14.0", "v0.14.0", true},
		{"=0.14.0", "v0.14.1", false},
		{">0.14.0 <=0.15.0", "v0.15.0", true},
	}
	for _, c := range cases {
		snap := &Snapshot{Apps: map[string]*AppMeta{"a": {Name: "a", Ddcore: c.rng}}}
		_, err := checkCoreCompat(snap, c.core)
		if (err == nil) != c.ok {
			t.Errorf("range %q on %s: err = %v, want ok=%v", c.rng, c.core, err, c.ok)
		}
	}

	snap := &Snapshot{Apps: map[string]*AppMeta{
		"core": {Name: "core"},
		"shop": {Name: "shop", Ddcore: ">=0.15.0"},
		"crm":  {Name: "crm", Ddcore: "<0.14.0"},
		"fine": {Name: "fine", Ddcore: "^0.14.0"},
	}}
	_, err := checkCoreCompat(snap, "v0.14.2")
	if err == nil {
		t.Fatal("expected incompatible apps to refuse the load")
	}
	for _, want := range []string{"app shop requires ddcore >=0.15.0, but this binary is 0.14.2", "app crm requires ddcore <0.14.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "fine") {
		t.Errorf("a compatible app is listed: %v", err)
	}

	// A non-release binary still parses ranges but does not enforce them.
	skipped, err := checkCoreCompat(snap, "dev")
	if err != nil || !skipped {
		t.Fatalf("dev build: skipped=%v err=%v", skipped, err)
	}
	for _, bad := range []*AppMeta{
		{Name: "x", Ddcore: ">=banana"},
		{Name: "x", Ddcore: "  "}, // present but blank is a typo, not "no range"
		{Name: "x", Version: "one"},
	} {
		if _, err := checkCoreCompat(&Snapshot{Apps: map[string]*AppMeta{"x": bad}}, "dev"); err == nil {
			t.Errorf("expected %+v to be refused even on a dev build", bad)
		}
	}
}
