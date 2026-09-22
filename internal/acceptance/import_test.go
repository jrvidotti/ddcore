package acceptance

// The migration round trip end to end (DAT-01/DAT-02): seed a site, export it
// whole, load it into an empty one, and check the two agree — then run the
// load again and check it changed nothing.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// exportAll writes every DocType of a site to dir, the way `ddcore export
// --all --children` does, manifest included.
func exportAll(t *testing.T, e *engine.Engine, dir string) {
	t.Helper()
	ctx := context.Background()
	man := &engine.ExportManifest{DDCore: engine.Version, Layout: engine.ExportLayout, Format: "ndjson", User: "Admin"}
	for _, name := range e.Meta.Names() {
		d := e.Meta.DocTypes[name]
		if d.IsChild || d.IsSingle {
			continue
		}
		file := strings.ReplaceAll(name, " ", "-") + ".ndjson"
		f, err := os.Create(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		sink := engine.NewNDJSONSink(f)
		var sum *engine.ExportSummary
		err = e.Run(ctx, "Admin", func(c *engine.Ctx) error {
			if err := c.SnapshotIsolation(); err != nil {
				return err
			}
			s, err := c.Export(engine.ExportArgs{Doctype: name, Children: true}, sink)
			sum = s
			return err
		})
		f.Close()
		if err != nil {
			t.Fatalf("export %s: %v", name, err)
		}
		man.Exports = append(man.Exports, &engine.ExportResult{
			Summary: sum, Outputs: []engine.ExportOutput{{File: file}},
		})
	}
	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func countIn(t *testing.T, e *engine.Engine, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := e.DB.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func TestImportRoundTrip(t *testing.T) {
	source := setup(t, "impsrc")
	seedProjects(t, source, 3)
	err := source.Run(context.Background(), "Admin", func(c *engine.Ctx) error {
		for i := 0; i < 2; i++ {
			d, err := c.NewDoc("Task", engine.Doc{
				"code": fmt.Sprintf("T-%04d", i), "title": fmt.Sprintf("Tarefa %d", i),
				"project": "P-0000", "priority": "Medium", "assignee": "Admin",
				"due_date": "2026-06-30",
			})
			if err != nil {
				return err
			}
			if _, err := c.Insert(d, engine.SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	exportAll(t, source, dir)

	target := setup(t, "imptgt")
	ctx := context.Background()
	run, err := target.Import(ctx, engine.ImportArgs{Dir: dir, Actor: "acceptance"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if run.Status != engine.ImportCompleted {
		t.Fatalf("status = %s (%s) errors=%+v dangling=%+v", run.Status, run.Message, run.Errors, run.Dangling)
	}
	if got := countIn(t, target, "SELECT count(*) FROM tab_project"); got != 3 {
		t.Fatalf("projects = %d", got)
	}
	if got := countIn(t, target, "SELECT count(*) FROM tab_project_milestone"); got != 6 {
		t.Fatalf("milestones = %d", got)
	}
	if got := countIn(t, target, "SELECT count(*) FROM tab_task"); got != 2 {
		t.Fatalf("tasks = %d", got)
	}
	// The load runs no controller hook and queues no work: the seeded site's
	// own jobs and versions do not come across as side effects of loading.
	if got := countIn(t, target, "SELECT count(*) FROM ddcore_job"); got != 0 {
		t.Fatalf("jobs = %d; a load must queue nothing", got)
	}

	rep, err := target.ImportReconcile(ctx, engine.ImportArgs{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("reconcile: %+v", rep.Mismatches)
	}

	again, err := target.Import(ctx, engine.ImportArgs{Dir: dir, Actor: "acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range again.Counts {
		if c.Loaded != 0 {
			t.Fatalf("the second run loaded %d %s", c.Loaded, name)
		}
	}
	if got := countIn(t, target, "SELECT count(*) FROM tab_project"); got != 3 {
		t.Fatalf("projects after a second run = %d", got)
	}
}
