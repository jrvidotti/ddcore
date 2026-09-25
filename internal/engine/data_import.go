package engine

// Data Import: rows from a CSV or XLSX file, written by the person who
// uploaded it through the ordinary document path.
//
// This is not `ddcore import`. That one loads another site's history as it
// was — ids, owners, timestamps — with no hook and no side effect, and only an
// administrator may run it. A spreadsheet is new business data: each row is an
// Insert or a Save as the user, so roles, scopes, field levels, controller
// hooks, naming, workflows, webhooks and Version all apply exactly as they do
// to a form. The reader lives in internal/tabular; this file maps its columns
// onto fields, turns each cell into the value the field expects, and reports
// every row's outcome.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/tabular"
)

// DataImportArgs is one upload.
type DataImportArgs struct {
	Doctype  string
	Mode     string // "insert" (default) | "update"
	FileName string
	File     []byte
	DryRun   bool
	// Sep is the CSV separator; 0 sniffs it from the header line.
	Sep rune
	// Decimal is the decimal separator a number cell is written with, "." or
	// ","; the thousands separator is the other one.
	Decimal string
	// DateOrder is how a date that is not ISO is read: "dmy", "mdy" or "ymd".
	DateOrder string
	// Columns overrides the matching: header → fieldname, or "" to ignore it.
	Columns map[string]string
	MaxRows int
}

// DataImportFile describes what was read.
type DataImportFile struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Sep    string `json:"sep,omitempty"`
	SHA256 string `json:"sha256"`
}

// DataImportColumn is what one column of the file was taken to be.
type DataImportColumn struct {
	Index     int    `json:"index"`
	Header    string `json:"header"`
	Fieldname string `json:"fieldname,omitempty"`
	Label     string `json:"label,omitempty"`
	// Status is "mapped", "ignored" or "unknown".
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// DataImportField is a field a column may be mapped onto.
type DataImportField struct {
	Fieldname string `json:"fieldname"`
	Label     string `json:"label"`
	Reqd      bool   `json:"reqd,omitempty"`
}

// DataImportRow is one row's outcome. Cells are returned for the rows that
// failed, so the caller can hand the person back a file of just those rows.
type DataImportRow struct {
	Row     int      `json:"row"`
	Status  string   `json:"status"` // "inserted" | "updated" | "error"
	ID      string   `json:"id,omitempty"`
	Type    string   `json:"type,omitempty"`
	Message string   `json:"message,omitempty"`
	Cells   []string `json:"cells,omitempty"`
}

type DataImportCounts struct {
	Rows     int `json:"rows"`
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Errors   int `json:"errors"`
}

type DataImportResult struct {
	Doctype string             `json:"doctype"`
	Mode    string             `json:"mode"`
	DryRun  bool               `json:"dryRun"`
	File    DataImportFile     `json:"file"`
	Headers []string           `json:"headers"`
	Columns []DataImportColumn `json:"columns"`
	Fields  []DataImportField  `json:"fields"`
	Counts  DataImportCounts   `json:"counts"`
	Rows    []DataImportRow    `json:"rows"`
}

// dataImportPlan is everything settled before the first row is written.
type dataImportPlan struct {
	d        *meta.DocType
	mode     string
	sheet    *tabular.Sheet
	idCol    int // -1 when the file has none (or it is ignored)
	modCol   int // the `modified` column an update checks against, or -1
	cols     map[int]*meta.Field
	labels   map[string]string // fieldname → label in the request language
	conv     cellConv
	columns  []DataImportColumn
	fields   []DataImportField
	sha      string
	fileName string
}

// fork is a fresh unit of work for the same caller: every row of a real
// import is its own transaction, and a Ctx carries its document cache and its
// after-commit callbacks from one transaction to the next.
func (c *Ctx) fork() *Ctx {
	n := c.E.NewCtx(c.Ctx, c.User)
	n.Lang, n.Request, n.Sid, n.ReqID = c.Lang, c.Request, c.Sid, c.ReqID
	return n
}

// DataImport reads the file and writes its rows. c must not be inside a
// transaction: a dry run is one rolled-back transaction with a savepoint per
// row, and a real run commits every row on its own, so one bad row costs that
// row and never the ones around it.
//
// Errors returned are about the upload as a whole — the permission, the file,
// the columns. A row that fails is reported in the result, not returned.
func (c *Ctx) DataImport(a DataImportArgs) (*DataImportResult, error) {
	var plan *dataImportPlan
	if err := c.fork().Run(func(tc *Ctx) error {
		var err error
		plan, err = tc.planDataImport(a)
		return err
	}); err != nil {
		return nil, err
	}

	res := &DataImportResult{
		Doctype: plan.d.Name, Mode: plan.mode, DryRun: a.DryRun,
		File:    DataImportFile{Name: plan.fileName, Format: plan.sheet.Format, SHA256: plan.sha},
		Headers: plan.sheet.Headers, Columns: plan.columns, Fields: plan.fields,
		Rows: make([]DataImportRow, 0, len(plan.sheet.Rows)),
	}
	if plan.sheet.Sep != 0 {
		res.File.Sep = string(plan.sheet.Sep)
	}
	record := func(tc *Ctx, row tabular.Row, id, status string, err error) {
		out := DataImportRow{Row: row.Line, Status: status, ID: id}
		if err != nil {
			e := cerr.From(err)
			if e.Key != "" {
				e = e.Translate(func(k string, args ...any) string { return tc.T(k, args...) })
			}
			if e.Type == "InternalError" {
				c.E.Log.Warn("data import row failed", "doctype", plan.d.Name, "row", row.Line, "err", err)
			}
			out.Status, out.Type, out.Message = "error", e.Type, e.Message
			out.Cells = row.Texts(len(plan.sheet.Headers))
			res.Counts.Errors++
		} else if status == "inserted" {
			res.Counts.Inserted++
		} else {
			res.Counts.Updated++
		}
		res.Counts.Rows++
		res.Rows = append(res.Rows, out)
	}

	if a.DryRun {
		// One transaction, rolled back at the end: row n sees rows 1…n-1, so
		// a duplicate inside the file, or a link to a record the file creates
		// further up, is caught exactly as the real run would catch it.
		tc := c.fork()
		tc.Flags["rollback"] = true
		err := tc.Run(func(tc *Ctx) error {
			for _, row := range plan.sheet.Rows {
				if err := tc.Ctx.Err(); err != nil {
					return err
				}
				var id, status string
				err := tc.WithSavepoint(func() error {
					var e error
					id, status, e = tc.importRow(plan, row)
					return e
				})
				record(tc, row, id, status, err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	for _, row := range plan.sheet.Rows {
		if err := c.Ctx.Err(); err != nil {
			// the caller went away: what committed stays, and the audit
			// below still says how far it got
			break
		}
		var id, status string
		tc := c.fork()
		err := tc.Run(func(tc *Ctx) error {
			var e error
			id, status, e = tc.importRow(plan, row)
			return e
		})
		record(tc, row, id, status, err)
	}
	// Written after the rows, on its own transaction: they committed one by
	// one, so there is no single transaction for the event to share.
	detail := map[string]any{
		"file": plan.fileName, "sha256": plan.sha, "format": plan.sheet.Format, "mode": plan.mode,
		"rows": res.Counts.Rows, "inserted": res.Counts.Inserted, "updated": res.Counts.Updated, "errors": res.Counts.Errors,
	}
	if err := c.fork().Run(func(tc *Ctx) error {
		return tc.Audit("data.import", plan.d.Name, "", detail)
	}); err != nil {
		c.E.Log.Warn("could not record a data import", "doctype", plan.d.Name, "err", err)
	}
	return res, nil
}

// dataImportRefused are DocTypes no spreadsheet writes to, whatever the
// roles say: the same records `ddcore import` leaves out by default, for the
// same reasons.
func dataImportRefused(name string) bool {
	_, ok := defaultExcluded[name]
	return ok
}

// CanDataImport is the preflight on its own: may this user import into this
// DocType in this mode? The template download asks it too.
func (c *Ctx) CanDataImport(doctype, mode string) (*meta.DocType, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	if d.IsChild {
		return nil, cerr.Validation("{0} is a child table: import its parent document instead", c.T(d.Label))
	}
	if d.IsSingle {
		return nil, cerr.Validation("{0} is a single record and has no rows to import", c.T(d.Label))
	}
	if err := refuseVirtual(d); err != nil {
		return nil, err
	}
	if dataImportRefused(d.Name) {
		return nil, cerr.Validation("{0} cannot be imported from a spreadsheet", c.T(d.Label))
	}
	ptype := "create"
	switch mode {
	case "", "insert":
	case "update":
		ptype = "write"
	default:
		return nil, cerr.Validation("Unknown import mode: {0}", mode)
	}
	for _, p := range []string{"import", ptype} {
		ok, err := c.HasPermission(d.Name, p, nil)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, cerr.Permission("No permission ({0}) to import {1}", p, c.T(d.Label))
		}
	}
	return d, nil
}

func (c *Ctx) planDataImport(a DataImportArgs) (*dataImportPlan, error) {
	mode := a.Mode
	if mode == "" {
		mode = "insert"
	}
	d, err := c.CanDataImport(a.Doctype, mode)
	if err != nil {
		return nil, err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return nil, err
	}
	if err := refuseVirtual(d); err != nil {
		return nil, err
	}
	sheet, err := tabular.Read(a.File, tabular.Options{Sep: a.Sep, MaxRows: a.MaxRows})
	if err != nil {
		var tm *tabular.TooManyRowsError
		switch {
		case errors.As(err, &tm):
			return nil, cerr.Validation("The file has more than {0} rows, the limit here. Split it into smaller files.", tm.Max)
		case errors.Is(err, tabular.ErrEmpty):
			return nil, cerr.Validation("The file is empty: the first row must name the columns")
		case errors.Is(err, tabular.ErrLegacyXLS):
			return nil, cerr.Validation("Excel 97-2003 (.xls) files are not supported: save the sheet as .xlsx or CSV")
		}
		return nil, cerr.Validation("Could not read the file: {0}", err.Error())
	}
	sum := sha256.Sum256(a.File)
	p := &dataImportPlan{
		d: d, mode: mode, sheet: sheet, idCol: -1, modCol: -1,
		cols: map[int]*meta.Field{}, labels: map[string]string{},
		sha: hex.EncodeToString(sum[:]), fileName: a.FileName,
		conv: cellConv{decimal: a.Decimal, order: a.DateOrder, date1904: sheet.Date1904},
	}
	if p.conv.decimal != "," {
		p.conv.decimal = "."
	}
	switch p.conv.order {
	case "dmy", "mdy", "ymd":
	default:
		p.conv.order = "dmy"
	}
	c.resolveDataImportColumns(p, a.Columns)
	if mode == "update" && p.idCol < 0 {
		return nil, cerr.Validation("Updating needs an ID column naming the record each row changes")
	}
	if len(p.cols) == 0 && (mode == "update" || p.idCol < 0) {
		return nil, cerr.Validation("No column of this file matches a field of {0}", c.T(d.Label))
	}
	return p, nil
}

// idSettable reports whether an insert may take its id from the file: only
// when the DocType leaves the id to whoever creates the document.
func idSettable(d *meta.DocType) bool {
	g := d.IDGeneration
	return g.Series == "" && g.Field == "" && g.Format == ""
}

// importableReason is why field f cannot take a value from a column, or "" if
// it can.
func (c *Ctx) importableReason(d *meta.DocType, f *meta.Field, access FieldAccess) string {
	switch {
	case meta.IsTableType(f.Fieldtype):
		return c.T("Child tables are not imported from a spreadsheet")
	case f.Fieldtype == "Password" || f.Fieldtype == "Vault":
		return c.T("Secret fields are not imported")
	case f.Fieldtype == "Attach" || f.Fieldtype == "Attach Image":
		return c.T("Files are attached on the form")
	case !meta.HasColumnField(f):
		return c.T("This field holds no value")
	case f.ReadOnly || f.FetchFrom != "":
		return c.T("This field is read-only")
	case !access.CanWrite(f):
		return c.T("You cannot change this field")
	}
	if wf := c.WorkflowFor(d.Name); wf != nil && wf.StateField == f.Fieldname {
		return c.T("Set by the workflow")
	}
	return ""
}

// importableFields are the fields a column may fill, in form order.
func (c *Ctx) importableFields(d *meta.DocType) []*meta.Field {
	access := c.FieldAccess(d)
	var out []*meta.Field
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		if c.importableReason(d, f, access) == "" {
			out = append(out, f)
		}
	}
	return out
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// resolveDataImportColumns matches each header to a field: an explicit
// override first, then the id, then a fieldname, the English label, and the
// label in the request language. A column that matches nothing is ignored
// rather than refused, so a file carrying an extra "Notes" or "Error" column
// still loads.
func (c *Ctx) resolveDataImportColumns(p *dataImportPlan, overrides map[string]string) {
	d := p.d
	access := c.FieldAccess(d)
	for _, f := range c.importableFields(d) {
		p.labels[f.Fieldname] = c.T(f.Label)
		p.fields = append(p.fields, DataImportField{Fieldname: f.Fieldname, Label: c.T(f.Label), Reqd: f.Reqd})
	}
	idNames := map[string]bool{"id": true, norm(c.T("ID")): true}
	if d.IDLabel != "" {
		idNames[norm(d.IDLabel)] = true
		idNames[norm(c.T(d.IDLabel))] = true
	}
	taken := map[string]bool{}
	for i, h := range p.sheet.Headers {
		col := DataImportColumn{Index: i, Header: h}
		key := norm(h)
		name, overridden := overrides[h]
		var f *meta.Field
		switch {
		case overridden && name == "":
			col.Status, col.Reason = "ignored", c.T("Ignored")
		case overridden && name == "id", !overridden && idNames[key]:
			c.mapIDColumn(p, &col)
		case overridden:
			if f = d.Field(name); f == nil {
				col.Status, col.Reason = "unknown", c.T("No field matches this column")
			}
		case key == "":
			col.Status, col.Reason = "ignored", c.T("The column has no header")
		case key == "modified":
			if p.mode == "update" {
				p.modCol = i
				col.Status, col.Fieldname, col.Reason = "mapped", "modified", c.T("Refuses a row changed since this file was exported")
			} else {
				col.Status, col.Reason = "ignored", c.T("Set by the system")
			}
		case d.IsStdColumn(key):
			col.Status, col.Reason = "ignored", c.T("Set by the system")
		default:
			var ambiguous bool
			if f, ambiguous = c.matchHeader(d, h); ambiguous {
				col.Status, col.Reason = "ignored", c.T("Several fields have this label: use the field name as the header")
			} else if f == nil {
				col.Status, col.Reason = "unknown", c.T("No field matches this column")
			}
		}
		if f != nil {
			col.Fieldname, col.Label = f.Fieldname, c.T(f.Label)
			switch reason := c.importableReason(d, f, access); {
			case reason != "":
				col.Status, col.Reason = "ignored", reason
			case taken[f.Fieldname]:
				col.Status, col.Reason = "ignored", c.T("Another column already fills this field")
			default:
				taken[f.Fieldname] = true
				p.cols[i] = f
				col.Status = "mapped"
			}
		}
		p.columns = append(p.columns, col)
	}
}

// matchHeader finds the field a header names: its fieldname, else its
// English label, else its label in the request language. A label two fields
// share names neither of them.
func (c *Ctx) matchHeader(d *meta.DocType, header string) (f *meta.Field, ambiguous bool) {
	key := norm(header)
	if f := d.Field(key); f != nil && !meta.LayoutTypes[f.Fieldtype] {
		return f, false
	}
	for _, translated := range []bool{false, true} {
		var found []*meta.Field
		for _, f := range d.Fields {
			if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
				continue
			}
			l := f.Label
			if translated {
				l = c.T(f.Label)
			}
			if norm(l) == key {
				found = append(found, f)
			}
		}
		if len(found) > 1 {
			return nil, true
		}
		if len(found) == 1 {
			return found[0], false
		}
	}
	return nil, false
}

func (c *Ctx) mapIDColumn(p *dataImportPlan, col *DataImportColumn) {
	col.Fieldname, col.Label = "id", c.T("ID")
	switch {
	case p.idCol >= 0:
		col.Status, col.Reason = "ignored", c.T("Another column already fills this field")
	case p.mode == "insert" && !idSettable(p.d):
		col.Status, col.Reason = "ignored", c.T("New records take their ID from the DocType's naming rule")
	default:
		p.idCol = col.Index
		col.Status = "mapped"
	}
}

// importRow writes one row as the caller, through Insert or Save.
func (c *Ctx) importRow(p *dataImportPlan, row tabular.Row) (id, status string, err error) {
	values := Doc{}
	for i, f := range p.cols {
		v, err := parseCell(f, p.labels[f.Fieldname], row.Cell(i), p.conv, c.T)
		if err != nil {
			return "", "", err
		}
		if f.Fieldtype == "Link" && v != nil {
			if v, err = c.resolveLink(f, p.labels[f.Fieldname], v.(string)); err != nil {
				return "", "", err
			}
		}
		values[f.Fieldname] = v
	}
	rowID := ""
	if p.idCol >= 0 {
		rowID = strings.TrimSpace(row.Cell(p.idCol).Text)
	}
	if p.mode == "update" {
		if rowID == "" {
			return "", "", cerr.Mandatory("The ID is empty: an update has to name the record it changes")
		}
		doc, err := c.GetDoc(p.d.Name, rowID)
		if err != nil {
			return "", "", err
		}
		// the sheet is the truth for the columns it carries: a blank cell
		// clears the field, and a column it does not carry is left alone
		for k, v := range values {
			doc[k] = v
		}
		if p.modCol >= 0 {
			if m := strings.TrimSpace(row.Cell(p.modCol).Text); m != "" {
				doc["modified"] = m
			}
		}
		saved, err := c.Save(doc, SaveOpts{})
		if err != nil {
			return rowID, "", err
		}
		return saved.ID(), "updated", nil
	}
	// a blank cell on a new record keeps the field's default
	for k, v := range values {
		if v == nil {
			delete(values, k)
		}
	}
	if rowID != "" {
		values["id"] = rowID
	}
	doc, err := c.NewDoc(p.d.Name, values)
	if err != nil {
		return "", "", err
	}
	saved, err := c.Insert(doc, SaveOpts{})
	if err != nil {
		return "", "", err
	}
	return saved.ID(), "inserted", nil
}

// resolveLink accepts a Link cell holding either the target's id or its
// title: people write "Acme Ltd", not "CUST-00042". A title matching two
// records is refused rather than guessed. The lookup is an ordinary list
// query, so it only finds what the user may read.
func (c *Ctx) resolveLink(f *meta.Field, label, v string) (any, error) {
	target := f.OptionsString()
	if ok, err := c.idExists(target, v); err != nil || ok {
		return v, err
	}
	td, err := c.St.DocType(target)
	if err != nil || td.TitleField == "" || td.TitleField == "id" {
		return v, nil // the save reports the missing link in its own words
	}
	// a Link to a DocType the user may not list is still a valid Link — the
	// save checks it exists without the user's permissions — so only the
	// lookup by title is out of reach, and the value goes on as an id
	if ok, err := c.HasPermission(target, "read", nil); err != nil || !ok {
		return v, err
	}
	rows, err := c.GetList(target, ListArgs{Filters: map[string]any{td.TitleField: v}, Fields: []string{"id"}, Limit: 2})
	if err != nil {
		return nil, err
	}
	switch len(rows) {
	case 1:
		return rows[0]["id"], nil
	case 0:
		return v, nil
	}
	return nil, cerr.Validation("{0}: more than one {1} is called \"{2}\"; use its ID", label, c.T(td.Label), v)
}

// ---------------------------------------------------------------- cells

// cellConv is how the person's spreadsheet writes numbers and dates.
type cellConv struct {
	decimal  string // "." or ","
	order    string // "dmy" | "mdy" | "ymd"
	date1904 bool
}

var (
	isoDateRe = regexp.MustCompile(`^\d{4}-\d{1,2}-\d{1,2}([T ]|$)`)
	dateRe    = regexp.MustCompile(`^(\d{1,4})[/.\-](\d{1,2})[/.\-](\d{1,4})$`)
	clockRe   = regexp.MustCompile(`^(\d{1,2}):(\d{2})(?::(\d{2}))?$`)
)

// parseCell turns one cell into the value field f takes, strictly. The
// engine's own coercion is forgiving on purpose — an API caller sends typed
// JSON — and would read "abc" as 0 and "maybe" as false; a spreadsheet cell
// is text a person typed, and a typo has to come back as an error on its row.
// A blank cell is nil. What is not listed here (text, a Select, a Link) is
// passed on as text for the save to validate.
func parseCell(f *meta.Field, label string, cell tabular.Cell, conv cellConv, T func(string, ...any) string) (any, error) {
	text := strings.TrimSpace(cell.Text)
	if text == "" && cell.Num == nil && cell.Bool == nil {
		return nil, nil // a Check saves nil as false
	}
	switch f.Fieldtype {
	case "Int", "Float", "Currency", "Percent", "Duration", "Rating":
		n, ok := cellNumber(cell, text, conv.decimal, f.Fieldtype == "Percent")
		if !ok {
			return nil, cerr.Validation("{0}: \"{1}\" is not a number", label, text)
		}
		if f.Fieldtype != "Float" && f.Fieldtype != "Currency" && f.Fieldtype != "Percent" {
			if n != math.Trunc(n) {
				return nil, cerr.Validation("{0}: \"{1}\" is not a whole number", label, text)
			}
			return int64(n), nil
		}
		return n, nil
	case "Check":
		if cell.Bool != nil {
			return *cell.Bool, nil
		}
		if cell.Num != nil && (*cell.Num == 0 || *cell.Num == 1) {
			return *cell.Num == 1, nil
		}
		switch strings.ToLower(text) {
		case "1", "true", "yes", "y", "x", strings.ToLower(T("Yes")):
			return true, nil
		case "0", "false", "no", "n", strings.ToLower(T("No")):
			return false, nil
		}
		return nil, cerr.Validation("{0}: \"{1}\" is not yes or no", label, text)
	case "Date":
		if cell.Num != nil {
			return tabular.ExcelSerialToTime(*cell.Num, conv.date1904).Format("2006-01-02"), nil
		}
		if s, ok := parseDate(text, conv.order); ok {
			return s, nil
		}
		return nil, cerr.Validation("{0}: \"{1}\" is not a date", label, text)
	case "Datetime":
		if cell.Num != nil {
			return tabular.ExcelSerialToTime(*cell.Num, conv.date1904).Format("2006-01-02 15:04:05"), nil
		}
		if isoDateRe.MatchString(text) && len(text) > 10 {
			return text, nil // ISO with a time: the save parses every ISO form
		}
		datePart, clock, _ := strings.Cut(text, " ")
		day, ok := parseDate(datePart, conv.order)
		if ok && clock == "" {
			return day + " 00:00:00", nil
		}
		if hms, okc := parseClock(strings.TrimSpace(clock)); ok && okc {
			return day + " " + hms, nil
		}
		return nil, cerr.Validation("{0}: \"{1}\" is not a date and time", label, text)
	case "Time":
		if cell.Num != nil {
			secs := int64(math.Round(math.Mod(*cell.Num, 1) * 86400))
			return fmt.Sprintf("%02d:%02d:%02d", secs/3600%24, secs/60%60, secs%60), nil
		}
		return text, nil
	case "Select":
		return selectValue(f, text, T), nil
	}
	if cell.Num != nil && text == "" {
		return strconv.FormatFloat(*cell.Num, 'f', -1, 64), nil
	}
	// a code like "007" or an id stays exactly as typed; only the ends are trimmed
	return text, nil
}

// cellNumber reads a number the way the person's locale writes it: with the
// decimal separator they use, and the other one taken as a thousands
// separator. A workbook's numeric cell needs none of that.
func cellNumber(cell tabular.Cell, text, decimal string, percent bool) (float64, bool) {
	if cell.Num != nil {
		return *cell.Num, true
	}
	s := strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "").Replace(text)
	if percent {
		s = strings.TrimSuffix(s, "%")
	}
	if decimal == "," {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
	} else {
		s = strings.ReplaceAll(s, ",", "")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// parseDate reads ISO always, and otherwise three numbers in the given order.
// A four-digit first number is a year whatever the order says: nobody writes
// 2024/31/12.
func parseDate(s, order string) (string, bool) {
	s = strings.TrimSpace(s)
	if isoDateRe.MatchString(s) {
		s, _, _ = strings.Cut(strings.Replace(s, "T", " ", 1), " ")
	}
	m := dateRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	var y, mo, d int
	switch {
	case len(m[1]) == 4:
		y, mo, d = a, b, c
	case order == "mdy":
		mo, d, y = a, b, c
	case order == "ymd":
		y, mo, d = a, b, c
	default:
		d, mo, y = a, b, c
	}
	if len(strconv.Itoa(y)) <= 2 && y < 100 {
		// a two-digit year, read the way spreadsheets read it
		if y < 30 {
			y += 2000
		} else {
			y += 1900
		}
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != mo || t.Day() != d {
		return "", false // 31/02 does not roll over into March
	}
	return t.Format("2006-01-02"), true
}

func parseClock(s string) (string, bool) {
	m := clockRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	sec := 0
	if m[3] != "" {
		sec, _ = strconv.Atoi(m[3])
	}
	if h > 23 || mi > 59 || sec > 59 {
		return "", false
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, mi, sec), true
}

// selectValue maps what the person typed back to the stored option: the
// canonical value, or the label they see in their language, in any case.
// Anything else is passed on for the save to refuse in its own words.
func selectValue(f *meta.Field, text string, T func(string, ...any) string) string {
	opts := f.SelectValues()
	for _, o := range opts {
		if o == text {
			return o
		}
	}
	for i, o := range opts {
		label := T(o)
		if i < len(f.OptionLabels) {
			label = f.OptionLabels[i]
		}
		if strings.EqualFold(o, text) || strings.EqualFold(label, text) {
			return o
		}
	}
	return text
}

// ---------------------------------------------------------------- template

// DataImportTemplate is the header row of a file that imports new records:
// every field a column may fill, headed by its label in the request language —
// or by its fieldname where two fields share a label, which would otherwise
// be ambiguous on the way back in.
func (c *Ctx) DataImportTemplate(doctype string) ([]string, error) {
	d, err := c.CanDataImport(doctype, "insert")
	if err != nil {
		return nil, err
	}
	var out []string
	if idSettable(d) {
		out = append(out, "id")
	}
	for _, f := range c.importableFields(d) {
		label := c.T(f.Label)
		if m, _ := c.matchHeader(d, label); label == "" || m != f || norm(label) == "id" || d.IsStdColumn(norm(label)) {
			label = f.Fieldname
		}
		out = append(out, label)
	}
	return out, nil
}
