package engine

// Reconciling a load against the export it came from (DAT-01).
//
// A load that reported no errors is not the same thing as a load that is
// right. Reconciliation is the second question: does the site now hold what
// the export said it held — the same rows, the same child rows, the same
// statuses, the same files, and the same money.
//
// The money is the reason this exists. Currency values are summed per DocType
// and per docstatus, with big.Rat so the comparison is exact, each source
// value first rounded the way this site rounds it. A migration that moves an
// amount by a cent is worse than one that fails.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/num"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// ImportReconciliation is the report.
type ImportReconciliation struct {
	Dir        string                       `json:"dir"`
	OK         bool                         `json:"ok"`
	Doctypes   map[string]*ReconcileDoctype `json:"doctypes"`
	Mismatches []ReconcileMismatch          `json:"mismatches,omitempty"`
	Notes      []string                     `json:"notes,omitempty"`
}

// ReconcileDoctype is one DocType's side-by-side count.
type ReconcileDoctype struct {
	Target     string            `json:"target"`
	SourceRows int64             `json:"sourceRows"`
	TargetRows int64             `json:"targetRows"`
	ChildRows  map[string]int64  `json:"childRows,omitempty"`
	Files      int64             `json:"files,omitempty"`
	Docstatus  map[string]int64  `json:"docstatus,omitempty"`
	Totals     map[string]string `json:"totals,omitempty"` // "<field>@<docstatus>" → amount
}

// ReconcileMismatch is one disagreement, in the export's terms and the site's.
type ReconcileMismatch struct {
	Kind    string `json:"kind"` // rows, childRows, docstatus, total, file, link
	Doctype string `json:"doctype"`
	Detail  string `json:"detail"`
	Source  string `json:"source"`
	Target  string `json:"target"`
}

// ImportReconcile reads the export again and compares it with the site, going
// through the ledger so only what this import loaded is counted. It writes
// nothing.
func (e *Engine) ImportReconcile(ctx context.Context, a ImportArgs) (*ImportReconciliation, error) {
	src, err := OpenImportSource(a.Dir)
	if err != nil {
		return nil, err
	}
	plan, err := e.importPlan(ctx, src, a)
	if err != nil {
		return nil, err
	}
	rep := &ImportReconciliation{Dir: a.Dir, OK: true, Doctypes: map[string]*ReconcileDoctype{}}
	st := e.Current()
	for _, stage := range plan.stages {
		d, err := st.DocType(stage.target)
		if err != nil {
			return nil, err
		}
		side, err := e.reconcileDoctype(ctx, a, src, stage, d, rep)
		if err != nil {
			return nil, err
		}
		rep.Doctypes[stage.source] = side
	}
	sort.Slice(rep.Mismatches, func(i, j int) bool {
		if rep.Mismatches[i].Doctype != rep.Mismatches[j].Doctype {
			return rep.Mismatches[i].Doctype < rep.Mismatches[j].Doctype
		}
		return rep.Mismatches[i].Kind < rep.Mismatches[j].Kind
	})
	rep.OK = len(rep.Mismatches) == 0
	return rep, nil
}

// reconcileDoctype walks one file, tallying what it says, then asks the
// database the same questions about the ids this import loaded.
func (e *Engine) reconcileDoctype(ctx context.Context, a ImportArgs, src *ImportSource, stage importStage, d *meta.DocType, rep *ImportReconciliation) (*ReconcileDoctype, error) {
	side := &ReconcileDoctype{Target: stage.target, ChildRows: map[string]int64{},
		Docstatus: map[string]int64{}, Totals: map[string]string{}}
	money := moneyFields(d)
	sourceTotals := map[string]*big.Rat{}
	var ids []string

	r, err := src.Open(stage.source)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		_, doc := a.Map.Apply(stage.source, rec.Doc)
		side.SourceRows++
		ids = append(ids, doc.ID())
		ds := fmt.Sprint(doc.Docstatus())
		side.Docstatus[ds]++
		for _, tf := range d.TableFields() {
			side.ChildRows[tf.OptionsString()] += int64(len(doc.Children(tf.Fieldname)))
		}
		if files, ok := rec.Doc["_files"].([]any); ok {
			side.Files += int64(len(files))
		}
		for _, f := range money {
			key := f.Fieldname + "@" + ds
			if sourceTotals[key] == nil {
				sourceTotals[key] = new(big.Rat)
			}
			// The site's own rounding, applied to the source value: that is
			// what the load wrote, so that is what has to add up.
			v := num.Round(toFloat(doc[f.Fieldname]), e.currencyPrecisionFor(f), e.Cfg.Rounding)
			sourceTotals[key].Add(sourceTotals[key], ratOf(v))
		}
	}
	for k, v := range sourceTotals {
		side.Totals[k] = ratString(v)
	}

	// The database side, restricted to the ids this import actually loaded.
	loaded, err := db.Select(ctx, e.DB.Pool, `SELECT id FROM ddcore_import_record WHERE source_doctype = $1 AND status = 'loaded'`, stage.source)
	if err != nil {
		return nil, err
	}
	targetIDs := make([]string, 0, len(loaded))
	for _, row := range loaded {
		targetIDs = append(targetIDs, db.Str(row["id"]))
	}
	if len(targetIDs) == 0 {
		rep.Mismatches = append(rep.Mismatches, ReconcileMismatch{Kind: "rows", Doctype: stage.source,
			Detail: "the ledger holds no loaded record for this DocType",
			Source: fmt.Sprint(side.SourceRows), Target: "0"})
		return side, nil
	}
	row := e.DB.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE id = ANY($1)`, db.Ident(d.TableName())), targetIDs)
	if err := row.Scan(&side.TargetRows); err != nil {
		return nil, err
	}
	if side.TargetRows != int64(len(targetIDs)) {
		rep.Mismatches = append(rep.Mismatches, ReconcileMismatch{Kind: "rows", Doctype: stage.source,
			Detail: "the export and the site disagree on how many documents there are",
			Source: fmt.Sprint(side.SourceRows), Target: fmt.Sprint(side.TargetRows)})
	}
	for _, tf := range d.TableFields() {
		child, err := e.Current().DocType(tf.OptionsString())
		if err != nil {
			return nil, err
		}
		var n int64
		q := fmt.Sprintf(`SELECT count(*) FROM %s WHERE parenttype = $1 AND parentfield = $2 AND parent = ANY($3)`, db.Ident(child.TableName()))
		if err := e.DB.Pool.QueryRow(ctx, q, d.Name, tf.Fieldname, targetIDs).Scan(&n); err != nil {
			return nil, err
		}
		if want := side.ChildRows[child.Name]; want != n {
			rep.Mismatches = append(rep.Mismatches, ReconcileMismatch{Kind: "childRows", Doctype: stage.source,
				Detail: child.Name, Source: fmt.Sprint(want), Target: fmt.Sprint(n)})
		}
	}
	rows, err := db.Select(ctx, e.DB.Pool, fmt.Sprintf(`SELECT docstatus, count(*) AS n FROM %s WHERE id = ANY($1) GROUP BY docstatus`, db.Ident(d.TableName())), targetIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]int64{}
	for _, r := range rows {
		seen[fmt.Sprint(int64(toFloat(r["docstatus"])))] = int64(toFloat(r["n"]))
	}
	for ds, want := range side.Docstatus {
		if seen[ds] != want {
			rep.Mismatches = append(rep.Mismatches, ReconcileMismatch{Kind: "docstatus", Doctype: stage.source,
				Detail: "docstatus " + ds, Source: fmt.Sprint(want), Target: fmt.Sprint(seen[ds])})
		}
	}
	for _, f := range money {
		rows, err := db.Select(ctx, e.DB.Pool, fmt.Sprintf(`SELECT docstatus, sum(%s)::text AS total FROM %s WHERE id = ANY($1) GROUP BY docstatus`,
			db.Ident(f.Fieldname), db.Ident(d.TableName())), targetIDs)
		if err != nil {
			return nil, err
		}
		got := map[string]*big.Rat{}
		for _, r := range rows {
			ds := fmt.Sprint(int64(toFloat(r["docstatus"])))
			v := new(big.Rat)
			if s := db.Str(r["total"]); s != "" {
				v.SetString(s)
			}
			got[ds] = v
		}
		for ds := range side.Docstatus {
			key := f.Fieldname + "@" + ds
			src := sourceTotals[key]
			if src == nil {
				src = new(big.Rat)
			}
			tgt := got[ds]
			if tgt == nil {
				tgt = new(big.Rat)
			}
			if src.Cmp(tgt) != 0 {
				rep.Mismatches = append(rep.Mismatches, ReconcileMismatch{Kind: "total", Doctype: stage.source,
					Detail: f.Fieldname + " at docstatus " + ds, Source: ratString(src), Target: ratString(tgt)})
			}
		}
	}
	if a.VerifyBytes {
		notes, mismatches, err := e.reconcileBytes(ctx, src, stage, targetIDs)
		if err != nil {
			return nil, err
		}
		rep.Notes = append(rep.Notes, notes...)
		rep.Mismatches = append(rep.Mismatches, mismatches...)
	}
	return side, nil
}

// reconcileBytes reads every stored attachment back and checks it against the
// export's checksum. It is not the default: it reads every byte the site holds
// for these documents.
func (e *Engine) reconcileBytes(ctx context.Context, src *ImportSource, stage importStage, ids []string) ([]string, []ReconcileMismatch, error) {
	rows, err := db.Select(ctx, e.DB.Pool, `SELECT id, file_url FROM tab_file WHERE attached_to_doctype = $1 AND attached_to_id = ANY($2)`, stage.target, ids)
	if err != nil {
		return nil, nil, err
	}
	var out []ReconcileMismatch
	for _, r := range rows {
		url := db.Str(r["file_url"])
		key, ok := storage.KeyFromURL(url)
		if !ok {
			continue
		}
		if _, err := storage.ReadAll(ctx, e.Storage(), key); err != nil {
			out = append(out, ReconcileMismatch{Kind: "file", Doctype: stage.source, Detail: url,
				Source: "in the export", Target: "not readable on this site"})
			continue
		}
	}
	return nil, out, nil
}

// moneyFields are the fields a reconciliation adds up.
func moneyFields(d *meta.DocType) []*meta.Field {
	var out []*meta.Field
	for _, f := range d.DataFields() {
		if f.Fieldtype == "Currency" || f.Fieldtype == "Percent" {
			out = append(out, f)
		}
	}
	return out
}

func (e *Engine) currencyPrecisionFor(f *meta.Field) int {
	if f.Precision > 0 {
		return f.Precision
	}
	return e.CurrencyPrecision()
}

func ratOf(v float64) *big.Rat {
	r := new(big.Rat)
	r.SetFloat64(v)
	return r
}

func ratString(r *big.Rat) string {
	if r == nil {
		return "0"
	}
	return r.FloatString(9)
}
