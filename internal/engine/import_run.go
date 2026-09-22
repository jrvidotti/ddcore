package engine

// The run: reading an export directory into the site, batch by batch (DAT-01).
//
// Three properties shape everything here.
//
// *Resumable*: the cursor advances in the same transaction as the rows it
// describes, so an interrupted run resumes at the first line that did not
// commit — never before it, never after.
//
// *Idempotent*: every loaded line leaves a ledger row, keyed by where it came
// from. A second run over the same export finds them and writes nothing.
//
// *Per-record*: one unloadable line is a finding, not the end of the run. A
// batch that hits a write error is rolled back and replayed with a savepoint
// per record, which is slower — and is why the first attempt runs without
// them, since past about 64 subtransactions a single transaction starts to
// cost every other session in the database.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Import run statuses.
const (
	ImportRunning             = "running"
	ImportPaused              = "paused"
	ImportCompleted           = "completed"
	ImportCompletedWithErrors = "completed_with_errors"
	ImportFailed              = "failed"
)

// DefaultImportBatch is how many lines one transaction carries.
const DefaultImportBatch = 500

// defaultExcluded are the DocTypes an import leaves alone unless it is asked
// for them by name. Each is a record of something that already happened, or
// carries a secret an export never took: loading them would either fail or
// start firing.
var defaultExcluded = map[string]string{
	"Audit Event":      "the audit ledger is immutable",
	"Error Log":        "a log of this site's own errors",
	"Email Delivery":   "a record of messages already sent",
	"Webhook Delivery": "a record of deliveries already made",
	"Webhook":          "its secret is not exported, and it would start firing",
	"API Key":          "its secret is not exported, so the keys would not work",
}

// ImportArgs is one run.
type ImportArgs struct {
	Dir        string
	Map        *ImportMap
	MapSHA     string
	DryRun     bool
	Batch      int
	Only       []string // load just these source DocTypes
	Include    []string // load these even though they are excluded by default
	Resume     string   // continue this run
	Actor      string
	MaxBatches int // stop after this many batches and leave the run paused
	// VerifyBytes has a reconciliation read every stored attachment back. It
	// is off by default: it reads every byte the site holds for these
	// documents, which on a real migration is the whole file store.
	VerifyBytes bool
}

// ImportCounts is one DocType's tally.
type ImportCounts struct {
	Rows    int `json:"rows"`
	Loaded  int `json:"loaded"`
	Skipped int `json:"skipped"`
	Errors  int `json:"errors"`
	Files   int `json:"files,omitempty"`
}

// ImportError is one line that did not load.
type ImportError struct {
	Doctype string `json:"doctype"`
	ID      string `json:"id,omitempty"`
	Line    int    `json:"line,omitempty"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

// DanglingLink is a link that still points at nothing once the load is done.
type DanglingLink struct {
	Doctype  string `json:"doctype"`
	Field    string `json:"field"`
	ID       string `json:"id"`
	Target   string `json:"target"`
	TargetID string `json:"targetId"`
}

// ImportExclusion is a DocType the run did not load, and why.
type ImportExclusion struct {
	Doctype string `json:"doctype"`
	Reason  string `json:"reason"`
}

// ImportRun is the state of one run, as the ledger holds it.
type ImportRun struct {
	ID       string                   `json:"id"`
	Dir      string                   `json:"dir"`
	Status   string                   `json:"status"`
	Actor    string                   `json:"actor"`
	DryRun   bool                     `json:"dryRun"`
	Started  time.Time                `json:"started"`
	Finished *time.Time               `json:"finished,omitempty"`
	Counts   map[string]*ImportCounts `json:"counts"`
	Cursor   map[string]int           `json:"cursor"`
	Message  string                   `json:"message,omitempty"`
	Errors   []ImportError            `json:"errors,omitempty"`
	Dangling []DanglingLink           `json:"dangling,omitempty"`
	Excluded []ImportExclusion        `json:"excluded,omitempty"`
	Notes    []string                 `json:"notes,omitempty"`
	Order    []string                 `json:"order,omitempty"`
}

// Import loads an export directory. It returns the run either finished or
// paused; a paused run is resumed by passing its id back.
func (e *Engine) Import(ctx context.Context, a ImportArgs) (*ImportRun, error) {
	src, err := OpenImportSource(a.Dir)
	if err != nil {
		return nil, err
	}
	if a.Batch <= 0 {
		a.Batch = DefaultImportBatch
	}
	plan, err := e.importPlan(ctx, src, a)
	if err != nil {
		return nil, err
	}
	run, err := e.openImportRun(ctx, src, a)
	if err != nil {
		return nil, err
	}
	if !a.DryRun {
		// The load forges owner and creation on rows nobody here wrote: that
		// belongs in the ledger every System Manager reads.
		_ = e.RecordAudit(ctx, a.Actor, "import.run", "Allowed", "", run.ID, map[string]any{
			"dir": a.Dir, "phase": "start", "doctypes": len(plan.stages),
		})
	}
	run.Excluded = plan.excluded
	run.Notes = plan.notes
	for _, s := range plan.stages {
		run.Order = append(run.Order, s.source)
	}

	// A dry run holds one transaction for the whole load and rolls it back at
	// the end. Anything narrower would be a rehearsal of a different thing:
	// with a transaction per batch, a Project rolled back before its Tasks are
	// read makes every link to it look broken.
	loadAll := func(shared *Ctx) error {
		batches := 0
		for _, stage := range plan.stages {
			reader, err := src.Open(stage.source)
			if err != nil {
				return err
			}
			// One reader per DocType, held open across its batches: reopening
			// the file to skip to the cursor each batch would re-parse the
			// whole prefix.
			if err := reader.SkipTo(run.Cursor[stage.source]); err != nil {
				reader.Close()
				return err
			}
			for {
				if a.MaxBatches > 0 && batches >= a.MaxBatches {
					reader.Close()
					run.Status = ImportPaused
					return nil
				}
				n, err := e.importBatch(ctx, shared, src, reader, a, run, stage)
				if err != nil {
					reader.Close()
					return err
				}
				batches++
				if n == 0 {
					break
				}
			}
			reader.Close()
		}
		return e.verifyDeferredLinks(ctx, queryOf(e, shared), run, plan)
	}

	if a.DryRun {
		c := e.NewCtx(ctx, "Admin")
		c.Flags["rollback"] = true
		err = c.Run(func(c *Ctx) error { return loadAll(c) })
	} else {
		err = loadAll(nil)
	}
	if err != nil {
		run.Status, run.Message = ImportFailed, err.Error()
		_ = e.saveImportRun(ctx, run)
		return run, err
	}
	if run.Status == ImportPaused {
		return run, e.saveImportRun(ctx, run)
	}
	run.Status = ImportCompleted
	for _, c := range run.Counts {
		if c.Errors > 0 {
			run.Status = ImportCompletedWithErrors
		}
	}
	if len(run.Dangling) > 0 {
		run.Status = ImportCompletedWithErrors
	}
	now := time.Now()
	run.Finished = &now
	if !a.DryRun {
		loaded, skipped, failed := 0, 0, 0
		for _, c := range run.Counts {
			loaded, skipped, failed = loaded+c.Loaded, skipped+c.Skipped, failed+c.Errors
		}
		_ = e.RecordAudit(ctx, a.Actor, "import.run", "Allowed", "", run.ID, map[string]any{
			"dir": a.Dir, "phase": "finish", "status": run.Status,
			"loaded": loaded, "skipped": skipped, "errors": failed, "dangling": len(run.Dangling),
		})
	}
	return run, e.saveImportRun(ctx, run)
}

// queryOf is the querier a phase runs on: the dry run's own transaction, or
// the pool when the batches committed on their own.
func queryOf(e *Engine, shared *Ctx) db.Querier {
	if shared != nil {
		return shared.Q()
	}
	return e.DB.Pool
}

// importStage is one DocType's place in the load.
type importStage struct {
	source, target string
	file           string
	deferred       map[string]bool
}

type importPlanned struct {
	stages   []importStage
	excluded []ImportExclusion
	notes    []string
}

// importPlan decides what is loaded and in what order, without writing.
func (e *Engine) importPlan(ctx context.Context, src *ImportSource, a ImportArgs) (*importPlanned, error) {
	st := e.Current()
	only := map[string]bool{}
	for _, n := range a.Only {
		only[n] = true
	}
	include := map[string]bool{}
	for _, n := range a.Include {
		include[n] = true
	}
	plan := &importPlanned{}
	targets := map[string]importStage{}
	var names []string
	for _, source := range src.Doctypes() {
		target := a.Map.Target(source)
		switch {
		case len(only) > 0 && !only[source]:
			continue
		case a.Map.Excluded(source):
			plan.excluded = append(plan.excluded, ImportExclusion{source, "the mapping leaves it out"})
			continue
		}
		if reason, skip := defaultExcluded[target]; skip && !include[source] {
			plan.excluded = append(plan.excluded, ImportExclusion{source, reason})
			continue
		}
		d, err := st.DocType(target)
		if err != nil {
			plan.excluded = append(plan.excluded, ImportExclusion{source, "this site has no " + target})
			continue
		}
		if d.IsChild || d.IsSingle {
			plan.excluded = append(plan.excluded, ImportExclusion{source, "a child table or a Single is not loaded on its own"})
			continue
		}
		res := src.Result(source)
		if res != nil && res.Summary != nil && res.Summary.Truncated {
			plan.notes = append(plan.notes, fmt.Sprintf("%s was exported with a limit: the file holds a sample, not the set", source))
		}
		if res != nil && res.Summary != nil {
			plan.notes = append(plan.notes, missingColumnNotes(st, d.Name, source, res.Summary.Columns)...)
		}
		targets[target] = importStage{source: source, target: target}
		names = append(names, target)
	}
	order, err := ImportOrder(names, st.DocType)
	if err != nil {
		return nil, err
	}
	for _, s := range order {
		stage := targets[s.Doctype]
		stage.deferred = map[string]bool{}
		for _, f := range s.Deferred {
			stage.deferred[f] = true
		}
		plan.stages = append(plan.stages, stage)
	}
	return plan, nil
}

// missingColumnNotes warns when the export was taken by a user who could not
// read every field: those columns are simply absent, and the load would write
// their defaults without saying so.
func missingColumnNotes(st *State, target, source string, columns []string) []string {
	if len(columns) == 0 {
		return nil
	}
	d, err := st.DocType(target)
	if err != nil {
		return nil
	}
	have := map[string]bool{}
	for _, c := range columns {
		have[c] = true
	}
	var missing []string
	for _, f := range d.DataFields() {
		if f.Fieldtype == "Vault" || f.Fieldtype == "Password" {
			continue
		}
		if !have[f.Fieldname] {
			missing = append(missing, f.Fieldname)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []string{fmt.Sprintf("%s: the export does not carry %s", source, strings.Join(missing, ", "))}
}

// verifyDeferredLinks is the finalize pass: every link the load order could
// not satisfy — a self link, a cycle, a Dynamic Link — checked at once, now
// that the whole set is in. One anti-join per field beats one query per row.
func (e *Engine) verifyDeferredLinks(ctx context.Context, q db.Querier, run *ImportRun, plan *importPlanned) error {
	st := e.Current()
	for _, stage := range plan.stages {
		if len(stage.deferred) == 0 {
			continue
		}
		d, err := st.DocType(stage.target)
		if err != nil {
			return err
		}
		for field := range stage.deferred {
			doctype, fieldname, isChild := strings.Cut(field, ".")
			owner := d
			if isChild {
				tf := d.Field(doctype)
				if tf == nil {
					continue
				}
				owner, err = st.DocType(tf.OptionsString())
				if err != nil {
					return err
				}
			} else {
				fieldname = field
			}
			f := owner.Field(fieldname)
			if f == nil {
				continue
			}
			dangling, err := e.danglingLinks(ctx, q, owner, f.Fieldname, f)
			if err != nil {
				return err
			}
			run.Dangling = append(run.Dangling, dangling...)
		}
	}
	sort.Slice(run.Dangling, func(i, j int) bool {
		if run.Dangling[i].Doctype != run.Dangling[j].Doctype {
			return run.Dangling[i].Doctype < run.Dangling[j].Doctype
		}
		return run.Dangling[i].ID < run.Dangling[j].ID
	})
	return nil
}

// danglingLinks finds the rows of one field that point at nothing. A Dynamic
// Link is checked per target DocType, since its target is a value.
func (e *Engine) danglingLinks(ctx context.Context, q db.Querier, d *meta.DocType, fieldname string, f *meta.Field) ([]DanglingLink, error) {
	var targets []string
	switch f.Fieldtype {
	case "Link":
		targets = []string{f.OptionsString()}
	case "Dynamic Link":
		rows, err := db.Select(ctx, q, fmt.Sprintf(`SELECT DISTINCT %s AS t FROM %s WHERE %s IS NOT NULL AND %s <> ''`,
			db.Ident(f.OptionsString()), db.Ident(d.TableName()), db.Ident(fieldname), db.Ident(fieldname)), nil)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if t := db.Str(r["t"]); t != "" {
				targets = append(targets, t)
			}
		}
	default:
		return nil, nil
	}
	st := e.Current()
	var out []DanglingLink
	for _, target := range targets {
		td, err := st.DocType(target)
		if err != nil {
			continue
		}
		where := ""
		args := []any{}
		if f.Fieldtype == "Dynamic Link" {
			where = fmt.Sprintf(" AND s.%s = $1", db.Ident(f.OptionsString()))
			args = append(args, target)
		}
		query := fmt.Sprintf(`SELECT s.id, s.%[1]s AS ref FROM %[2]s s
			LEFT JOIN %[3]s t ON t.id = s.%[1]s
			WHERE s.%[1]s IS NOT NULL AND s.%[1]s <> '' AND t.id IS NULL%[4]s LIMIT 200`,
			db.Ident(fieldname), db.Ident(d.TableName()), db.Ident(td.TableName()), where)
		rows, err := db.Select(ctx, q, query, args...)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, DanglingLink{Doctype: d.Name, Field: fieldname, ID: db.Str(r["id"]),
				Target: target, TargetID: db.Str(r["ref"])})
		}
	}
	return out, nil
}
