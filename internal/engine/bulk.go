package engine

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Bulk writes (#145): ddcore.db.insertMany, updateMany and upsertMany write
// many rows of a plain DocType with one statement per chunk, inside the
// current transaction. They keep what makes a row valid — the casts, reqd,
// Select options, Links, the unique indexes, the tenant column and its policy,
// the id rule — and drop what makes a row a document: no hooks, no reload, no
// Version, no webhooks or notifications, no realtime event per row. A DocType
// whose writes carry more than that (a workflow, a tree, a Single, the core's
// own DocTypes) is refused rather than written behind its back.

const (
	// bulkMaxRows bounds one call: the rows cross the JS bridge as one JSON
	// value and are held in memory twice over. The caller pages.
	bulkMaxRows = 10000
	// bulkMaxParams keeps a statement under Postgres's 65,535 bind parameters.
	bulkMaxParams = 65000
	// bulkMaxChunk caps the rows of one statement.
	bulkMaxChunk = 1000
)

// BulkOpts are the options of InsertMany, UpdateMany and UpsertMany.
type BulkOpts struct {
	IgnorePermissions bool
	IgnoreLinks       bool
	// Key is what UpsertMany matches stored rows by: "id" or a unique field.
	Key string
}

// BulkChange is one row of UpdateMany: the document's id and the fields to set.
type BulkChange struct {
	ID     string `json:"id"`
	Values Doc    `json:"values"`
}

// UpsertResult counts what UpsertMany did with each row.
type UpsertResult struct {
	Inserted  int `json:"inserted"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

type bulkOp int

const (
	bulkInsert bulkOp = iota
	bulkUpdate
	bulkUpsert
)

// InsertMany inserts rows as new documents of doctype and returns how many
// were written. Each row is a document's values, as newDoc takes them: the
// defaults fill what it leaves out.
func (c *Ctx) InsertMany(doctype string, rows []Doc, opts BulkOpts) (int, error) {
	var n int
	err := c.bulk(doctype, bulkInsert, len(rows), opts, func(c *Ctx, d *meta.DocType) error {
		var err error
		n, err = c.insertMany(d, rows, opts)
		return err
	})
	return n, err
}

// UpdateMany sets values on existing documents and returns how many changed.
// A row whose stored values already equal the change is not written: its
// modified stays as it was.
func (c *Ctx) UpdateMany(doctype string, changes []BulkChange, opts BulkOpts) (int, error) {
	var n int
	err := c.bulk(doctype, bulkUpdate, len(changes), opts, func(c *Ctx, d *meta.DocType) error {
		var err error
		n, err = c.updateMany(d, changes, opts)
		return err
	})
	return n, err
}

// UpsertMany inserts the rows whose key no stored document holds and updates
// the others, leaving alone those whose stored values already equal the row's.
func (c *Ctx) UpsertMany(doctype string, rows []Doc, opts BulkOpts) (UpsertResult, error) {
	var res UpsertResult
	err := c.bulk(doctype, bulkUpsert, len(rows), opts, func(c *Ctx, d *meta.DocType) error {
		var err error
		res, err = c.upsertMany(d, rows, opts)
		return err
	})
	return res, err
}

// bulk runs fn after the refusals every bulk write shares, in the space the
// write belongs to.
func (c *Ctx) bulk(doctype string, op bulkOp, n int, opts BulkOpts, fn func(c *Ctx, d *meta.DocType) error) error {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return err
	}
	if err := c.writeGuard(d); err != nil {
		return err
	}
	if d.IsSingle || d.IsChild || d.IsTree || d.App == "core" || c.WorkflowFor(d.Name) != nil {
		return cerr.Validation("{0} cannot be written in bulk", c.T(d.Label))
	}
	if op != bulkInsert && d.TrackChanges {
		// a bulk write leaves no Version, and this DocType promises one
		return cerr.Validation("{0} tracks changes: write its documents one at a time", c.T(d.Label))
	}
	if op == bulkUpsert {
		if err := c.checkUpsertKey(d, opts.Key); err != nil {
			return err
		}
	}
	if n > bulkMaxRows {
		return cerr.Validation("At most {0} rows can be written in one call", bulkMaxRows)
	}
	if n == 0 {
		return nil
	}
	server := opts.IgnorePermissions || c.IgnorePermissions()
	// A tenant's server code adds to a DocType the platform keeps, and
	// refreshes it, in the platform space, as insert and save do; updateMany
	// is setValue's bulk form and stays refused there, as setValue is.
	if op != bulkUpdate && c.serverWriteInPlatform(d, server) {
		_, err := c.inPlatform(func(p *Ctx) (Doc, error) { return nil, p.bulkIn(d, op, opts, fn) })
		return err
	}
	return c.bulkIn(d, op, opts, fn)
}

func (c *Ctx) bulkIn(d *meta.DocType, op bulkOp, opts BulkOpts, fn func(c *Ctx, d *meta.DocType) error) error {
	server := opts.IgnorePermissions || c.IgnorePermissions()
	if err := c.spaceRefusal(d, true, server); err != nil {
		return err
	}
	if !server {
		if op != bulkUpdate {
			if ok, err := c.HasPermission(d.Name, "create", nil); err != nil {
				return err
			} else if !ok {
				return cerr.Permission("No permission to create {0}", c.T(d.Label))
			}
		}
		if op != bulkInsert {
			if ok, err := c.HasPermission(d.Name, "write", nil); err != nil {
				return err
			} else if !ok {
				return cerr.Permission("No permission to write {0}", c.T(d.Label))
			}
		}
	}
	// Access scopes are checked per document against the stored row, which a
	// bulk write never reads; a scoped user writes one document at a time.
	// Like Insert's scope check, this yields only to the ctx (a job, Admin).
	if perms, err := c.UserPermissions(); err != nil {
		return err
	} else if len(perms) > 0 {
		return cerr.Permission("Bulk writes are not available to a user with access scopes")
	}
	return fn(c, d)
}

func (c *Ctx) checkUpsertKey(d *meta.DocType, key string) error {
	if key == "id" {
		if countsIDs(d) {
			// the insert side of every row would take a number, and an
			// update would leave a gap in the series
			return cerr.Validation("{0} takes its ids from a counter and cannot be upserted", c.T(d.Label))
		}
		return nil
	}
	if f := d.Field(key); f != nil && f.Unique && meta.HasColumnField(f) {
		if countsIDs(d) {
			return cerr.Validation("{0} takes its ids from a counter and cannot be upserted", c.T(d.Label))
		}
		return nil
	}
	return cerr.Validation("The upsert key must be id or a unique field of {0}", c.T(d.Label))
}

// bulkFields checks the fields a bulk write was given, the union over its
// rows, and returns them by name; `id` is accepted only where a row names
// its document (an insert, an upsert) and is not among them.
func (c *Ctx) bulkFields(d *meta.DocType, op bulkOp, key string, names map[string]bool, opts BulkOpts) (map[string]*meta.Field, error) {
	out := make(map[string]*meta.Field, len(names))
	keys := slices.Sorted(maps.Keys(names))
	var access *FieldAccess
	if c.fieldPermissionsApply(d, SaveOpts{IgnorePermissions: opts.IgnorePermissions}) {
		a := c.FieldAccess(d)
		access = &a
	}
	for _, k := range keys {
		if k == "id" {
			if op == bulkUpdate {
				return nil, cerr.Validation("{0} is set by the framework", "id")
			}
			continue
		}
		f := d.Field(k)
		if f == nil {
			if d.IsStdColumn(k) {
				return nil, cerr.Validation("{0} is set by the framework", k)
			}
			return nil, cerr.Validation("Field {0} does not exist on {1}", k, d.Name)
		}
		switch {
		case meta.LayoutTypes[f.Fieldtype], f.Computed, !meta.HasColumnField(f),
			f.Fieldtype == "Attach", f.Fieldtype == "Attach Image", f.Fieldtype == "Vault", f.Fieldtype == "Password":
			return nil, cerr.Validation("{0}: a {1} field cannot be written in bulk", c.T(f.Label), f.Fieldtype)
		case f.SetOnlyOnce && (op == bulkUpdate || (op == bulkUpsert && k != key)):
			return nil, cerr.Validation("{0} can only be set once: write it with insert or save", c.T(f.Label))
		}
		if access != nil && !access.CanWrite(f) {
			return nil, c.fieldWriteDenied(f)
		}
		out[k] = f
	}
	return out, nil
}

// atRow marks err with the index of the row that caused it, as extra.row.
func atRow(err error, i int) error {
	var e *cerr.Error
	if !errors.As(err, &e) {
		return err
	}
	out := *e
	extra := map[string]any{}
	if m, ok := e.Extra.(map[string]any); ok {
		maps.Copy(extra, m)
	}
	extra["row"] = i
	out.Extra = extra
	return &out
}

// ------------------------------------------------------------ rows

// bulkNewRows turns rows into documents ready to insert: defaults, casts,
// fetched values, reqd, e-mail and Select checks, the standard columns and an
// id. The second result is each row's own fields, which an upsert updates.
func (c *Ctx) bulkNewRows(d *meta.DocType, op bulkOp, rows []Doc, opts BulkOpts, now time.Time) ([]Doc, []map[string]bool, error) {
	names := map[string]bool{}
	given := make([]map[string]bool, len(rows))
	for i, r := range rows {
		given[i] = make(map[string]bool, len(r))
		for k := range r {
			if !IsInternalKey(k) && k != "doctype" {
				names[k], given[i][k] = true, true
			}
		}
	}
	if _, err := c.bulkFields(d, op, opts.Key, names, opts); err != nil {
		return nil, nil, err
	}
	tmpl, err := c.NewDoc(d.Name, nil)
	if err != nil {
		return nil, nil, err
	}
	docs := make([]Doc, len(rows))
	for i, r := range rows {
		doc := maps.Clone(tmpl)
		for k := range given[i] {
			doc[k] = r[k]
		}
		if err := c.castAll(d, doc); err != nil {
			return nil, nil, atRow(err, i)
		}
		docs[i] = doc
	}
	if err := c.bulkFetch(d, docs, nil); err != nil {
		return nil, nil, err
	}
	for i, doc := range docs {
		for _, check := range []func() error{
			func() error { return c.castAll(d, doc) },
			func() error { return c.checkMandatory(d, doc, fieldChecks{}) },
			func() error { return c.checkEmails(d, doc) },
			func() error { return c.checkSelect(d, doc) },
		} {
			if err := check(); err != nil {
				return nil, nil, atRow(err, i)
			}
		}
		doc["owner"], doc["creation"], doc["modified"], doc["modified_by"], doc["docstatus"] = c.User, now, now, c.User, 0
	}
	if !opts.IgnoreLinks {
		if err := c.bulkLinks(d, docs, nil); err != nil {
			return nil, nil, err
		}
	}
	if err := c.bulkIDs(d, docs); err != nil {
		return nil, nil, err
	}
	if err := c.bulkDuplicates(d, docs); err != nil {
		return nil, nil, err
	}
	return docs, given, nil
}

// bulkIDs gives each document its id. A series or a counted format takes one
// block of numbers per prefix: a first pass counts the rows of each prefix
// with a counter that hands out nothing, the block is reserved in one
// statement, and a second pass numbers the rows out of it.
func (c *Ctx) bulkIDs(d *meta.DocType, docs []Doc) error {
	next := counterFunc(c.nextCounter)
	if countsIDs(d) {
		need := map[string]int64{}
		for i, doc := range docs {
			if err := c.assignID(d, doc, func(p string) (int64, error) { need[p]++; return 0, nil }); err != nil {
				return atRow(err, i)
			}
		}
		first, left := map[string]int64{}, map[string]int64{}
		for _, p := range slices.Sorted(maps.Keys(need)) {
			last, err := c.nextCounterN(p, need[p])
			if err != nil {
				return err
			}
			first[p], left[p] = last-need[p]+1, need[p]
		}
		next = func(p string) (int64, error) {
			if left[p] == 0 {
				// a prefix the first pass did not see: the date rolled over
				return c.nextCounter(p)
			}
			n := first[p]
			first[p]++
			left[p]--
			return n, nil
		}
	}
	for i, doc := range docs {
		if err := c.assignID(d, doc, next); err != nil {
			return atRow(err, i)
		}
	}
	return nil
}

// bulkDuplicates refuses two rows of one call holding the same id, the same
// value of a unique field or the same compound key — what the index would
// refuse, said with the row it is about.
func (c *Ctx) bulkDuplicates(d *meta.DocType, docs []Doc) error {
	seen := map[string]bool{}
	for i, doc := range docs {
		if k := "id\x00" + doc.ID(); seen[k] {
			return atRow(cerr.Duplicate("{0} {1} already exists", c.T(d.Label), doc.ID()).WithTitleKey("Duplicate ID"), i)
		} else {
			seen[k] = true
		}
		for _, f := range d.DataFields() {
			if !f.Unique || outsideKey(f, doc[f.Fieldname]) {
				continue
			}
			k := "f\x00" + f.Fieldname + "\x00" + db.Str(doc[f.Fieldname])
			if seen[k] {
				return atRow(cerr.Duplicate("{0} \"{1}\" already exists", c.T(f.Label), doc.Str(f.Fieldname)).WithTitleKey("Duplicate value"), i)
			}
			seen[k] = true
		}
	keys:
		for _, uk := range d.UniqueKeys {
			k := "k\x00" + uk.Name
			for _, fn := range uk.Fields {
				f := d.Field(fn)
				if f == nil || outsideKey(f, doc[fn]) {
					continue keys
				}
				k += "\x00" + db.Str(doc[fn])
			}
			if seen[k] {
				return atRow(cerr.Duplicate("{0} already exists", c.keyValues(d, uk, doc)).WithTitleKey("Duplicate value"), i)
			}
			seen[k] = true
		}
	}
	return nil
}

// bulkLinks checks the Link and Dynamic Link values of docs with one query
// per target DocType. written, when given, limits row i to its own fields.
func (c *Ctx) bulkLinks(d *meta.DocType, docs []Doc, written []map[string]bool) error {
	type ref struct {
		row        int
		f          *meta.Field
		target, id string
	}
	var refs []ref
	ids := map[string]map[string]bool{}
	for i, doc := range docs {
		for _, f := range d.Fields {
			if f.Fieldtype != "Link" && f.Fieldtype != "Dynamic Link" {
				continue
			}
			if written != nil && !written[i][f.Fieldname] {
				continue
			}
			v := doc.Str(f.Fieldname)
			if v == "" {
				continue
			}
			target := f.OptionsString()
			if f.Fieldtype == "Dynamic Link" {
				if target = doc.Str(f.OptionsString()); target == "" {
					return atRow(cerr.Validation("{0}: choose the type before the link", c.T(f.Label)), i)
				}
				if _, err := c.St.DocType(target); err != nil {
					return atRow(cerr.Validation("{0}: DocType \"{1}\" does not exist", c.T(f.Label), target), i)
				}
			}
			refs = append(refs, ref{i, f, target, v})
			if ids[target] == nil {
				ids[target] = map[string]bool{}
			}
			ids[target][v] = true
		}
	}
	found := map[string]map[string]bool{}
	for target, set := range ids {
		td, err := c.St.DocType(target)
		if err != nil {
			return err
		}
		found[target] = map[string]bool{}
		list := slices.Sorted(maps.Keys(set))
		if td.IsVirtual() {
			for _, id := range list {
				ok, err := c.linkExists(target, id)
				if err != nil {
					return err
				}
				found[target][id] = ok
			}
			continue
		}
		rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT id FROM %s WHERE id = ANY($1)", db.Ident(td.TableName())), list)
		if err != nil {
			return err
		}
		for _, r := range rows {
			found[target][db.Str(r["id"])] = true
		}
		for _, id := range list {
			if !found[target][id] {
				// a Link to User inside a tenant may name an operator
				ok, err := c.operatorLink(target, id)
				if err != nil {
					return err
				}
				found[target][id] = ok
			}
		}
	}
	for _, r := range refs {
		if !found[r.target][r.id] {
			return atRow(cerr.LinkExists("{0}: {1} \"{2}\" does not exist", c.T(r.f.Label), r.target, r.id).WithTitleKey("Invalid link"), r.row)
		}
	}
	return nil
}

// bulkFetch fills the fetchFrom fields of docs with one query per target
// DocType and column, as fetchFrom does one document at a time. written,
// when given, limits row i to the links it writes, and gains the fields
// fetched for it.
func (c *Ctx) bulkFetch(d *meta.DocType, docs []Doc, written []map[string]bool) error {
	type want struct {
		row    int
		f      *meta.Field
		target string
		col    string
		id     string
	}
	var wants []want
	for _, f := range d.Fields {
		if f.FetchFrom == "" || !meta.HasColumnField(f) {
			continue
		}
		parts := strings.SplitN(f.FetchFrom, ".", 2)
		lf := d.Field(parts[0])
		if lf == nil || len(parts) != 2 {
			continue
		}
		for i, doc := range docs {
			if written != nil && !written[i][parts[0]] {
				continue
			}
			linkVal := doc.Str(parts[0])
			if linkVal == "" {
				if f.ReadOnly {
					doc[f.Fieldname] = nil
					if written != nil {
						written[i][f.Fieldname] = true
					}
				}
				continue
			}
			target := lf.OptionsString()
			if lf.Fieldtype == "Dynamic Link" {
				target = doc.Str(lf.OptionsString())
			}
			if target == "" || (!f.ReadOnly && !isEmpty(doc[f.Fieldname])) {
				continue
			}
			wants = append(wants, want{i, f, target, parts[1], linkVal})
		}
	}
	type source struct{ target, col string }
	ids := map[source]map[string]bool{}
	for _, w := range wants {
		s := source{w.target, w.col}
		if ids[s] == nil {
			ids[s] = map[string]bool{}
		}
		ids[s][w.id] = true
	}
	values := map[source]map[string]any{}
	for s, set := range ids {
		td, err := c.St.DocType(s.target)
		if err != nil {
			return err
		}
		values[s] = map[string]any{}
		list := slices.Sorted(maps.Keys(set))
		if td.IsVirtual() || !td.HasColumn(s.col) || s.col == "id" {
			// what GetValue reads is not a plain column here: ask it per id
			for _, id := range list {
				v, err := c.GetValue(s.target, id, s.col)
				if err != nil {
					return err
				}
				values[s][id] = v
			}
			continue
		}
		rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT id, %s AS v FROM %s WHERE id = ANY($1)", db.Ident(s.col), db.Ident(td.TableName())), list)
		if err != nil {
			return err
		}
		for _, r := range rows {
			values[s][db.Str(r["id"])] = r["v"]
		}
	}
	for _, w := range wants {
		docs[w.row][w.f.Fieldname] = values[source{w.target, w.col}][w.id]
		if written != nil {
			written[w.row][w.f.Fieldname] = true
		}
	}
	return nil
}

// ------------------------------------------------------------ SQL

// chunkSize is how many rows of cols columns one statement takes.
func chunkSize(cols int) int {
	n := bulkMaxParams / max(cols, 1)
	return max(1, min(n, bulkMaxChunk))
}

// insertColumns are the columns a bulk insert writes, in order: the standard
// ones and every data field, as writeInsert writes them. The tenant column is
// left to its default, the transaction's tenant.
func insertColumns(d *meta.DocType) []string {
	cols := []string{"id", "owner", "creation", "modified", "modified_by", "docstatus"}
	for _, f := range d.DataFields() {
		cols = append(cols, f.Fieldname)
	}
	return cols
}

// insertValues renders the VALUES list of docs for cols.
func (c *Ctx) insertValues(d *meta.DocType, cols []string, docs []Doc, b *db.Builder) (string, error) {
	tuples := make([]string, len(docs))
	ph := make([]string, len(cols))
	for i, doc := range docs {
		for j, col := range cols {
			v := doc[col]
			switch col {
			case "id", "owner", "modified_by":
			case "creation", "modified":
				v = parseTimeOrNow(v, c.E.Location())
			case "docstatus":
				v = int64(doc.Docstatus())
			default:
				cv, err := c.castValue(d.Field(col), v)
				if err != nil {
					return "", atRow(err, i)
				}
				v = cv
			}
			ph[j] = b.Arg(v)
		}
		tuples[i] = "(" + strings.Join(ph, ", ") + ")"
	}
	return strings.Join(tuples, ", "), nil
}

func identList(cols []string, prefix string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = prefix + db.Ident(c)
	}
	return strings.Join(out, ", ")
}

// bulkErr turns a statement's unique violation into the DuplicateEntryError
// insert and save raise, naming the value Postgres reported.
func (c *Ctx) bulkErr(d *meta.DocType, err error) error {
	if _, ok := db.UniqueViolation(err); !ok {
		return err
	}
	doc := Doc{}
	for k, v := range db.UniqueViolationKey(err) {
		doc[k] = v
	}
	return c.duplicateErr(d, doc, err)
}

// listChanged tells the open lists of d, once the transaction commits, that
// its rows changed: one event for the whole call, not one per row.
func (c *Ctx) listChanged(d *meta.DocType) {
	c.AfterCommit(func() {
		c.publish(Event{Name: "list_update", Payload: map[string]any{"doctype": d.Name}})
	})
}

func (c *Ctx) insertMany(d *meta.DocType, rows []Doc, opts BulkOpts) (int, error) {
	docs, _, err := c.bulkNewRows(d, bulkInsert, rows, opts, time.Now())
	if err != nil {
		return 0, err
	}
	cols := insertColumns(d)
	size := chunkSize(len(cols))
	n := 0
	for start := 0; start < len(docs); start += size {
		chunk := docs[start:min(start+size, len(docs))]
		var b db.Builder
		values, err := c.insertValues(d, cols, chunk, &b)
		if err != nil {
			return 0, err
		}
		tag, err := c.Q().Exec(c.Ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", db.Ident(d.TableName()), identList(cols, ""), values), b.Args...)
		if err != nil {
			return 0, c.bulkErr(d, err)
		}
		n += int(tag.RowsAffected())
	}
	if n > 0 {
		c.listChanged(d)
	}
	return n, nil
}

// conflictTarget is the ON CONFLICT clause that names the index key matches
// by: the primary key for id, the field's partial unique index otherwise —
// with its predicate, which Postgres needs to infer a partial index. Both
// lead with the tenant on a tenant's table, as the indexes do.
func conflictTarget(d *meta.DocType, key string) string {
	if key == "id" {
		if d.TenantKeyed() {
			return fmt.Sprintf("(%s, id)", db.Ident(meta.TenantColumn))
		}
		return "(id)"
	}
	f := d.Field(key)
	cols := db.Ident(key)
	if d.TenantKeyed() {
		cols = db.Ident(meta.TenantColumn) + ", " + cols
	}
	return fmt.Sprintf("(%s) WHERE %s", cols, db.UniquePredicate(key, meta.ColumnType(f.Fieldtype)))
}

func (c *Ctx) upsertMany(d *meta.DocType, rows []Doc, opts BulkOpts) (UpsertResult, error) {
	var res UpsertResult
	docs, given, err := c.bulkNewRows(d, bulkUpsert, rows, opts, time.Now())
	if err != nil {
		return res, err
	}
	if opts.Key != "id" {
		f := d.Field(opts.Key)
		for i, doc := range docs {
			if outsideKey(f, doc[opts.Key]) {
				return res, atRow(cerr.Mandatory("{0} is required to identify the document", c.T(f.Label)), i)
			}
		}
	}
	// What an update rewrites is what the row brought, and what a link it
	// brought fetches — never a default the row left out, the id or the key.
	fetchedBy := map[string][]string{}
	for _, f := range d.Fields {
		if f.FetchFrom != "" && meta.HasColumnField(f) {
			link, _, _ := strings.Cut(f.FetchFrom, ".")
			fetchedBy[link] = append(fetchedBy[link], f.Fieldname)
		}
	}
	groups := map[string][]int{}
	sets := map[string][]string{}
	for i, g := range given {
		set := map[string]bool{}
		for k := range g {
			if k == "id" || k == opts.Key {
				continue
			}
			set[k] = true
			for _, fetched := range fetchedBy[k] {
				set[fetched] = true
			}
		}
		cols := slices.Sorted(maps.Keys(set))
		sig := strings.Join(cols, "\x00")
		groups[sig] = append(groups[sig], i)
		sets[sig] = cols
	}
	cols := insertColumns(d)
	size := chunkSize(len(cols))
	table := db.Ident(d.TableName())
	target := conflictTarget(d, opts.Key)
	for _, sig := range slices.Sorted(maps.Keys(groups)) {
		idx := groups[sig]
		set := sets[sig]
		action := "DO NOTHING RETURNING true"
		if len(set) > 0 {
			assign := make([]string, 0, len(set)+2)
			for _, col := range append(slices.Clone(set), "modified", "modified_by") {
				assign = append(assign, db.Ident(col)+" = EXCLUDED."+db.Ident(col))
			}
			action = fmt.Sprintf("DO UPDATE SET %s WHERE (%s) IS DISTINCT FROM (%s) RETURNING (xmax = 0)",
				strings.Join(assign, ", "), identList(set, table+"."), identList(set, "EXCLUDED."))
		}
		for start := 0; start < len(idx); start += size {
			part := idx[start:min(start+size, len(idx))]
			chunk := make([]Doc, len(part))
			for j, i := range part {
				chunk[j] = docs[i]
			}
			var b db.Builder
			values, err := c.insertValues(d, cols, chunk, &b)
			if err != nil {
				return res, err
			}
			sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s ON CONFLICT %s %s", table, identList(cols, ""), values, target, action)
			rs, err := db.Select(c.Ctx, c.Q(), sql, b.Args...)
			if err != nil {
				return res, c.bulkErr(d, err)
			}
			for _, r := range rs {
				for _, v := range r {
					if v == true {
						res.Inserted++
					} else {
						res.Updated++
					}
				}
			}
			res.Unchanged += len(part) - len(rs)
		}
	}
	if res.Inserted+res.Updated > 0 {
		c.listChanged(d)
	}
	return res, nil
}

func (c *Ctx) updateMany(d *meta.DocType, changes []BulkChange, opts BulkOpts) (int, error) {
	// one id named twice is one document: its changes apply in order
	var order []string
	byID := map[string]Doc{}
	firstRow := map[string]int{}
	names := map[string]bool{}
	for i, ch := range changes {
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			return 0, atRow(cerr.Mandatory("Provide the document ID"), i)
		}
		if byID[id] == nil {
			byID[id] = Doc{}
			order = append(order, id)
			firstRow[id] = i
		}
		for k, v := range ch.Values {
			if IsInternalKey(k) {
				continue
			}
			byID[id][k] = v
			names[k] = true
		}
	}
	fields, err := c.bulkFields(d, bulkUpdate, "", names, opts)
	if err != nil {
		return 0, err
	}
	docs := make([]Doc, 0, len(order))
	written := make([]map[string]bool, 0, len(order))
	for _, id := range order {
		i := firstRow[id]
		doc := Doc{}
		w := map[string]bool{}
		for k, v := range byID[id] {
			f := fields[k]
			cv, err := c.castValue(f, v)
			if err != nil {
				return 0, atRow(err, i)
			}
			if f.Reqd && isEmpty(cv) {
				return 0, atRow(cerr.Mandatory("Fill in the required fields: {0}", c.T(f.Label)).WithTitleKey("Required fields"), i)
			}
			doc[k], w[k] = cv, true
		}
		if err := c.checkEmails(d, doc); err != nil {
			return 0, atRow(err, i)
		}
		if err := c.checkSelect(d, doc); err != nil {
			return 0, atRow(err, i)
		}
		docs = append(docs, doc)
		written = append(written, w)
	}
	if err := c.bulkFetch(d, docs, written); err != nil {
		return 0, err
	}
	if !opts.IgnoreLinks {
		if err := c.bulkLinks(d, docs, written); err != nil {
			return 0, err
		}
	}
	table := db.Ident(d.TableName())
	rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT id FROM %s WHERE id = ANY($1)", table), order)
	if err != nil {
		return 0, err
	}
	stored := make(map[string]bool, len(rows))
	for _, r := range rows {
		stored[db.Str(r["id"])] = true
	}
	for _, id := range order {
		if !stored[id] {
			return 0, atRow(cerr.NotFound("{0} {1} not found", d.Name, id), firstRow[id])
		}
	}
	groups := map[string][]int{}
	sets := map[string][]string{}
	for j, w := range written {
		cols := slices.Sorted(maps.Keys(w))
		sig := strings.Join(cols, "\x00")
		groups[sig] = append(groups[sig], j)
		sets[sig] = cols
	}
	now := time.Now()
	n := 0
	for _, sig := range slices.Sorted(maps.Keys(groups)) {
		idx, set := groups[sig], sets[sig]
		types := make([]string, len(set))
		assign := make([]string, len(set))
		for k, col := range set {
			types[k] = meta.ColumnType(d.Field(col).Fieldtype)
			assign[k] = db.Ident(col) + " = v." + db.Ident(col)
		}
		size := chunkSize(len(set) + 1)
		for start := 0; start < len(idx); start += size {
			part := idx[start:min(start+size, len(idx))]
			var b db.Builder
			tuples := make([]string, len(part))
			for t, j := range part {
				ph := []string{b.Arg(order[j]) + "::text"}
				for k, col := range set {
					ph = append(ph, b.Arg(docs[j][col])+"::"+types[k])
				}
				tuples[t] = "(" + strings.Join(ph, ", ") + ")"
			}
			sql := fmt.Sprintf("UPDATE %s AS t SET %s, modified = %s, modified_by = %s FROM (VALUES %s) AS v(id, %s) WHERE t.id = v.id AND (%s) IS DISTINCT FROM (%s)",
				table, strings.Join(assign, ", "), b.Arg(now), b.Arg(c.User), strings.Join(tuples, ", "),
				identList(set, ""), identList(set, "t."), identList(set, "v."))
			tag, err := c.Q().Exec(c.Ctx, sql, b.Args...)
			if err != nil {
				return 0, c.bulkErr(d, err)
			}
			n += int(tag.RowsAffected())
		}
	}
	if n > 0 {
		c.listChanged(d)
	}
	return n, nil
}
