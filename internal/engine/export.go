package engine

// Exporting a whole DocType, not a screenful of it (DAT-02).
//
// The desk's CSV has always been "what is on screen"; reconciling a migration
// needs the opposite — every row a filter matches, its child tables, and the
// files hanging off it, with the same authorisation a normal read would apply.
//
// Three decisions shape everything here:
//
//   - The walk goes through Ctx.GetList, one keyset page at a time. Reusing
//     the ordinary list query is what makes `ifOwner` and an app's
//     `permissionQuery` apply to an export for free; re-deriving them here
//     would be a second place to get authorisation wrong.
//   - Paging is `name > last`, never OFFSET. An OFFSET walk over a table that
//     is being written skips and repeats rows, which is precisely what a
//     reconcilable export cannot do.
//   - The caller owns the bytes. Export pushes documents into an ExportSink;
//     whether they become NDJSON on a socket or CSV files in a directory is
//     none of the engine's business, and memory stays flat either way.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// DefaultExportBatch is how many documents one page of the walk carries.
const DefaultExportBatch = 500

// redactedColumns never leave the instance, whatever the caller asks for.
// Every `Password` field is dropped by fieldtype; these two are the core's
// own credential columns, declared as Data because they hold a hash rather
// than the secret itself. A hash still identifies the credential, so an
// export — which is a file that travels — does not carry them.
var redactedColumns = map[string]map[string]bool{
	"User":    {"password_hash": true},
	"API Key": {"secret_hash": true},
}

// ExportArgs describes one export. Filters and OrFilters take the shapes
// ListArgs takes, so a desk list and its export agree on what "the filtered
// set" means.
type ExportArgs struct {
	Doctype     string
	Filters     any
	OrFilters   any
	Fields      []string // empty = every exportable column of the doctype
	Children    bool
	Attachments bool
	Batch       int // documents per page; 0 = DefaultExportBatch
	Limit       int // 0 = the whole set
}

// ExportFile is one attachment in the manifest. The bytes are not here: a
// sink that wants them reads FileURL through Ctx.OpenAttachment.
type ExportFile struct {
	ID          string `json:"id"` // the File document
	AttachedTo  string `json:"attachedTo"`
	Field       string `json:"field,omitempty"`
	FileName    string `json:"fileName"`
	FileURL     string `json:"fileUrl"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType,omitempty"`
	Private     bool   `json:"private"`
	SHA256      string `json:"sha256,omitempty"`
	Missing     bool   `json:"missing,omitempty"` // the row exists, the bytes do not
}

// ExportSummary is the manifest: what was asked, what came out, and when.
// It is what a reconciliation compares against the source system.
type ExportSummary struct {
	Doctype   string           `json:"doctype"`
	User      string           `json:"user"`
	Filters   any              `json:"filters,omitempty"`
	OrFilters any              `json:"orFilters,omitempty"`
	Columns   []string         `json:"columns"`
	Rows      int64            `json:"rows"`
	ChildRows map[string]int64 `json:"childRows,omitempty"`
	Files     int64            `json:"files,omitempty"`
	FileBytes int64            `json:"fileBytes,omitempty"`
	Started   time.Time        `json:"started"`
	Finished  time.Time        `json:"finished"`
	// Truncated says the walk stopped at Limit with rows still matching, so
	// the output is a sample and must not be reconciled as a whole.
	Truncated bool `json:"truncated,omitempty"`
}

// ExportSink receives the walk. Begin runs once before any document, End once
// after the last one, and neither is called when the export fails to start.
type ExportSink interface {
	Begin(d *meta.DocType, columns []string) error
	Doc(doc Doc, files []ExportFile) error
	End(s *ExportSummary) error
}

// SnapshotIsolation puts the current transaction in REPEATABLE READ so every
// page of the walk sees one instant of the database. It must be the first
// statement of the transaction — Postgres refuses the change once a query has
// run — so call it immediately after Ctx.Run opens, before touching roles,
// metadata or anything else.
func (c *Ctx) SnapshotIsolation() error {
	if c.Tx == nil {
		return nil
	}
	_, err := c.Tx.Exec(c.Ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ")
	return err
}

// ExportColumns lists the columns an export of d carries, in a stable order:
// name, the declared fields as the app declared them, then the audit columns.
// Layout fields have no column and secrets are dropped.
func ExportColumns(d *meta.DocType) []string {
	out := []string{"id"}
	for _, f := range d.DataFields() {
		if f.Fieldtype == "Password" || redactedColumns[d.Name][f.Fieldname] {
			continue
		}
		out = append(out, f.Fieldname)
	}
	if d.IsChild {
		out = append(out, meta.ChildColumns...)
	}
	for _, c := range meta.StdColumns {
		if c != "id" {
			out = append(out, c)
		}
	}
	return out
}

// exportableColumn reports whether the caller may ask for this column by name.
func exportableColumn(d *meta.DocType, name string) bool {
	if !d.HasColumn(name) {
		return false
	}
	if redactedColumns[d.Name][name] {
		return false
	}
	f := d.Field(name)
	return f == nil || f.Fieldtype != "Password"
}

// exportPlan settles everything that can refuse an export: the doctype, the
// permissions, the columns and the filters. It is separate from the walk so a
// caller that has already begun a response — the HTTP handler — can get the
// refusal while a status code is still changeable, without keeping a second
// copy of the rules.
func (c *Ctx) exportPlan(a ExportArgs) (*meta.DocType, []string, []db.Filter, error) {
	d, err := c.St.DocType(a.Doctype)
	if err != nil {
		return nil, nil, nil, err
	}
	if d.IsChild {
		return nil, nil, nil, cerr.Validation("{0} is a child table: export the DocType that embeds it.", c.T(d.Label))
	}
	if d.IsSingle {
		return nil, nil, nil, cerr.Validation("Export of Single DocTypes is not supported: {0}.", c.T(d.Label))
	}
	// read says which rows; export says the rows may leave as a file. Both.
	for _, ptype := range []string{"read", "export"} {
		ok, err := c.HasPermission(d.Name, ptype, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		if !ok {
			return nil, nil, nil, cerr.Permission("No permission to export {0}", c.T(d.Label))
		}
	}
	access := c.FieldAccess(d)
	columns := a.Fields
	if len(columns) == 0 {
		for _, col := range ExportColumns(d) {
			if access.CanRead(d.Field(col)) {
				columns = append(columns, col)
			}
		}
	} else {
		for _, f := range columns {
			if !exportableColumn(d, f) {
				return nil, nil, nil, cerr.Validation("Unknown field: {0}", f)
			}
			if !access.CanRead(d.Field(f)) {
				return nil, nil, nil, cerr.Permission("No permission to read field {0} of {1}", f, c.T(d.Label))
			}
		}
	}
	base, err := db.ParseFilters(a.Filters)
	if err != nil {
		return nil, nil, nil, cerr.Validation("Invalid filters: {0}", err)
	}
	return d, columns, base, nil
}

// CanExport reports whether this export would be allowed to start, raising the
// same error Export would. It is the preflight for a streaming caller.
func (c *Ctx) CanExport(a ExportArgs) error {
	_, _, _, err := c.exportPlan(a)
	return err
}

// Export walks every document matching the filters and hands it to sink.
func (c *Ctx) Export(a ExportArgs, sink ExportSink) (*ExportSummary, error) {
	d, columns, base, err := c.exportPlan(a)
	if err != nil {
		return nil, err
	}

	sum := &ExportSummary{
		Doctype: d.Name, User: c.User, Filters: a.Filters, OrFilters: a.OrFilters,
		Columns: columns, ChildRows: map[string]int64{}, Started: c.Now(),
	}
	if err := sink.Begin(d, columns); err != nil {
		return nil, err
	}

	batch := a.Batch
	if batch <= 0 {
		batch = DefaultExportBatch
	}
	last := ""
	for {
		page := batch
		if a.Limit > 0 {
			left := int64(a.Limit) - sum.Rows
			if left <= 0 {
				// The limit is spent. Whether that truncated anything is a
				// question worth one more query: the flag's only job is to
				// stop someone reconciling a sample as if it were the whole
				// set, and a flag raised over a complete export would teach
				// them to ignore it.
				more, err := c.GetList(d.Name, ListArgs{
					Filters: keysetFilters(base, last), OrFilters: a.OrFilters,
					Fields: []string{"id"}, OrderBy: "id asc", Limit: 1,
				})
				if err != nil {
					return nil, err
				}
				sum.Truncated = len(more) > 0
				break
			}
			if left < int64(page) {
				page = int(left)
			}
		}
		rows, err := c.GetList(d.Name, ListArgs{
			Filters:   keysetFilters(base, last),
			OrFilters: a.OrFilters,
			Fields:    columns,
			OrderBy:   "id asc",
			Limit:     page,
		})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		names := make([]string, len(rows))
		for i, r := range rows {
			names[i] = db.Str(r["id"])
		}
		var children map[string]map[string][]any
		if a.Children {
			if children, err = c.exportChildren(d, names, sum); err != nil {
				return nil, err
			}
		}
		var files map[string][]ExportFile
		if a.Attachments {
			if files, err = c.exportFiles(d.Name, names, sum); err != nil {
				return nil, err
			}
		}
		for i, r := range rows {
			doc := Doc(r)
			doc["doctype"] = d.Name
			for field, list := range children[names[i]] {
				doc[field] = list
			}
			if err := sink.Doc(doc, files[names[i]]); err != nil {
				return nil, err
			}
			sum.Rows++
		}
		last = names[len(names)-1]
		if len(rows) < page {
			break
		}
	}
	sum.Finished = c.Now()
	if err := sink.End(sum); err != nil {
		return nil, err
	}
	return sum, nil
}

// keysetFilters rebuilds the caller's filters as tuples and adds the keyset
// condition. It goes into Filters, which are ANDed — putting it in OrFilters
// would widen the page instead of advancing it.
func keysetFilters(base []db.Filter, last string) any {
	out := make([]any, 0, len(base)+1)
	for _, f := range base {
		out = append(out, []any{f.Field, f.Op, f.Value})
	}
	if last != "" {
		out = append(out, []any{"id", ">", last})
	}
	return out
}

// exportChildren loads the child rows of a whole page in one query per table
// field. Loading them document by document (as GetDoc does) would be
// len(page) × len(tables) round trips for no gain.
//
// The rows are not re-authorised: a child follows its parent's read
// permission, and the parents in `names` came back from a query that had the
// permission filters applied.
func (c *Ctx) exportChildren(d *meta.DocType, names []string, sum *ExportSummary) (map[string]map[string][]any, error) {
	out := map[string]map[string][]any{}
	access := c.FieldAccess(d)
	for _, tf := range d.TableFields() {
		if !access.CanRead(tf) {
			continue
		}
		child, err := c.St.DocType(tf.OptionsString())
		if err != nil {
			return nil, err
		}
		var sel []string
		for _, col := range ExportColumns(child) {
			if access.CanRead(child.Field(col)) {
				sel = append(sel, db.Ident(col))
			}
		}
		sql := fmt.Sprintf(
			"SELECT %s FROM %s WHERE parenttype = $1 AND parentfield = $2 AND parent = ANY($3) ORDER BY parent, idx",
			strings.Join(sel, ", "), db.Ident(child.TableName()))
		rows, err := db.Select(c.Ctx, c.Q(), sql, d.Name, tf.Fieldname, names)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			parent := db.Str(r["parent"])
			r["doctype"] = child.Name
			if out[parent] == nil {
				out[parent] = map[string][]any{}
			}
			out[parent][tf.Fieldname] = append(out[parent][tf.Fieldname], Doc(r))
			sum.ChildRows[child.Name]++
		}
		// A document with no rows in this table still declares the field, so
		// the shape of every exported document is the same.
		for _, n := range names {
			if out[n] == nil {
				out[n] = map[string][]any{}
			}
			if out[n][tf.Fieldname] == nil {
				out[n][tf.Fieldname] = []any{}
			}
		}
	}
	return out, nil
}

// exportFiles lists the attachments of a page and checksums their bytes.
//
// File is readable by its owner only, so a plain list would hide a colleague's
// attachment on a document the user can read. That is the same question
// /private/files answers, and it answers it by the attached document's read
// permission — which these names already passed. So the lookup ignores File's
// own permissions and stays scoped to this page.
func (c *Ctx) exportFiles(doctype string, names []string, sum *ExportSummary) (map[string][]ExportFile, error) {
	var rows []map[string]any
	err := c.WithIgnorePermissions(func() error {
		var e error
		rows, e = c.GetList("File", ListArgs{
			Filters: []any{
				[]any{"attached_to_doctype", "=", doctype},
				[]any{"attached_to_id", "in", toAnySlice(names)},
			},
			Fields:  []string{"id", "file_name", "file_url", "file_size", "content_type", "is_private", "attached_to_id", "attached_to_field"},
			OrderBy: "attached_to_id asc, creation asc",
		})
		return e
	})
	if err != nil {
		return nil, err
	}
	out := map[string][]ExportFile{}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	access := c.FieldAccess(d)
	for _, r := range rows {
		// an attachment held by a field the user cannot read is that field's value
		if field := db.Str(r["attached_to_field"]); field != "" && !access.CanRead(d.Field(field)) {
			continue
		}
		f := ExportFile{
			ID:          db.Str(r["id"]),
			AttachedTo:  db.Str(r["attached_to_id"]),
			Field:       db.Str(r["attached_to_field"]),
			FileName:    db.Str(r["file_name"]),
			FileURL:     db.Str(r["file_url"]),
			Size:        int64(toFloat(r["file_size"])),
			ContentType: db.Str(r["content_type"]),
			Private:     r["is_private"] == true,
		}
		sha, size, err := c.checksum(f.FileURL)
		if err != nil {
			// A row whose bytes are gone is a finding for the reconciliation,
			// not a reason to abort the export: say so and carry on.
			f.Missing = true
		} else {
			f.SHA256, f.Size = sha, size
		}
		out[f.AttachedTo] = append(out[f.AttachedTo], f)
		sum.Files++
		sum.FileBytes += f.Size
	}
	return out, nil
}

func toAnySlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// OpenAttachment opens the bytes behind a File's url, wherever the site keeps
// them. A url with nothing stored under it is storage.ErrNotFound.
func (c *Ctx) OpenAttachment(fileURL string) (io.ReadCloser, error) {
	key, ok := storage.KeyFromURL(fileURL)
	if !ok {
		return nil, storage.ErrNotFound
	}
	rc, _, err := c.E.Storage().Open(c.Ctx, key)
	return rc, err
}

// ReadAttachment reads the whole of a File's bytes.
func (c *Ctx) ReadAttachment(fileURL string) ([]byte, error) {
	rc, err := c.OpenAttachment(fileURL)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (c *Ctx) checksum(fileURL string) (string, int64, error) {
	rc, err := c.OpenAttachment(fileURL)
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()
	h := sha256.New()
	n, err := io.Copy(h, rc)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// ExportLayout is the current arrangement of an export directory; see
// ExportManifest.Layout.
const ExportLayout = 2

// ExportManifest is what `ddcore export` leaves in manifest.json: what was
// asked, what came out, and the checksum of every file written, so a second run
// can be compared with this one — and so an import can be reconciled against
// what was actually taken. The importer reads this type, so it lives here
// rather than in the command that writes it.
type ExportManifest struct {
	DDCore string `json:"ddcore"`
	// Layout is how the directory is arranged. 1 wrote the attachment bytes to
	// files/<base name>; 2 writes them to files/<storage key>. An importer
	// reads it to know which one it is looking at.
	Layout   int               `json:"exportFormat"`
	Apps     map[string]string `json:"apps,omitempty"` // app name → declared version
	Site     string            `json:"site"`
	User     string            `json:"user"`
	Dir      string            `json:"dir"`
	Format   string            `json:"format"`
	Filters  any               `json:"filters,omitempty"`
	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"`
	Skipped  []string          `json:"skipped,omitempty"`
	Exports  []*ExportResult   `json:"exports"`
}

// ExportResult is one DocType's part of the manifest.
type ExportResult struct {
	Summary     *ExportSummary `json:"summary"`
	Outputs     []ExportOutput `json:"outputs"`
	Attachments []ExportFile   `json:"attachments,omitempty"`
}

// ExportOutput is one written file, with the checksum of what was produced
// rather than of what was meant to be.
type ExportOutput struct {
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
