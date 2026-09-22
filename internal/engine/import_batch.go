package engine

// One batch of a run, and the ledger that makes the next one resumable.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

// openImportRun finds the run to continue, or starts one.
func (e *Engine) openImportRun(ctx context.Context, src *ImportSource, a ImportArgs) (*ImportRun, error) {
	if a.Resume != "" {
		run, err := e.ImportRunByID(ctx, a.Resume)
		if err != nil {
			return nil, err
		}
		if run.Dir != a.Dir {
			return nil, cerr.Validation("run %s loaded %s, not %s", a.Resume, run.Dir, a.Dir)
		}
		run.Status = ImportRunning
		return run, nil
	}
	run := &ImportRun{
		ID: randomID(), Dir: a.Dir, Status: ImportRunning, Actor: a.Actor, DryRun: a.DryRun,
		Started: time.Now(), Counts: map[string]*ImportCounts{}, Cursor: map[string]int{},
	}
	if a.DryRun {
		// A dry run writes no ledger: there is nothing to resume, and the run
		// row itself would be the one trace it left behind.
		return run, nil
	}
	_, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_import_run (id, dir, manifest_sha, mapping_sha, status, actor, dry_run, started, cursor, counts)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'{}'::jsonb,'{}'::jsonb)`,
		run.ID, a.Dir, manifestSHA(src), a.MapSHA, run.Status, a.Actor, a.DryRun, run.Started)
	if err != nil {
		return nil, err
	}
	return run, nil
}

// ImportRunByID reads a run from the ledger.
func (e *Engine) ImportRunByID(ctx context.Context, id string) (*ImportRun, error) {
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT id, dir, status, actor, dry_run, started, finished, cursor, counts, message
		FROM ddcore_import_run WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, cerr.NotFound("no import run %s", id)
	}
	return importRunFromRow(rows[0])
}

// ImportRuns lists the runs, newest first.
func (e *Engine) ImportRuns(ctx context.Context, limit int) ([]*ImportRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT id, dir, status, actor, dry_run, started, finished, cursor, counts, message
		FROM ddcore_import_run ORDER BY started DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*ImportRun, 0, len(rows))
	for _, r := range rows {
		run, err := importRunFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func importRunFromRow(r map[string]any) (*ImportRun, error) {
	run := &ImportRun{
		ID: db.Str(r["id"]), Dir: db.Str(r["dir"]), Status: db.Str(r["status"]),
		Actor: db.Str(r["actor"]), Message: db.Str(r["message"]),
		Counts: map[string]*ImportCounts{}, Cursor: map[string]int{},
	}
	if b, ok := r["dry_run"].(bool); ok {
		run.DryRun = b
	}
	// db.Select normalises a timestamptz to an RFC3339 string on its way out.
	if t := parseTime(r["started"], time.UTC); !t.IsZero() {
		run.Started = t
	}
	if t := parseTime(r["finished"], time.UTC); !t.IsZero() {
		run.Finished = &t
	}
	if err := decodeJSONColumn(r["counts"], &run.Counts); err != nil {
		return nil, err
	}
	if err := decodeJSONColumn(r["cursor"], &run.Cursor); err != nil {
		return nil, err
	}
	return run, nil
}

func decodeJSONColumn(v any, into any) error {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

// saveImportRun writes the run's state; a dry run has no row to write to.
func (e *Engine) saveImportRun(ctx context.Context, run *ImportRun) error {
	if run.DryRun {
		return nil
	}
	counts, err := json.Marshal(run.Counts)
	if err != nil {
		return err
	}
	cursor, err := json.Marshal(run.Cursor)
	if err != nil {
		return err
	}
	_, err = e.DB.Pool.Exec(ctx, `UPDATE ddcore_import_run SET status=$2, finished=$3, counts=$4, cursor=$5, message=$6 WHERE id=$1`,
		run.ID, run.Status, run.Finished, counts, cursor, run.Message)
	return err
}

func manifestSHA(src *ImportSource) string {
	b, err := json.Marshal(src.Manifest)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func lineSHA(doc Doc) string {
	b, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// importBatch loads up to a.Batch lines of one DocType in one transaction,
// advancing the cursor inside it. It returns how many lines it read; zero
// means the file is done.
func (e *Engine) importBatch(ctx context.Context, shared *Ctx, src *ImportSource, reader *ImportReader, a ImportArgs, run *ImportRun, stage importStage) (int, error) {
	lines, err := readImportBatch(reader, a.Batch)
	if err != nil {
		return 0, err
	}
	if len(lines) == 0 {
		return 0, nil
	}
	// First attempt: no savepoints. A write error rolls the batch back and it
	// is replayed one savepoint at a time.
	outcome, err := e.runImportBatch(ctx, shared, a, src, run, stage, lines, false)
	if err != nil {
		outcome, err = e.runImportBatch(ctx, shared, a, src, run, stage, lines, true)
		if err != nil {
			return 0, err
		}
	}
	counts := run.Counts[stage.source]
	if counts == nil {
		counts = &ImportCounts{}
		run.Counts[stage.source] = counts
	}
	counts.Rows += len(lines)
	counts.Loaded += outcome.loaded
	counts.Skipped += outcome.skipped
	counts.Files += outcome.files
	counts.Errors += len(outcome.errors)
	run.Errors = append(run.Errors, outcome.errors...)
	run.Notes = append(run.Notes, outcome.notes...)
	run.Cursor[stage.source] = lines[len(lines)-1].Line
	return len(lines), nil
}

func readImportBatch(r *ImportReader, size int) ([]ImportRecord, error) {
	var out []ImportRecord
	for len(out) < size {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

type batchOutcome struct {
	loaded, skipped, files int
	errors                 []ImportError
	notes                  []string
}

// runImportBatch writes one batch. In careful mode each record gets its own
// savepoint, so a write error costs that record and not the batch.
func (e *Engine) runImportBatch(ctx context.Context, shared *Ctx, a ImportArgs, src *ImportSource, run *ImportRun, stage importStage, lines []ImportRecord, careful bool) (batchOutcome, error) {
	var out batchOutcome
	batch := func(c *Ctx) error {
		out = batchOutcome{}
		for _, rec := range lines {
			one := func() error { return e.importOne(c, a, src, run, stage, rec, &out) }
			var err error
			if careful {
				err = c.WithSavepoint(one)
			} else {
				err = one()
			}
			if err == nil {
				continue
			}
			if !careful {
				// Let the whole batch roll back and be replayed carefully:
				// after a failed statement the transaction is unusable.
				return err
			}
			out.errors = append(out.errors, ImportError{
				Doctype: stage.source, ID: rec.Doc.ID(), Line: rec.Line,
				Phase: "record", Message: cerr.From(err).Message,
			})
			if err := e.recordImportError(ctx, run, stage.source, rec, err); err != nil {
				return err
			}
		}
		if a.DryRun {
			return nil
		}
		return e.writeCursor(c, run, stage.source, lines[len(lines)-1].Line, out)
	}
	if shared != nil {
		// Inside the dry run's one transaction: a savepoint around the batch
		// so a failed attempt can be replayed carefully without taking the
		// rest of the rehearsal down with it.
		return out, shared.WithSavepoint(func() error { return batch(shared) })
	}
	c := e.importCtx(ctx, a)
	return out, c.Run(batch)
}

// importOne loads a single line: map it, decide what to do about a document
// that is already there, write it, and leave a ledger row.
func (e *Engine) importOne(c *Ctx, a ImportArgs, src *ImportSource, run *ImportRun, stage importStage, rec ImportRecord, out *batchOutcome) error {
	source := stage.source
	target, doc := a.Map.Apply(source, rec.Doc)
	if target == "" {
		target = stage.target
	}
	doc["doctype"] = stage.target
	id := doc.ID()
	if id == "" {
		return cerr.Validation("the record has no id")
	}
	known, err := e.ledgerHas(c, source, rec.Doc.ID())
	if err != nil {
		return err
	}
	if known {
		out.skipped++
		return nil
	}
	exists, err := c.idExists(stage.target, id)
	if err != nil {
		return err
	}
	if exists {
		switch a.Map.OnExisting(source) {
		case OnExistingSkip:
			out.skipped++
			return e.ledgerWrite(c, a, run.ID, source, rec, stage.target, id, "skipped")
		case OnExistingError:
			return cerr.Duplicate("%s %s is already on this site", stage.target, id)
		default:
			same, err := c.sameAsStored(stage.target, doc)
			if err != nil {
				return err
			}
			if !same {
				return cerr.Duplicate("%s %s is already on this site and differs from the export", stage.target, id)
			}
			out.skipped++
			return e.ledgerWrite(c, a, run.ID, source, rec, stage.target, id, "skipped")
		}
	}
	res, err := c.ImportDoc(doc, ImportOpts{Deferred: func(doctype, fieldname string) bool {
		return stage.deferred[fieldname]
	}})
	if err != nil {
		return err
	}
	out.loaded++
	for _, n := range res.Notes {
		out.notes = append(out.notes, stage.source+" "+id+": "+n)
	}
	files, notes, err := c.importAttachments(a, src, stage, rec, doc)
	if err != nil {
		return err
	}
	out.files += files
	out.notes = append(out.notes, notes...)
	return e.ledgerWrite(c, a, run.ID, source, rec, stage.target, id, "loaded")
}

func (e *Engine) ledgerHas(c *Ctx, sourceDoctype, sourceID string) (bool, error) {
	var n int
	err := c.Q().QueryRow(c.Ctx, `SELECT count(*) FROM ddcore_import_record WHERE source_doctype=$1 AND source_id=$2`,
		sourceDoctype, sourceID).Scan(&n)
	return n > 0, err
}

func (e *Engine) ledgerWrite(c *Ctx, a ImportArgs, runID, sourceDoctype string, rec ImportRecord, doctype, id, status string) error {
	if a.DryRun {
		return nil
	}
	_, err := c.Q().Exec(c.Ctx, `INSERT INTO ddcore_import_record (run_id, source_doctype, source_id, doctype, id, line_sha, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, runID, sourceDoctype, rec.Doc.ID(), doctype, id, lineSHA(rec.Doc), status)
	return err
}

// writeCursor moves the run's cursor in the same transaction as the rows it
// describes. That is what makes a resume exact rather than approximate.
func (e *Engine) writeCursor(c *Ctx, run *ImportRun, source string, line int, out batchOutcome) error {
	cursor := map[string]int{}
	for k, v := range run.Cursor {
		cursor[k] = v
	}
	cursor[source] = line
	b, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	_, err = c.Q().Exec(c.Ctx, `UPDATE ddcore_import_run SET cursor = $2 WHERE id = $1`, run.ID, b)
	return err
}

// recordImportError writes on the pool, outside the batch's transaction, so a
// rolled-back record still leaves its reason behind.
func (e *Engine) recordImportError(ctx context.Context, run *ImportRun, doctype string, rec ImportRecord, cause error) error {
	if run.DryRun {
		return nil
	}
	_, err := e.DB.Pool.Exec(ctx, `INSERT INTO ddcore_import_error (run_id, doctype, source_id, line, phase, message)
		VALUES ($1,$2,$3,$4,$5,$6)`, run.ID, doctype, rec.Doc.ID(), rec.Line, "record", cerr.From(cause).Message)
	return err
}

// sameAsStored compares what the export carries with what the site holds,
// field by field and ignoring metadata: the same document loaded twice is a
// skip, a different one is a conflict.
func (c *Ctx) sameAsStored(doctype string, doc Doc) (bool, error) {
	stored, err := c.GetDocIgnoringPerms(doctype, doc.ID())
	if err != nil {
		return false, err
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return false, err
	}
	for _, f := range d.DataFields() {
		if f.Fieldtype == "Vault" || f.Fieldtype == "Password" {
			continue
		}
		in, ok := doc[f.Fieldname]
		if !ok {
			continue
		}
		want, err := c.castValue(f, in)
		if err != nil {
			return false, err
		}
		if fmt.Sprint(want) != fmt.Sprint(stored[f.Fieldname]) {
			return false, nil
		}
	}
	return true, nil
}

// ImportRunErrors are the lines a run could not load.
func (e *Engine) ImportRunErrors(ctx context.Context, runID string, limit int) ([]ImportError, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT doctype, source_id, line, phase, message
		FROM ddcore_import_error WHERE run_id = $1 ORDER BY id LIMIT $2`, runID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ImportError, 0, len(rows))
	for _, r := range rows {
		out = append(out, ImportError{
			Doctype: db.Str(r["doctype"]), ID: db.Str(r["source_id"]),
			Line: int(toFloat(r["line"])), Phase: db.Str(r["phase"]), Message: db.Str(r["message"]),
		})
	}
	return out, nil
}
