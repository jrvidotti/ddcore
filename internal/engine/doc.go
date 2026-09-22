package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/num"
)

// Doc is a document as a plain map; child tables are []any of Doc.
type Doc map[string]any

func (d Doc) ID() string          { return db.Str(d["id"]) }
func (d Doc) DocType() string     { return db.Str(d["doctype"]) }
func (d Doc) Docstatus() int      { return int(toFloat(d["docstatus"])) }
func (d Doc) Str(k string) string { return db.Str(d[k]) }

func (d Doc) Children(field string) []Doc {
	rows, _ := d[field].([]any)
	out := make([]Doc, 0, len(rows))
	for _, r := range rows {
		switch x := r.(type) {
		case Doc:
			out = append(out, x)
		case map[string]any:
			out = append(out, Doc(x))
		}
	}
	return out
}

func (d Doc) JSON() json.RawMessage {
	b, _ := json.Marshal(d)
	return b
}

func (d Doc) Clone() Doc {
	var out Doc
	json.Unmarshal(d.JSON(), &out)
	return out
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int16:
		return float64(x)
	case bool:
		if x {
			return 1
		}
	case string:
		var f float64
		fmt.Sscanf(x, "%g", &f)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

// finite coerces to a number and refuses the two values a float can hold but a
// column cannot. NaN reaches here from JS arithmetic and ±Inf from a string
// like "1e400"; both used to be written and then read back as null or as a
// driver error, with nothing pointing at the field that caused it.
func finite(f *meta.Field, v any) (float64, error) {
	n := toFloat(v)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, cerr.Validation("{0} is not a number: \"{1}\"", f.Label, db.Str(v))
	}
	return n, nil
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case []any:
		return len(x) == 0
	}
	return false
}

// castOpts is what coercion needs from the site: which timezone a naked
// datetime is written in, how many decimals a Currency has, and which way a
// half rounds. They are properties of the site, never of the request, so the
// engine resolves them once (see Engine.castOpts).
type castOpts struct {
	loc          *time.Location
	currencyPrec int
	rounding     num.Rounding
}

// castValue coerces a JS/JSON value to what the column expects, using the
// engine's site settings.
func (c *Ctx) castValue(f *meta.Field, v any) (any, error) {
	return castValueWith(f, v, c.E.castOpts())
}

// maxNumeric is the ceiling of a numeric(21,9) column: 12 integer digits.
// Beyond it Postgres raises a numeric overflow, which reaches the caller as an
// opaque driver error naming neither the field nor the value.
const maxNumeric = 1e12

// castValueWith coerces a JS/JSON value to what the column expects.
//
// This is the single place a value crosses into the database — REST, MCP, app
// code, `dbSet`, defaults and fixtures all arrive here — which is why it is
// where the money contract is enforced rather than at the INSERT. It also runs
// on *both sides* of the read-only and allow-on-submit comparisons, so a
// document whose stored value predates rounding still compares equal to itself.
func castValueWith(f *meta.Field, v any, o castOpts) (any, error) {
	if isEmpty(v) && f.Fieldtype != "Check" {
		return nil, nil
	}
	switch f.Fieldtype {
	case "Email":
		value := normalizeEmail(db.Str(v))
		if value == "" {
			return nil, nil
		}
		return value, nil
	case "Int":
		n, err := finite(f, v)
		if err != nil {
			return nil, err
		}
		// Int keeps its own rule: half away from zero, and deliberately not
		// subject to the site's money rounding. A count that changed because
		// someone configured banker's rounding for invoices would be a genuine
		// surprise.
		return int64(math.Round(n)), nil
	case "Float":
		// a measurement, not money: `precision` is a display hint and rounding
		// here would destroy data the app meant to keep
		return finite(f, v)
	case "Currency", "Percent":
		n, err := finite(f, v)
		if err != nil {
			return nil, err
		}
		if n >= maxNumeric || n <= -maxNumeric {
			return nil, cerr.Validation("{0} is too large to store: \"{1}\"", f.Label, db.Str(v))
		}
		if f.Fieldtype == "Percent" {
			// a rate, not an amount: 33.333333 is a legitimate third
			return n, nil
		}
		p := o.currencyPrec
		if f.Precision > 0 {
			p = f.Precision
		}
		return num.Round(n, p, o.rounding), nil
	case "Check":
		switch x := v.(type) {
		case bool:
			return x, nil
		case string:
			return x == "1" || strings.EqualFold(x, "true"), nil
		}
		return toFloat(v) != 0, nil
	case "Date":
		s := db.Str(v)
		if len(s) >= 10 {
			s = s[:10]
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, cerr.Validation("Invalid date in {0}: \"{1}\"", f.Label, db.Str(v))
		}
		return s, nil
	case "Month":
		s := db.Str(v)
		if len(s) >= 10 {
			s = s[:10]
		}
		for _, layout := range []string{"2006-01-02", "2006-01", "01/2006", "01/06", "2006/01"} {
			if t, err := time.Parse(layout, s); err == nil {
				return fmt.Sprintf("%04d-%02d-01", t.Year(), t.Month()), nil
			}
		}
		return nil, cerr.Validation("Invalid month in {0}: \"{1}\"", f.Label, db.Str(v))
	case "Datetime":
		// castAll writes the converted time.Time back to the document, so the
		// next coercion of the same field receives an already normalized value.
		if t, ok := v.(time.Time); ok {
			return t, nil
		}
		s := db.Str(v)
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
			if t, err := time.ParseInLocation(layout, s, o.loc); err == nil {
				return t, nil
			}
		}
		return nil, cerr.Validation("Invalid date and time in {0}: \"{1}\"", f.Label, s)
	case "Time":
		// a civil time, and until now the one fieldtype that validated nothing:
		// any string reached Postgres and came back as a driver error naming
		// neither the field nor the value
		s := strings.TrimSpace(db.Str(v))
		for _, layout := range []string{"15:04:05.999999999", "15:04:05", "15:04"} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.Format("15:04:05.999999999"), nil
			}
		}
		return nil, cerr.Validation("Invalid time in {0}: \"{1}\"", f.Label, db.Str(v))
	case "JSON":
		// A save casts twice — once before the validate hook and once after, so
		// that whatever the hook wrote is cast too — and marshalling a value
		// twice buries it inside a JSON string. A string that already parses as
		// JSON has been through here before, or arrived encoded, and is left
		// alone. Version never hit this because it writes its own SQL; a JSON
		// field on an ordinary document does not.
		if s, ok := v.(string); ok && json.Valid([]byte(s)) {
			return s, nil
		}
		b, err := json.Marshal(v)
		return string(b), err
	case "Table", "Vault":
		return v, nil
	}
	return db.Str(v), nil
}

// ------------------------------------------------------------ load & new

func (c *Ctx) docKey(dt, name string) string { return dt + "\x00" + name }

// GetDoc loads a document with its child tables.
func (c *Ctx) GetDoc(doctype, name string) (Doc, error) {
	return c.getDoc(doctype, name, false)
}

// getDocForUpdate loads a document taking a row lock on the parent, so that
// concurrent savers of the same document serialise instead of overwriting
// each other (see writeUpdate's compare-and-swap).
func (c *Ctx) getDocForUpdate(doctype, name string) (Doc, error) {
	var doc Doc
	err := c.WithIgnorePermissions(func() error {
		var e error
		doc, e = c.getDoc(doctype, name, true)
		return e
	})
	return doc, err
}

func (c *Ctx) getDoc(doctype, name string, forUpdate bool) (Doc, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	if d.IsSingle {
		if name == "" {
			name = "singleton"
		}
		if name != "singleton" {
			return nil, cerr.Validation("Invalid Single identity for {0}", d.Name)
		}
	}
	if name == "" {
		return nil, cerr.NotFound("{0}: empty name", doctype)
	}
	sel := fmt.Sprintf("SELECT * FROM %s WHERE id = $1", db.Ident(d.TableName()))
	if forUpdate && c.Tx != nil {
		sel += " FOR UPDATE"
	}
	rows, err := db.Select(c.Ctx, c.Q(), sel, name)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if d.IsSingle && !forUpdate {
			doc, err := c.NewDoc(doctype, nil)
			if err != nil {
				return nil, err
			}
			if !c.IgnorePermissions() {
				ok, err := c.HasPermission(doctype, "read", doc)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, cerr.Permission("No permission to read {0} {1}", c.T(d.Label), name)
				}
			}
			return doc, nil
		}
		return nil, cerr.NotFound("{0} {1} not found", c.T(d.Label), name)
	}
	doc := Doc(rows[0])
	doc["doctype"] = doctype
	for _, tf := range d.TableFields() {
		child, _ := c.St.DocType(tf.OptionsString())
		crows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT * FROM %s WHERE parent = $1 AND parenttype = $2 AND parentfield = $3 ORDER BY idx", db.Ident(child.TableName())), name, doctype, tf.Fieldname)
		if err != nil {
			return nil, err
		}
		list := make([]any, 0, len(crows))
		for _, r := range crows {
			r["doctype"] = child.Name
			list = append(list, r)
		}
		doc[tf.Fieldname] = list
	}
	if !c.IgnorePermissions() {
		if ok, err := c.HasPermission(doctype, "read", doc); err != nil {
			return nil, err
		} else if !ok {
			return nil, cerr.Permission("No permission to read {0} {1}", c.T(d.Label), name)
		}
	}
	return doc, nil
}

// NewDoc builds an unsaved document with defaults applied.
func (c *Ctx) NewDoc(doctype string, values Doc) (Doc, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	doc := Doc{"doctype": doctype, "docstatus": 0, "__islocal": true}
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		if f.Fieldtype == "Table" {
			doc[f.Fieldname] = []any{}
		} else if f.Default != nil {
			doc[f.Fieldname] = c.defaultValue(f)
		} else if f.Fieldtype == "Check" {
			doc[f.Fieldname] = false
		} else {
			doc[f.Fieldname] = nil
		}
	}
	for k, v := range values {
		doc[k] = v
	}
	if d.IsSingle {
		if doc.ID() != "" && doc.ID() != "singleton" {
			return nil, cerr.Validation("Invalid Single identity for {0}", d.Name)
		}
		doc["id"] = "singleton"
	}
	return doc, nil
}

func (c *Ctx) defaultValue(f *meta.Field) any {
	switch s := db.Str(f.Default); s {
	case "Today", "today", "now":
		if f.Fieldtype == "Datetime" {
			// the site's clock, for the same reason Today() is: a default has
			// to mean the same instant the app would have computed itself
			return c.Now().Format(time.RFC3339)
		}
		return c.Today()
	case "__user":
		return c.User
	}
	v, _ := c.castValue(f, f.Default)
	return v
}

// ------------------------------------------------------------ save

type SaveOpts struct {
	IgnorePermissions bool
	IgnoreVersion     bool
	IgnoreLinks       bool
	Action            string // "save" | "submit" | "cancel" | "update_after_submit"
}

// Insert validates and inserts a new document.
func (c *Ctx) Insert(doc Doc, opts SaveOpts) (Doc, error) {
	d, err := c.St.DocType(doc.DocType())
	if err != nil {
		return nil, err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return nil, err
	}
	if d.IsChild {
		return nil, cerr.Validation("{0} is a child table", d.Name)
	}
	if d.Name == "Audit Event" {
		return nil, cerr.Permission("Audit Event records are immutable and cannot be created directly")
	}
	permission := "create"
	if d.IsSingle {
		permission = "write"
		if doc.ID() != "" && doc.ID() != "singleton" {
			return nil, cerr.Validation("Invalid Single identity for {0}", d.Name)
		}
	}
	if !opts.IgnorePermissions && !c.IgnorePermissions() {
		if ok, err := c.HasPermission(d.Name, permission, doc); err != nil {
			return nil, err
		} else if !ok {
			if d.IsSingle {
				return nil, cerr.Permission("No permission ({0}) on {1} {2}", "write", c.T(d.Label), "singleton")
			}
			return nil, cerr.Permission("No permission to create {0}", c.T(d.Label))
		}
	}
	if doc.Docstatus() == 1 && d.Submittable {
		if !opts.IgnorePermissions && !c.IgnorePermissions() {
			if ok, _ := c.HasPermission(d.Name, "submit", doc); !ok {
				return nil, cerr.Permission("No permission to submit {0}", c.T(d.Label))
			}
		}
	} else if doc.Docstatus() != 0 {
		return nil, cerr.Validation("Invalid docstatus for an insert")
	}
	if wf := c.WorkflowFor(d.Name); wf != nil {
		if doc[wf.StateField] == nil || doc[wf.StateField] == "" {
			doc[wf.StateField] = wf.InitialState
		} else if doc.Str(wf.StateField) != wf.InitialState && !c.inWorkflowTransition {
			// like SaveDoc's guard, this yields only to a workflow transition, never
			// to opts.IgnorePermissions or c.IgnorePermissions() — a bulk import or a
			// background job cannot insert a document past the initial state; data
			// repair belongs in a migration patch's ctx.sql.
			return nil, cerr.Validation("New {0} must start in initial workflow state '{1}'", d.Name, wf.InitialState)
		}
		// a submitted insert would skip every approval the workflow requires
		if initial := wf.GetState(wf.InitialState); initial != nil && doc.Docstatus() != initial.Docstatus && !c.inWorkflowTransition {
			return nil, cerr.Validation("New {0} must start with the docstatus of initial workflow state '{1}'", d.Name, wf.InitialState)
		}
	}
	if c.fieldPermissionsApply(d, opts) {
		base, err := c.insertFieldBase(d, doc)
		if err != nil {
			return nil, err
		}
		if err := c.applyFieldWrites(d, base, doc); err != nil {
			return nil, err
		}
	}
	doc["__islocal"] = true
	now := time.Now()
	doc["owner"], doc["creation"], doc["modified"], doc["modified_by"] = c.User, now, now, c.User
	if doc.Docstatus() != 1 {
		doc["docstatus"] = 0
	}
	if err := c.runHook(d, "beforeInsert", doc, nil); err != nil {
		return nil, err
	}
	if err := c.setID(d, doc); err != nil {
		return nil, err
	}
	if err := c.validate(d, doc, nil, opts); err != nil {
		return nil, err
	}
	if err := c.runHook(d, "beforeSave", doc, nil); err != nil {
		return nil, err
	}
	if doc.Docstatus() == 1 {
		if err := c.runHook(d, "beforeSubmit", doc, nil); err != nil {
			return nil, err
		}
	}
	if !c.IgnorePermissions() {
		if ok, err := c.checkUserPermissions(d, doc); err != nil {
			return nil, err
		} else if !ok {
			if d.IsSingle {
				return nil, cerr.Permission("No permission ({0}) on {1} {2}", "write", c.T(d.Label), "singleton")
			}
			return nil, cerr.Permission("No permission to create {0}", c.T(d.Label))
		}
	}
	if err := c.writeInsert(d, doc); err != nil {
		return nil, err
	}
	if err := c.writeChildren(d, doc); err != nil {
		return nil, err
	}
	if err := c.processVaultFields(d, doc); err != nil {
		return nil, err
	}
	delete(doc, "__islocal")
	delete(doc, "__unsaved")
	if err := c.runHook(d, "afterInsert", doc, nil); err != nil {
		return nil, err
	}
	if err := c.runHook(d, "onUpdate", doc, nil); err != nil {
		return nil, err
	}
	if doc.Docstatus() == 1 {
		if err := c.runHook(d, "onSubmit", doc, nil); err != nil {
			return nil, err
		}
	}
	saved, err := c.GetDocIgnoringPerms(d.Name, doc.ID())
	if err != nil {
		return nil, err
	}
	if err := c.queueDocWebhooks(d.Name, saved, "on_insert"); err != nil {
		return nil, err
	}
	if err := c.queueDocNotifications(d.Name, saved, nil, "on_insert"); err != nil {
		return nil, err
	}
	if saved.Docstatus() == 1 {
		if err := c.queueDocWebhooks(d.Name, saved, "on_submit"); err != nil {
			return nil, err
		}
		if err := c.queueDocNotifications(d.Name, saved, nil, "on_submit"); err != nil {
			return nil, err
		}
	}
	c.notify(d, saved, "insert")
	if d.Name == "User Permission" {
		if err := c.auditUserPermissionGrant(saved); err != nil {
			return nil, err
		}
		c.invalidateUserPermissionCache(saved)
	}
	if d.Name == shareDoctype {
		if err := c.auditShareSaved(nil, saved); err != nil {
			return nil, err
		}
	}
	return saved, nil
}

// Save updates an existing document (or inserts when __islocal).
func (c *Ctx) Save(doc Doc, opts SaveOpts) (Doc, error) {
	if doc["__islocal"] == true || doc.ID() == "" {
		return c.Insert(doc, opts)
	}
	d, err := c.St.DocType(doc.DocType())
	if err != nil {
		return nil, err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return nil, err
	}
	if d.Name == "Audit Event" {
		return nil, cerr.Permission("Audit Event records are immutable and cannot be modified")
	}
	// FOR UPDATE: subsequent callers wait here and only then compare the
	// timestamp, instead of reading a version that is actively being modified.
	before, err := c.getDocForUpdate(d.Name, doc.ID())
	if err != nil {
		return nil, err
	}
	oldStatus, newStatus := before.Docstatus(), doc.Docstatus()
	action := "save"
	switch {
	case oldStatus == 0 && newStatus == 1:
		action = "submit"
	case (oldStatus == 1 || (c.inWorkflowTransition && oldStatus == 0)) && newStatus == 2:
		action = "cancel"
	case oldStatus == 1 && newStatus == 1:
		action = "update_after_submit"
	case oldStatus == 2:
		return nil, cerr.Validation("{0} {1} is cancelled and cannot be changed", c.T(d.Label), doc.ID())
	case oldStatus == 0 && newStatus == 0:
	default:
		return nil, cerr.Validation("Invalid docstatus transition ({0} → {1})", oldStatus, newStatus)
	}
	if action != "save" && !d.Submittable {
		return nil, cerr.Validation("{0} is not submittable", c.T(d.Label))
	}
	if wf := c.WorkflowFor(d.Name); wf != nil && !c.inWorkflowTransition {
		if doc.Str(wf.StateField) != before.Str(wf.StateField) {
			return nil, cerr.Validation("Cannot manually modify workflow state field '{0}'. Use workflow actions to transition.", wf.StateField)
		}
		if before.Docstatus() != doc.Docstatus() {
			return nil, cerr.Validation("Direct submit or cancel is disabled for documents governed by workflow '{0}'", wf.Name)
		}
	}
	ptype := "write"
	if action == "submit" {
		ptype = "submit"
	} else if action == "cancel" {
		ptype = "cancel"
	}
	if !opts.IgnorePermissions && !c.IgnorePermissions() && !c.inWorkflowTransition {
		if ok, err := c.HasPermission(d.Name, ptype, before); err != nil {
			return nil, err
		} else if !ok {
			return nil, cerr.Permission("No permission ({0}) on {1} {2}", ptype, c.T(d.Label), doc.ID())
		}
	}
	if !c.IgnorePermissions() {
		if ok, err := c.scopeAllows(d, before, ptype); err != nil {
			return nil, err
		} else if !ok {
			return nil, cerr.Permission("No permission ({0}) on {1} {2}", ptype, c.T(d.Label), doc.ID())
		}
	}
	// optimistic concurrency
	if m, ok := doc["modified"]; ok && m != nil && before["modified"] != nil {
		if !sameTime(m, before["modified"], c.E.Location()) {
			return nil, cerr.Timestamp("The document was changed by someone else after you opened it. Reload and try again.")
		}
	}
	doc["owner"], doc["creation"] = before["owner"], before["creation"]
	doc["modified"], doc["modified_by"] = time.Now(), c.User
	if c.fieldPermissionsApply(d, opts) {
		if err := c.applyFieldWrites(d, before, doc); err != nil {
			return nil, err
		}
	}
	if action == "update_after_submit" {
		if err := c.checkAllowOnSubmit(d, before, doc); err != nil {
			return nil, err
		}
	}
	if err := c.validate(d, doc, before, opts); err != nil {
		return nil, err
	}
	if err := c.runHook(d, "beforeSave", doc, before); err != nil {
		return nil, err
	}
	if action == "update_after_submit" {
		// hooks ran after the first check and might have modified protected
		// fields: check again with the final document. The deliberate
		// exception remains dbSet, which does not pass through here.
		if err := c.checkAllowOnSubmit(d, before, doc); err != nil {
			return nil, err
		}
	}
	switch action {
	case "submit":
		if err := c.runHook(d, "beforeSubmit", doc, before); err != nil {
			return nil, err
		}
	case "cancel":
		if err := c.runHook(d, "beforeCancel", doc, before); err != nil {
			return nil, err
		}
	}
	if !c.IgnorePermissions() {
		if ok, err := c.scopeAllows(d, doc, ptype); err != nil {
			return nil, err
		} else if !ok {
			return nil, cerr.Permission("No permission ({0}) on {1} {2}", ptype, c.T(d.Label), doc.ID())
		}
	}
	if err := c.writeUpdate(d, doc, before["modified"]); err != nil {
		return nil, err
	}
	if err := c.writeChildren(d, doc); err != nil {
		return nil, err
	}
	if err := c.processVaultFields(d, doc); err != nil {
		return nil, err
	}
	delete(doc, "__unsaved")
	if d.TrackChanges && !opts.IgnoreVersion {
		c.saveVersion(d, before, doc)
	}
	switch action {
	case "submit":
		if err := c.runHook(d, "onUpdate", doc, before); err != nil {
			return nil, err
		}
		if err := c.runHook(d, "onSubmit", doc, before); err != nil {
			return nil, err
		}
	case "cancel":
		if err := c.runHook(d, "onCancel", doc, before); err != nil {
			return nil, err
		}
	case "update_after_submit":
		if err := c.runHook(d, "onUpdateAfterSubmit", doc, before); err != nil {
			return nil, err
		}
	default:
		if err := c.runHook(d, "onUpdate", doc, before); err != nil {
			return nil, err
		}
	}
	saved, err := c.GetDocIgnoringPerms(d.Name, doc.ID())
	if err != nil {
		return nil, err
	}
	if err := c.queueDocWebhooks(d.Name, saved, webhookSaveEvent[action]); err != nil {
		return nil, err
	}
	if err := c.queueDocNotifications(d.Name, saved, before, webhookSaveEvent[action]); err != nil {
		return nil, err
	}
	c.notify(d, saved, action)
	if d.Name == "User Permission" {
		if userPermissionScopeChanged(before, saved) {
			if err := c.auditUserPermissionRevoke(before); err != nil {
				return nil, err
			}
			if err := c.auditUserPermissionGrant(saved); err != nil {
				return nil, err
			}
		}
		c.invalidateUserPermissionCache(before, saved)
	}
	if d.Name == shareDoctype {
		if err := c.auditShareSaved(before, saved); err != nil {
			return nil, err
		}
	}
	return saved, nil
}

// sameTime compares two timestamps at millisecond precision. A value that
// cannot be parsed is never "the same": accepting an invalid timestamp was
// equivalent to disabling concurrency control.
func sameTime(a, b any, loc *time.Location) bool {
	if a == nil && b == nil {
		return true
	}
	pa, pb := parseTime(a, loc), parseTime(b, loc)
	if pa.IsZero() || pb.IsZero() {
		return false
	}
	return pa.Truncate(time.Millisecond).Equal(pb.Truncate(time.Millisecond))
}

// parseTime reads a timestamp that carries its own offset, or — for the naked
// layouts an app writes by hand — the site's wall clock.
//
// It takes the location for the same reason castValue does: two parsers that
// disagree about which clock a naked string was written on is one bug, not
// several. `ddcore.utils.now()` returns the site's wall clock with no offset,
// and it may be handed straight back as a job's runAfter.
func parseTime(v any, loc *time.Location) time.Time {
	switch x := v.(type) {
	case time.Time:
		return x
	case string:
		for _, l := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07", "2006-01-02 15:04:05"} {
			if t, err := time.ParseInLocation(l, x, loc); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// Submit sets docstatus=1 and saves.
func (c *Ctx) Submit(doc Doc) (Doc, error) {
	doc["docstatus"] = 1
	return c.Save(doc, SaveOpts{})
}

// Cancel sets docstatus=2 and saves.
func (c *Ctx) Cancel(doc Doc) (Doc, error) {
	if doc.Docstatus() != 1 {
		return nil, cerr.Validation("Only submitted documents can be cancelled")
	}
	doc["docstatus"] = 2
	return c.Save(doc, SaveOpts{})
}

// SaveDoc is an alias for Save.
func (c *Ctx) SaveDoc(doc Doc, opts SaveOpts) (Doc, error) {
	return c.Save(doc, opts)
}

// SubmitDoc is an alias for Submit.
func (c *Ctx) SubmitDoc(doc Doc) (Doc, error) {
	return c.Submit(doc)
}

// CancelDoc is an alias for Cancel.
func (c *Ctx) CancelDoc(doc Doc) (Doc, error) {
	return c.Cancel(doc)
}

// Amend creates a draft copy of a cancelled document.
func (c *Ctx) Amend(doctype, name string) (Doc, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return nil, err
	}
	src, err := c.GetDoc(doctype, name)
	if err != nil {
		return nil, err
	}
	if src.Docstatus() != 2 {
		return nil, cerr.Validation("Only cancelled documents can be amended")
	}
	if ok, _ := c.HasPermission(doctype, "amend", src); !ok && !c.IgnorePermissions() {
		return nil, cerr.Permission("No permission to amend {0}", c.T(d.Label))
	}
	doc := src.Clone()
	for _, k := range []string{"id", "owner", "creation", "modified", "modified_by"} {
		delete(doc, k)
	}
	doc["docstatus"], doc["__islocal"] = 0, true
	// the amendment starts the workflow over: a copied state would be refused on insert
	if wf := c.WorkflowFor(doctype); wf != nil {
		delete(doc, wf.StateField)
	}
	if d.Field("amended_from") != nil {
		doc["amended_from"] = name
	}
	// "X" → "X-1"; amending "X-1" again → "X-2"
	base := name
	if src.Str("amended_from") != "" {
		if i := strings.LastIndex(name, "-"); i > 0 {
			base = name[:i]
		}
	}
	n := 1
	for {
		cand := fmt.Sprintf("%s-%d", base, n)
		if ok, _ := c.idExists(doctype, cand); !ok {
			doc["id"] = cand
			break
		}
		n++
	}
	for _, tf := range d.TableFields() {
		for _, row := range doc.Children(tf.Fieldname) {
			delete(row, "id")
			delete(row, "parent")
		}
	}
	return doc, nil
}

// DBSet writes columns directly, bypassing validation (allowed after submit).
// It returns the new `modified` so the caller can keep its in-memory document
// in sync (B21) — zero when nothing was written.
func (c *Ctx) DBSet(doctype, name string, values Doc, updateModified bool) (time.Time, error) {
	var modified time.Time
	d, err := c.St.DocType(doctype)
	if err != nil {
		return modified, err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return modified, err
	}
	if d.Name == "Audit Event" {
		return modified, cerr.Permission("Audit Event records are immutable and cannot be modified")
	}
	if !c.IgnorePermissions() {
		if refused, err := c.refusedToScopedUser(d.Name); err != nil {
			return modified, err
		} else if refused {
			return modified, cerr.Permission("No permission ({0}) on {1} {2}", "write", c.T(d.Label), name)
		}
	}
	// the state field and docstatus move only through a workflow transition;
	// this does not yield to c.IgnorePermissions() (raised for every background
	// job) — a job that needs to repair workflow data uses a migration patch's
	// ctx.sql instead.
	if wf := c.WorkflowFor(d.Name); wf != nil && !c.inWorkflowTransition {
		if _, ok := values[wf.StateField]; ok {
			return modified, cerr.Validation("Cannot manually modify workflow state field '{0}'. Use workflow actions to transition.", wf.StateField)
		}
		if _, ok := values["docstatus"]; ok {
			return modified, cerr.Validation("Direct submit or cancel is disabled for documents governed by workflow '{0}'", wf.Name)
		}
	}
	if d.IsSingle {
		if name != "singleton" {
			return modified, cerr.Validation("Invalid Single identity for {0}", d.Name)
		}
		if v, ok := values["id"]; ok && v != "singleton" {
			return modified, cerr.Validation("Invalid Single identity for {0}", d.Name)
		}
		if v, ok := values["docstatus"]; ok && toFloat(v) != 0 {
			return modified, cerr.Validation("Invalid Single identity or status for {0}", d.Name)
		}
	}
	if len(values) == 0 {
		return modified, nil
	}
	if !c.IgnorePermissions() {
		// scope: neither the stored document nor the written values may be out
		// of the user's scope, as for Save
		if perms, err := c.UserPermissions(); err != nil {
			return modified, err
		} else if len(perms) > 0 {
			stored, err := c.GetDocIgnoringPerms(d.Name, name)
			if err != nil {
				return modified, err
			}
			merged := Doc{}
			for k, v := range stored {
				merged[k] = v
			}
			for k, v := range values {
				merged[k] = v
			}
			// A child row's scope applies under its parent's applicable_for,
			// the same rule Save enforces (perm.go's recursion into table
			// rows). d.Name (the child DocType's own name) would miss any
			// rule scoped to the parent DocType.
			applicableFor := d.Name
			if d.IsChild {
				if pt := stored.Str("parenttype"); pt != "" {
					applicableFor = pt
				}
			}
			// a share that overrides the scopes for write lifts them here too;
			// a child row is shared through its parent
			sharedDoctype, sharedName := d.Name, name
			if d.IsChild {
				sharedDoctype, sharedName = stored.Str("parenttype"), stored.Str("parent")
			}
			for _, doc := range []Doc{stored, merged} {
				if ok, err := c.checkUserPermissionsFor(d, doc, applicableFor); err != nil {
					return modified, err
				} else if !ok {
					if lifted, err := c.scopeOverridden(sharedDoctype, sharedName, "write"); err != nil {
						return modified, err
					} else if !lifted {
						return modified, cerr.Permission("No permission ({0}) on {1} {2}", "write", c.T(d.Label), name)
					}
				}
			}
		}
	}
	var beforeShare Doc
	if d.Name == shareDoctype {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT "user", share_doctype, share_id, "read", "write", "share", override_scope FROM tab_document_share WHERE id = $1`, name)
		if err != nil {
			return modified, err
		}
		if len(rows) > 0 {
			beforeShare = Doc(rows[0])
		}
	}
	var beforePermission Doc
	if d.Name == "User Permission" {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT "user", allow, for_value, applicable_for FROM tab_user_permission WHERE id = $1`, name)
		if err != nil {
			return modified, err
		}
		if len(rows) > 0 {
			beforePermission = Doc(rows[0])
		}
	}
	var b db.Builder
	var sets []string
	for k, v := range values {
		f := d.Field(k)
		if f == nil || meta.ColumnType(f.Fieldtype) == "" {
			if !d.IsStdColumn(k) {
				return modified, cerr.Validation("Field {0} does not exist on {1}", k, doctype)
			}
			sets = append(sets, db.Ident(k)+" = "+b.Arg(v))
			continue
		}
		cv, err := c.castValue(f, v)
		if err != nil {
			return modified, err
		}
		sets = append(sets, db.Ident(k)+" = "+b.Arg(cv))
	}
	if updateModified {
		modified = time.Now()
		sets = append(sets, "modified = "+b.Arg(modified), "modified_by = "+b.Arg(c.User))
	}
	sql := fmt.Sprintf("UPDATE %s SET %s WHERE id = %s", db.Ident(d.TableName()), strings.Join(sets, ", "), b.Arg(name))
	tag, err := c.Q().Exec(c.Ctx, sql, b.Args...)
	if err != nil {
		return modified, err
	}
	if tag.RowsAffected() == 0 {
		return modified, cerr.NotFound("{0} {1} not found", doctype, name)
	}
	if d.Name == "User Permission" {
		afterPermission := Doc{}
		for _, field := range []string{"user", "allow", "for_value", "applicable_for"} {
			afterPermission[field] = beforePermission[field]
			if value, ok := values[field]; ok {
				afterPermission[field] = value
			}
		}
		if userPermissionScopeChanged(beforePermission, afterPermission) {
			if err := c.auditUserPermissionRevoke(beforePermission); err != nil {
				return modified, err
			}
			if err := c.auditUserPermissionGrant(afterPermission); err != nil {
				return modified, err
			}
		}
		c.invalidateUserPermissionCache(beforePermission, afterPermission)
	}
	if beforeShare != nil {
		afterShare := Doc{}
		for k, v := range beforeShare {
			afterShare[k] = v
			if value, ok := values[k]; ok {
				if f := d.Field(k); f != nil {
					if cv, err := c.castValue(f, value); err == nil {
						value = cv
					}
				}
				afterShare[k] = value
			}
		}
		if err := c.auditShareSaved(beforeShare, afterShare); err != nil {
			return modified, err
		}
	}
	// only after commit: a rolled back transaction must not announce
	// changes that never took place (B20).
	c.AfterCommit(func() {
		c.E.Events.Publish(Event{Name: "doc_update", Doctype: doctype, DocID: name, Payload: map[string]any{"doctype": doctype, "id": name}})
	})
	return modified, nil
}

// Delete removes a document after checking links.
func (c *Ctx) Delete(doctype, name string, ignorePerms, force bool) error {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return err
	}
	if doctype == "Audit Event" {
		return cerr.Permission("Audit Event records are immutable and cannot be deleted")
	}
	if d.IsSingle {
		return cerr.Validation("Single DocTypes cannot be deleted or renamed")
	}
	doc, err := c.GetDocIgnoringPerms(doctype, name)
	if err != nil {
		return err
	}
	if !ignorePerms && !c.IgnorePermissions() {
		if ok, _ := c.HasPermission(doctype, "delete", doc); !ok {
			return cerr.Permission("No permission to delete {0} {1}", c.T(d.Label), name)
		}
	}
	if !c.IgnorePermissions() {
		if ok, err := c.checkUserPermissions(d, doc); err != nil {
			return err
		} else if !ok {
			return cerr.Permission("No permission to delete {0} {1}", c.T(d.Label), name)
		}
	}
	// a draft that has left the initial state is deleted only by a role that
	// may edit it there; a cancelled document keeps the rules above. This does
	// not yield to ignorePerms or c.IgnorePermissions() — ddcore.deleteDoc(...,
	// {ignorePermissions:true}) and a background job (which always runs with
	// c.IgnorePermissions() true) cannot delete a document out from under an
	// approval; data repair belongs in a migration patch's ctx.sql.
	if wf := c.WorkflowFor(doctype); wf != nil && doc.Docstatus() == 0 && c.User != "Admin" {
		st := doc.Str(wf.StateField)
		if st == "" {
			st = wf.InitialState
		}
		state := wf.GetState(st)
		if state == nil || state.Docstatus != 0 || (st != wf.InitialState && !c.workflowStateAllowsEdit(state)) {
			return cerr.Permission("No permission to delete {0} {1} in workflow state '{2}'", c.T(d.Label), name, c.T(st))
		}
	}
	if doc.Docstatus() == 1 {
		return cerr.Validation("Cancel {0} {1} before deleting", c.T(d.Label), name)
	}
	if err := c.runHook(d, "onTrash", doc, nil); err != nil {
		return err
	}
	if !force {
		if err := c.checkLinksBeforeDelete(d, name); err != nil {
			return err
		}
	}
	for _, tf := range d.TableFields() {
		child, _ := c.St.DocType(tf.OptionsString())
		if child != nil {
			for _, row := range doc.Children(tf.Fieldname) {
				for _, cf := range child.Fields {
					if cf.Fieldtype == "Vault" {
						key := c.DeriveVaultKey(child, cf, row)
						_ = c.E.VaultDel(c, key)
					}
				}
			}
		}
		if _, err := c.Q().Exec(c.Ctx, fmt.Sprintf("DELETE FROM %s WHERE parent = $1 AND parenttype = $2", db.Ident(child.TableName())), name, doctype); err != nil {
			return err
		}
	}
	for _, f := range d.Fields {
		if f.Fieldtype == "Vault" {
			key := c.DeriveVaultKey(d, f, doc)
			_ = c.E.VaultDel(c, key)
		}
	}
	if _, err := c.Q().Exec(c.Ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", db.Ident(d.TableName())), name); err != nil {
		return err
	}
	if d.Name == "File" {
		c.deleteFileBytesAfterCommit(db.Str(doc["file_url"]))
	}
	// Attachments go with their document, and so do their bytes. Read the urls
	// before the rows disappear below.
	var attached []string
	rows, err := c.Q().Query(c.Ctx, "SELECT file_url FROM tab_file WHERE attached_to_doctype = $1 AND attached_to_id = $2", doctype, name)
	if err != nil {
		return err
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return err
		}
		attached = append(attached, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	c.deleteFileBytesAfterCommit(attached...)
	// The tables that name a document without linking to it. File was missing
	// here: deleting a document left its attachments behind, pointing at a name
	// nothing answers to. Email Delivery is deliberately exempt — see
	// keepOnDelete in coreRefs.
	for _, ref := range coreRefs {
		if ref.keepOnDelete {
			continue
		}
		tag, err := c.Q().Exec(c.Ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = $1 AND %s = $2",
			db.Ident(ref.table), db.Ident(ref.doctypeCol), db.Ident(ref.idCol)), doctype, name)
		if err == nil && ref.table == "tab_document_share" && tag.RowsAffected() > 0 {
			c.allSharesChanged()
		}
	}
	delete(c.docCache, c.docKey(doctype, name))
	if err := c.runHook(d, "afterDelete", doc, nil); err != nil {
		return err
	}
	if err := c.queueDocWebhooks(d.Name, doc, "on_trash"); err != nil {
		return err
	}
	if d.Name == "User Permission" {
		if err := c.auditUserPermissionRevoke(doc); err != nil {
			return err
		}
		c.invalidateUserPermissionCache(doc)
	}
	if d.Name == "User" {
		// Not a DocType, so the meta cannot find it: a sign-in identity left
		// behind would sign a stranger into a User later created with the
		// same name.
		if _, err := c.Q().Exec(c.Ctx, `DELETE FROM ddcore_user_identity WHERE "user" = $1`, name); err != nil {
			return err
		}
	}
	if d.Name == shareDoctype {
		if err := c.auditShareDeleted(doc); err != nil {
			return err
		}
	}
	c.AfterCommit(func() {
		c.E.Events.Publish(Event{Name: "list_update", Payload: map[string]any{"doctype": doctype}})
	})
	return nil
}

// Rename changes a document's name and every link pointing to it.
func (c *Ctx) Rename(doctype, oldID, newID string) (string, error) {
	d, err := c.St.DocType(doctype)
	if err != nil {
		return "", err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return "", err
	}
	if d.IsSingle {
		return "", cerr.Validation("Single DocTypes cannot be deleted or renamed")
	}
	newID = strings.TrimSpace(newID)
	if newID == "" || newID == oldID {
		return oldID, nil
	}
	if !d.AllowRename {
		return "", cerr.Validation("{0} cannot be renamed", c.T(d.Label))
	}
	doc, err := c.GetDoc(doctype, oldID)
	if err != nil {
		return "", err
	}
	if ok, _ := c.HasPermission(doctype, "write", doc); !ok && !c.IgnorePermissions() {
		return "", cerr.Permission("No permission to rename {0}", c.T(d.Label))
	}
	if ok, _ := c.idExists(doctype, newID); ok {
		return "", cerr.Duplicate("{0} {1} already exists", c.T(d.Label), newID)
	}
	if err := c.runHook(d, "beforeRename", doc, nil); err != nil {
		return "", err
	}
	if err := c.moveID(d, oldID, newID); err != nil {
		return "", err
	}
	delete(c.docCache, c.docKey(doctype, oldID))
	doc["id"] = newID
	if err := c.runHook(d, "afterRename", doc, nil); err != nil {
		return "", err
	}
	c.AfterCommit(func() {
		c.E.Events.Publish(Event{Name: "list_update", Payload: map[string]any{"doctype": doctype}})
	})
	return newID, nil
}

// moveID is the data half of Rename: the row, its vault keys, and every
// reference the meta and coreRefs know about. It checks nothing and runs no
// hook, which is what lets a migration rename a document its DocType does not
// allow a person to rename.
func (c *Ctx) moveID(d *meta.DocType, oldID, newID string) error {
	doctype := d.Name
	q := c.Q()
	if _, err := q.Exec(c.Ctx, fmt.Sprintf("UPDATE %s SET id = $1 WHERE id = $2", db.Ident(d.TableName())), newID, oldID); err != nil {
		return err
	}
	if d.IDGeneration.Field != "" {
		q.Exec(c.Ctx, fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = $1", db.Ident(d.TableName()), db.Ident(d.IDGeneration.Field)), newID)
	}
	// Vault keys default to "<DocType>:<Fieldname>:<name>" (DeriveVaultKey); a
	// rename must carry the default-shaped ones to the new name, or the
	// secret is orphaned under a name the document no longer answers to. A
	// custom key template is not touched — see the vault docs for the
	// limitation.
	for _, f := range d.Fields {
		if f.Fieldtype != "Vault" || f.OptionsString() != "" {
			continue
		}
		oldKey := c.DeriveVaultKey(d, f, Doc{"id": oldID})
		newKey := c.DeriveVaultKey(d, f, Doc{"id": newID})
		if _, err := q.Exec(c.Ctx, "UPDATE ddcore_vault SET name = $1 WHERE name = $2", newKey, oldKey); err != nil {
			return err
		}
	}
	for _, other := range c.St.Meta.DocTypes {
		t := db.Ident(other.TableName())
		if other.IsChild {
			q.Exec(c.Ctx, fmt.Sprintf("UPDATE %s SET parent = $1 WHERE parent = $2 AND parenttype = $3", t), newID, oldID, doctype)
		}
		for _, f := range other.Fields {
			switch f.Fieldtype {
			case "Link":
				if f.OptionsString() == doctype {
					if _, err := q.Exec(c.Ctx, fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s = $2", t, db.Ident(f.Fieldname), db.Ident(f.Fieldname)), newID, oldID); err != nil {
						return err
					}
				}
			case "Dynamic Link":
				tf := f.OptionsString()
				if other.Field(tf) != nil {
					q.Exec(c.Ctx, fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s = $2 AND %s = $3", t, db.Ident(f.Fieldname), db.Ident(f.Fieldname), db.Ident(tf)), newID, oldID, doctype)
				}
			}
		}
	}
	// Same tables, same omission: a renamed document used to lose its
	// attachments, and with them the permission check on a private file, which
	// reads File.attached_to_id to decide who may download it. Rename sweeps
	// every one of them, keepOnDelete or not: a record that survives its
	// document must still point at the name that document answers to now.
	for _, ref := range coreRefs {
		tag, err := q.Exec(c.Ctx, fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s = $2 AND %s = $3",
			db.Ident(ref.table), db.Ident(ref.idCol), db.Ident(ref.doctypeCol), db.Ident(ref.idCol)),
			newID, doctype, oldID)
		if err != nil {
			return fmt.Errorf("coreRefs update %s: %w", ref.table, err)
		}
		if ref.table == "tab_document_share" && tag.RowsAffected() > 0 {
			c.allSharesChanged()
		}
	}
	if d.Name == "User" {
		if _, err := q.Exec(c.Ctx, `UPDATE ddcore_user_identity SET "user" = $1 WHERE "user" = $2`, newID, oldID); err != nil {
			return fmt.Errorf("identity rename: %w", err)
		}
	}
	return nil
}

// GetDocIgnoringPerms loads without the read check.
func (c *Ctx) GetDocIgnoringPerms(doctype, name string) (Doc, error) {
	var doc Doc
	err := c.WithIgnorePermissions(func() error {
		var e error
		doc, e = c.GetDoc(doctype, name)
		return e
	})
	return doc, err
}

func (c *Ctx) notify(d *meta.DocType, doc Doc, action string) {
	c.AfterCommit(func() {
		c.E.Events.Publish(Event{Name: "doc_update", Doctype: d.Name, DocID: doc.ID(), Payload: map[string]any{"doctype": d.Name, "id": doc.ID(), "action": action, "modified": doc["modified"], "user": c.User}})
		c.E.Events.Publish(Event{Name: "list_update", Payload: map[string]any{"doctype": d.Name}})
	})
}

func userPermissionScopeChanged(before, after Doc) bool {
	for _, field := range []string{"user", "allow", "for_value", "applicable_for"} {
		if before.Str(field) != after.Str(field) {
			return true
		}
	}
	return false
}

func (c *Ctx) auditUserPermissionGrant(doc Doc) error {
	return c.Audit("permission.scope_grant", "User", doc.Str("user"), map[string]any{
		"allow":          doc.Str("allow"),
		"for_value":      doc.Str("for_value"),
		"applicable_for": doc.Str("applicable_for"),
	})
}

func (c *Ctx) auditUserPermissionRevoke(doc Doc) error {
	return c.Audit("permission.scope_revoke", "User", doc.Str("user"), map[string]any{
		"allow":     doc.Str("allow"),
		"for_value": doc.Str("for_value"),
	})
}

// ------------------------------------------------------------ hooks

func (c *Ctx) runHook(d *meta.DocType, event string, doc Doc, before Doc) error {
	rt, err := c.RT()
	if err != nil {
		return err
	}
	if !rt.HasHook(d.Name, event) {
		return nil
	}
	var bj json.RawMessage
	if before != nil {
		bj = before.JSON()
	}
	out, err := rt.RunHook(d.Name, event, doc.JSON(), bj)
	if err != nil {
		return err
	}
	var updated Doc
	if err := json.Unmarshal(out, &updated); err != nil {
		return fmt.Errorf("hook %s.%s returned invalid JSON: %w", d.Name, event, err)
	}
	for k := range doc {
		delete(doc, k)
	}
	for k, v := range updated {
		doc[k] = v
	}
	return nil
}

// ------------------------------------------------------------ validation

func (c *Ctx) validate(d *meta.DocType, doc Doc, before Doc, opts SaveOpts) error {
	if err := c.runHook(d, "beforeValidate", doc, before); err != nil {
		return err
	}
	if err := c.castAll(d, doc); err != nil {
		return err
	}
	if err := c.fetchFrom(d, doc); err != nil {
		return err
	}
	if err := c.checkReadOnlyDependsOn(d, doc, before); err != nil {
		return err
	}
	if err := c.runHook(d, "validate", doc, before); err != nil {
		return err
	}
	if err := c.castAll(d, doc); err != nil {
		return err
	}
	if err := c.checkMandatory(d, doc); err != nil {
		return err
	}
	if err := c.checkEmails(d, doc); err != nil {
		return err
	}
	if err := c.checkSelect(d, doc); err != nil {
		return err
	}
	if !opts.IgnoreLinks {
		if err := c.checkLinks(d, doc); err != nil {
			return err
		}
	}
	if err := c.checkUnique(d, doc); err != nil {
		return err
	}
	if err := c.checkUniqueKeys(d, doc); err != nil {
		return err
	}
	return c.validateChildren(d, doc, opts)
}

func (c *Ctx) checkEmails(d *meta.DocType, doc Doc) error {
	for _, f := range d.Fields {
		if f.Fieldtype != "Email" || isEmpty(doc[f.Fieldname]) {
			continue
		}
		value := doc.Str(f.Fieldname)
		if d.Name == "User" && f.Fieldname == "email" && (value == "Admin" || value == "Guest") {
			continue
		}
		if !validEmail(value) {
			return cerr.Validation("{0}: \"{1}\" is not a valid email address", c.T(f.Label), value).WithTitleKey("Invalid email")
		}
	}
	return nil
}

func (c *Ctx) castAll(d *meta.DocType, doc Doc) error {
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] || f.Fieldtype == "Table" {
			continue
		}
		v, err := c.castValue(f, doc[f.Fieldname])
		if err != nil {
			return err
		}
		doc[f.Fieldname] = v
	}
	return nil
}

func (c *Ctx) fetchFrom(d *meta.DocType, doc Doc) error {
	for _, f := range d.Fields {
		if f.FetchFrom == "" {
			continue
		}
		parts := strings.SplitN(f.FetchFrom, ".", 2)
		lf := d.Field(parts[0])
		linkVal := doc.Str(parts[0])
		if linkVal == "" {
			if !f.ReadOnly {
				continue
			}
			doc[f.Fieldname] = nil
			continue
		}
		target := lf.OptionsString()
		if lf.Fieldtype == "Dynamic Link" {
			target = doc.Str(lf.OptionsString())
		}
		if target == "" {
			continue
		}
		// editable fetched fields keep a user value when already set
		if !f.ReadOnly && !isEmpty(doc[f.Fieldname]) {
			continue
		}
		v, err := c.GetValue(target, linkVal, parts[1])
		if err != nil {
			return err
		}
		doc[f.Fieldname] = v
	}
	return nil
}

func (c *Ctx) checkMandatory(d *meta.DocType, doc Doc) error {
	var missing []string
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		req := f.Reqd
		if !req && f.MandatoryDependsOn != "" {
			ok, err := c.evalExpr(f.MandatoryDependsOn, doc)
			if err != nil {
				return err
			}
			req = ok
		}
		if f.Fieldtype == "Vault" {
			if req && isEmpty(doc[f.Fieldname]) {
				if doc.ID() != "" {
					key := c.DeriveVaultKey(d, f, doc)
					has, _ := c.E.VaultHas(c.Ctx, c.Q(), key)
					if has {
						continue
					}
				}
				missing = append(missing, c.T(f.Label))
			}
			continue
		}
		if req && isEmpty(doc[f.Fieldname]) {
			missing = append(missing, c.T(f.Label))
		}
	}
	if len(missing) > 0 {
		return cerr.Mandatory("Fill in the required fields: {0}", strings.Join(missing, ", ")).WithTitleKey("Required fields")
	}
	return nil
}

// checkReadOnlyDependsOn enforces readOnlyDependsOn on the server: hiding the
// field on screen is not authorization. The expression is evaluated against the
// saved document — the state the user saw — and applies only to external changes;
// what controllers calculate afterwards remains permitted.
func (c *Ctx) checkReadOnlyDependsOn(d *meta.DocType, doc, before Doc) error {
	if before == nil {
		return nil
	}
	for _, f := range d.Fields {
		if f.Fieldname == "" || f.ReadOnlyDependsOn == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		ro, err := c.evalExpr(f.ReadOnlyDependsOn, before)
		if err != nil {
			return err
		}
		if !ro {
			continue
		}
		if f.Fieldtype == "Table" {
			if string(mustJSON(stripChildMeta(before.Children(f.Fieldname)))) == string(mustJSON(stripChildMeta(doc.Children(f.Fieldname)))) {
				continue
			}
		} else {
			nv, _ := c.castValue(f, doc[f.Fieldname])
			ov, _ := c.castValue(f, before[f.Fieldname])
			if db.Str(nv) == db.Str(ov) {
				continue
			}
		}
		return cerr.Validation("{0} is read-only on this document", c.T(f.Label)).WithTitleKey("Read-only field")
	}
	return nil
}

func (c *Ctx) evalExpr(expr string, doc Doc) (bool, error) {
	rt, err := c.RT()
	if err != nil {
		return false, err
	}
	return rt.EvalExpr(expr, doc.JSON())
}

func (c *Ctx) checkSelect(d *meta.DocType, doc Doc) error {
	for _, f := range d.Fields {
		if f.Fieldtype != "Select" || isEmpty(doc[f.Fieldname]) {
			continue
		}
		v := doc.Str(f.Fieldname)
		ok := false
		for _, o := range f.SelectValues() {
			if o == v {
				ok = true
				break
			}
		}
		if !ok {
			return cerr.Validation("{0}: value \"{1}\" is not one of the options", c.T(f.Label), v)
		}
	}
	return nil
}

func (c *Ctx) checkLinks(d *meta.DocType, doc Doc) error {
	for _, f := range d.Fields {
		v := doc.Str(f.Fieldname)
		if v == "" {
			continue
		}
		switch f.Fieldtype {
		case "Link":
			if ok, err := c.idExists(f.OptionsString(), v); err != nil {
				return err
			} else if !ok {
				return cerr.LinkExists("{0}: {1} \"{2}\" does not exist", c.T(f.Label), f.OptionsString(), v).WithTitleKey("Invalid link")
			}
		case "Dynamic Link":
			target := doc.Str(f.OptionsString())
			if target == "" {
				return cerr.Validation("{0}: choose the type before the link", c.T(f.Label))
			}
			if _, err := c.St.DocType(target); err != nil {
				return cerr.Validation("{0}: DocType \"{1}\" does not exist", c.T(f.Label), target)
			}
			if ok, err := c.idExists(target, v); err != nil {
				return err
			} else if !ok {
				return cerr.LinkExists("{0}: {1} \"{2}\" does not exist", c.T(f.Label), target, v).WithTitleKey("Invalid link")
			}
		}
	}
	return nil
}

func (c *Ctx) checkUnique(d *meta.DocType, doc Doc) error {
	for _, f := range d.DataFields() {
		if !f.Unique || isEmpty(doc[f.Fieldname]) {
			continue
		}
		rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT id FROM %s WHERE %s = $1 AND id <> $2 LIMIT 1", db.Ident(d.TableName()), db.Ident(f.Fieldname)), doc[f.Fieldname], doc.ID())
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return cerr.Duplicate("{0} \"{1}\" already exists on {2} {3}", c.T(f.Label), doc.Str(f.Fieldname), c.T(d.Label), rows[0]["id"]).WithTitleKey("Duplicate value")
		}
	}
	return nil
}

// checkUniqueKeys is the readable half of a compound business key: a SELECT
// before the write, so an ordinary collision can name the document already
// holding the key. The index behind it is the half that actually holds — a
// SELECT cannot see a row a concurrent transaction has not committed yet, so
// two writers can both pass here and only one can pass the index. See
// duplicateErr for what the loser is told.
func (c *Ctx) checkUniqueKeys(d *meta.DocType, doc Doc) error {
	for _, k := range d.UniqueKeys {
		where := make([]string, 0, len(k.Fields))
		args := make([]any, 0, len(k.Fields)+1)
		for _, fn := range k.Fields {
			f := d.Field(fn)
			if f == nil || outsideKey(f, doc[fn]) {
				// The index leaves this row out, so nothing is being claimed:
				// an incomplete key is an unfinished document, not a duplicate.
				where = nil
				break
			}
			args = append(args, doc[fn])
			where = append(where, fmt.Sprintf("%s = $%d", db.Ident(fn), len(args)))
		}
		if len(where) == 0 {
			continue
		}
		args = append(args, doc.ID())
		rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT id FROM %s WHERE %s AND id <> $%d LIMIT 1",
			db.Ident(d.TableName()), strings.Join(where, " AND "), len(args)), args...)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return cerr.Duplicate("{0} already exists on {1} {2}",
				c.keyValues(d, k, doc), c.T(d.Label), rows[0]["id"]).WithTitleKey("Duplicate value")
		}
	}
	return nil
}

// outsideKey reports whether a value leaves its row out of the partial unique
// index — the very test the index predicate makes, and deliberately not
// isEmpty: `false` is a value to a boolean column, and calling it absent would
// let the pre-check wave through a row the index then refuses.
func outsideKey(f *meta.Field, v any) bool {
	if v == nil {
		return true
	}
	return meta.ColumnType(f.Fieldtype) == "text" && db.Str(v) == ""
}

// keyValues renders a compound key as the reader entered it — `Label "value"`
// per component — so the message names the business key and not the index.
// Each label goes through the catalogue, the way a label always does.
func (c *Ctx) keyValues(d *meta.DocType, k meta.UniqueKey, doc Doc) string {
	parts := make([]string, 0, len(k.Fields))
	for _, fn := range k.Fields {
		label := fn
		if f := d.Field(fn); f != nil && f.Label != "" {
			label = f.Label
		}
		parts = append(parts, fmt.Sprintf("%s \"%s\"", c.T(label), doc.Str(fn)))
	}
	return strings.Join(parts, ", ")
}

// duplicateErr turns a unique-index violation into the same sentence the
// pre-checks write. This is the path a race takes: both writers passed
// validation, and the index refused the second one.
//
// It cannot name the document already holding the key. A 23505 aborts the
// transaction, so there is no query left to ask with — which is why this
// message stops at the value where checkUniqueKeys goes on to say where.
func (c *Ctx) duplicateErr(d *meta.DocType, doc Doc, err error) error {
	idx, ok := db.UniqueViolation(err)
	if !ok {
		return err
	}
	suffix := strings.TrimPrefix(idx, d.TableName()+"_")
	switch {
	case suffix == idx:
		// An index on another table, or one whose name this DocType does not
		// own: the collision is real, its name is simply not ours to read.
	case suffix == "pkey":
		return cerr.Duplicate("{0} {1} already exists", c.T(d.Label), doc.ID()).WithTitleKey("Duplicate ID")
	case strings.HasPrefix(suffix, "uk_"):
		if k := d.UniqueKey(strings.TrimPrefix(suffix, "uk_")); k != nil {
			return cerr.Duplicate("{0} already exists", c.keyValues(d, *k, doc)).WithTitleKey("Duplicate value")
		}
	default:
		if f := d.Field(suffix); f != nil {
			return cerr.Duplicate("{0} \"{1}\" already exists", c.T(f.Label), doc.Str(suffix)).WithTitleKey("Duplicate value")
		}
	}
	return cerr.Duplicate("Duplicate value in {0}", c.T(d.Label)).WithTitleKey("Duplicate value")
}

func (c *Ctx) validateChildren(d *meta.DocType, doc Doc, opts SaveOpts) error {
	for _, tf := range d.TableFields() {
		child, _ := c.St.DocType(tf.OptionsString())
		rows := doc.Children(tf.Fieldname)
		list := make([]any, 0, len(rows))
		for i, row := range rows {
			row["doctype"], row["parenttype"], row["parentfield"], row["idx"] = child.Name, d.Name, tf.Fieldname, int64(i+1)
			row["parent"] = doc["id"]
			row["docstatus"] = doc["docstatus"]
			if err := c.castAll(child, row); err != nil {
				return err
			}
			if err := c.fetchFrom(child, row); err != nil {
				return err
			}
			if err := c.checkMandatory(child, row); err != nil {
				return cerr.Validation("{0}, row {1}: {2}", c.T(tf.Label), i+1, cerr.From(err).Message).WithTitleKey("Required fields")
			}
			if err := c.checkSelect(child, row); err != nil {
				return err
			}
			if !opts.IgnoreLinks {
				if err := c.checkLinks(child, row); err != nil {
					return err
				}
			}
			list = append(list, map[string]any(row))
		}
		doc[tf.Fieldname] = list
	}
	return nil
}

func (c *Ctx) checkAllowOnSubmit(d *meta.DocType, before, doc Doc) error {
	var exempt map[string]bool
	if c.inWorkflowTransition {
		if wf := c.WorkflowFor(d.Name); wf != nil {
			exempt = map[string]bool{wf.StateField: true}
			if st := wf.FindState(doc.Str(wf.StateField)); st != nil {
				for k := range st.UpdateFields {
					exempt[k] = true
				}
			}
		}
	}
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] || f.AllowOnSubmit || exempt[f.Fieldname] {
			continue
		}
		if f.Fieldtype == "Table" {
			if string(mustJSON(stripChildMeta(before.Children(f.Fieldname)))) != string(mustJSON(stripChildMeta(doc.Children(f.Fieldname)))) {
				return cerr.Validation("{0} cannot be changed after submission", c.T(f.Label)).WithTitleKey("Submitted document")
			}
			continue
		}
		nv, _ := c.castValue(f, doc[f.Fieldname])
		ov, _ := c.castValue(f, before[f.Fieldname])
		if db.Str(nv) != db.Str(ov) && !(f.Fieldtype == "Datetime" && sameTime(nv, ov, c.E.Location())) {
			return cerr.Validation("{0} cannot be changed after submission", c.T(f.Label)).WithTitleKey("Submitted document")
		}
	}
	return nil
}

func stripChildMeta(rows []Doc) []Doc {
	out := make([]Doc, len(rows))
	for i, r := range rows {
		x := r.Clone()
		for _, k := range []string{"modified", "modified_by", "creation", "owner", "docstatus", "__islocal", "__unsaved", "id", "parent"} {
			delete(x, k)
		}
		out[i] = x
	}
	return out
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func (c *Ctx) checkLinksBeforeDelete(d *meta.DocType, name string) error {
	for _, other := range c.St.Meta.DocTypes {
		for _, f := range other.Fields {
			var sql string
			var args []any
			switch {
			case f.Fieldtype == "Link" && f.OptionsString() == d.Name:
				sql = fmt.Sprintf("SELECT id, parent, parenttype FROM %s WHERE %s = $1 LIMIT 1", db.Ident(other.TableName()), db.Ident(f.Fieldname))
				args = []any{name}
			case f.Fieldtype == "Dynamic Link" && other.Field(f.OptionsString()) != nil:
				sql = fmt.Sprintf("SELECT id, parent, parenttype FROM %s WHERE %s = $1 AND %s = $2 LIMIT 1", db.Ident(other.TableName()), db.Ident(f.Fieldname), db.Ident(f.OptionsString()))
				args = []any{name, d.Name}
			default:
				continue
			}
			if !other.IsChild {
				sql = strings.Replace(sql, "id, parent, parenttype", "id, NULL AS parent, NULL AS parenttype", 1)
			}
			rows, err := db.Select(c.Ctx, c.Q(), sql, args...)
			if err != nil {
				return err
			}
			if len(rows) > 0 {
				ref, refName := other.Name, db.Str(rows[0]["id"])
				if other.IsChild {
					ref, refName = db.Str(rows[0]["parenttype"]), db.Str(rows[0]["parent"])
				}
				return cerr.LinkExists("{0} {1} is linked from {2} {3}", c.T(d.Label), name, ref, refName).WithTitleKey("Cannot delete")
			}
		}
	}
	return nil
}

// ------------------------------------------------------------ write

func (c *Ctx) columnValues(d *meta.DocType, doc Doc) ([]string, []any, error) {
	if d.IsSingle && (doc.ID() != "singleton" || doc.Docstatus() != 0) {
		return nil, nil, cerr.Validation("Invalid Single identity or status for {0}", d.Name)
	}
	var cols []string
	var vals []any
	add := func(k string, v any) { cols = append(cols, db.Ident(k)); vals = append(vals, v) }
	add("id", doc["id"])
	add("owner", doc["owner"])
	add("creation", parseTimeOrNow(doc["creation"], c.E.Location()))
	add("modified", parseTimeOrNow(doc["modified"], c.E.Location()))
	add("modified_by", doc["modified_by"])
	add("docstatus", int64(doc.Docstatus()))
	if d.IsChild {
		add("parent", doc["parent"])
		add("parenttype", doc["parenttype"])
		add("parentfield", doc["parentfield"])
		add("idx", int64(toFloat(doc["idx"])))
	}
	for _, f := range d.DataFields() {
		v, err := c.castValue(f, doc[f.Fieldname])
		if err != nil {
			return nil, nil, err
		}
		add(f.Fieldname, v)
	}
	return cols, vals, nil
}

func parseTimeOrNow(v any, loc *time.Location) time.Time {
	if t := parseTime(v, loc); !t.IsZero() {
		return t
	}
	return time.Now()
}

func (c *Ctx) writeInsert(d *meta.DocType, doc Doc) error {
	cols, vals, err := c.columnValues(d, doc)
	if err != nil {
		return err
	}
	ph := make([]string, len(vals))
	for i := range vals {
		ph[i] = fmt.Sprintf("$%d", i+1)
	}
	_, err = c.Q().Exec(c.Ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", db.Ident(d.TableName()), strings.Join(cols, ", "), strings.Join(ph, ", ")), vals...)
	if err != nil {
		return c.duplicateErr(d, doc, err)
	}
	return nil
}

// writeUpdate saves the document with compare-and-swap using the `modified` timestamp
// read at the beginning of Save: if another transaction wrote in the meantime, no row
// is affected and an error on timestamp occurs, never a silent overwrite.
func (c *Ctx) writeUpdate(d *meta.DocType, doc Doc, prevModified any) error {
	cols, vals, err := c.columnValues(d, doc)
	if err != nil {
		return err
	}
	var sets []string
	var args []any
	for i, col := range cols {
		if col == `"id"` {
			continue
		}
		args = append(args, vals[i])
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	args = append(args, doc["id"])
	sql := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d", db.Ident(d.TableName()), strings.Join(sets, ", "), len(args))
	args = append(args, parseTimeOrNil(prevModified, c.E.Location()))
	sql += fmt.Sprintf(" AND modified IS NOT DISTINCT FROM $%d", len(args))
	tag, err := c.Q().Exec(c.Ctx, sql, args...)
	if err != nil {
		return c.duplicateErr(d, doc, err)
	}
	if tag.RowsAffected() == 0 {
		return cerr.Timestamp("{0} {1} was changed by someone else. Reload and try again.", c.T(d.Label), doc.ID())
	}
	return nil
}

func parseTimeOrNil(v any, loc *time.Location) any {
	if v == nil {
		return nil
	}
	if t := parseTime(v, loc); !t.IsZero() {
		return t
	}
	return nil
}

// childIDs lists the rows already belonging to (parent, parenttype, parentfield).
func (c *Ctx) childIDs(child *meta.DocType, parent, parenttype, parentfield string) (map[string]bool, error) {
	out := map[string]bool{}
	if parent == "" {
		return out, nil
	}
	rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT id FROM %s WHERE parent = $1 AND parenttype = $2 AND parentfield = $3", db.Ident(child.TableName())), parent, parenttype, parentfield)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[db.Str(r["id"])] = true
	}
	return out, nil
}

func (c *Ctx) childExists(child *meta.DocType, name string) (bool, error) {
	rows, err := db.Select(c.Ctx, c.Q(), fmt.Sprintf("SELECT 1 FROM %s WHERE id = $1", db.Ident(child.TableName())), name)
	return len(rows) > 0, err
}

func (c *Ctx) writeChildren(d *meta.DocType, doc Doc) error {
	for _, tf := range d.TableFields() {
		child, _ := c.St.DocType(tf.OptionsString())
		rows := doc.Children(tf.Fieldname)
		keep := make([]any, 0, len(rows))
		now := time.Now()
		owned, err := c.childIDs(child, doc.Str("id"), d.Name, tf.Fieldname)
		if err != nil {
			return err
		}
		for i, row := range rows {
			// A row only retains its received `name` if it already belongs to this
			// parent/field. Otherwise it becomes a copy: without this, saving a
			// document would hijack another document's child row (B03).
			if n := row.Str("id"); n != "" && !owned[n] {
				taken, err := c.childExists(child, n)
				if err != nil {
					return err
				}
				if taken {
					row["id"] = randomID()
					row["creation"], row["owner"] = now, c.User
				}
			}
			if row.Str("id") == "" {
				row["id"] = randomID()
				row["creation"], row["owner"] = now, c.User
			}
			if row["creation"] == nil {
				row["creation"] = now
			}
			row["modified"], row["modified_by"] = now, c.User
			row["idx"] = int64(i + 1)
			row["docstatus"] = doc["docstatus"]
			keep = append(keep, row["id"])
			cols, vals, err := c.columnValues(child, row)
			if err != nil {
				return err
			}
			ph := make([]string, len(vals))
			var sets []string
			for j, col := range cols {
				ph[j] = fmt.Sprintf("$%d", j+1)
				if col != `"id"` && col != `"creation"` && col != `"owner"` {
					sets = append(sets, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
				}
			}
			sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (id) DO UPDATE SET %s",
				db.Ident(child.TableName()), strings.Join(cols, ", "), strings.Join(ph, ", "), strings.Join(sets, ", "))
			if _, err := c.Q().Exec(c.Ctx, sql, vals...); err != nil {
				return fmt.Errorf("%w\nSQL: %s", err, sql)
			}
			delete(row, "__islocal")
		}
		var b db.Builder
		var ph []string
		for _, n := range keep {
			ph = append(ph, b.Arg(n))
		}
		sql := fmt.Sprintf("DELETE FROM %s WHERE parent = %s AND parenttype = %s AND parentfield = %s", db.Ident(child.TableName()), b.Arg(doc["id"]), b.Arg(d.Name), b.Arg(tf.Fieldname))
		if len(ph) > 0 {
			sql += " AND id NOT IN (" + strings.Join(ph, ", ") + ")"
		}
		if _, err := c.Q().Exec(c.Ctx, sql, b.Args...); err != nil {
			return fmt.Errorf("%w\nSQL: %s %v", err, sql, b.Args)
		}
	}
	return nil
}

// isSecretField marks columns that never belong in a version diff, even
// when declared as Data.
func isSecretField(name string) bool {
	switch name {
	case "password_hash", "new_password", "password", "api_secret", "secret":
		return true
	}
	return strings.HasSuffix(name, "_password") || strings.HasSuffix(name, "_secret")
}

// ------------------------------------------------------------ versions

// versionRows copies child rows for a Version diff, without secret columns.
func versionRows(cd *meta.DocType, rows []Doc) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		x := map[string]any{}
		for k, v := range r {
			if cd != nil {
				if f := cd.Field(k); f != nil && (f.Fieldtype == "Password" || f.Fieldtype == "Vault") {
					continue
				}
			}
			if isSecretField(k) {
				continue
			}
			x[k] = v
		}
		out[i] = x
	}
	return out
}

func (c *Ctx) saveVersion(d *meta.DocType, before, after Doc) {
	changed := map[string][]any{}
	for _, f := range d.Fields {
		if f.Fieldname == "" || meta.LayoutTypes[f.Fieldtype] {
			continue
		}
		// secrets do not enter version history: Version is readable by anyone permitted to read
		// the document, and the diff would leak the password/hash (B04)
		if f.Fieldtype == "Password" || f.Fieldtype == "Vault" || isSecretField(f.Fieldname) {
			continue
		}
		var a, b any = before[f.Fieldname], after[f.Fieldname]
		if f.Fieldtype == "Table" {
			a, b = stripChildMeta(before.Children(f.Fieldname)), stripChildMeta(after.Children(f.Fieldname))
		}
		if string(mustJSON(a)) != string(mustJSON(b)) {
			if f.Fieldtype == "Table" {
				// a child row's secrets stay out of history just as the parent's do
				cd, _ := c.St.DocType(f.OptionsString())
				changed[f.Fieldname] = []any{versionRows(cd, before.Children(f.Fieldname)), versionRows(cd, after.Children(f.Fieldname))}
				continue
			}
			changed[f.Fieldname] = []any{before[f.Fieldname], after[f.Fieldname]}
		}
	}
	if before.Docstatus() != after.Docstatus() {
		changed["docstatus"] = []any{before["docstatus"], after["docstatus"]}
	}
	if len(changed) == 0 {
		return
	}
	data := mustJSON(map[string]any{"changed": changed})
	c.Q().Exec(c.Ctx, `INSERT INTO tab_version (id, owner, creation, modified, modified_by, docstatus, ref_doctype, doc_id, data)
		VALUES ($1, $2, now(), now(), $2, 0, $3, $4, $5)`, randomID(), c.User, d.Name, after.ID(), string(data))
}
