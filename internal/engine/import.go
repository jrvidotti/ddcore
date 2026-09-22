package engine

// The restricted write path an import uses (DAT-01).
//
// Insert is the wrong tool for history. It stamps owner, creation and modified
// with now and whoever is running, regenerates a series id over the one the
// document already has, runs the app's hooks, and then queues the webhooks,
// notifications and SSE that tell the world something just happened. None of
// that is true of a document that was created three years ago on another
// system: its effects happened then, and replaying them is the one thing a
// migration must not do.
//
// So ImportDoc keeps what it is given and writes it. What it does not skip is
// the structural half: casting, the mandatory fields, the Select options, the
// unique keys and the links it can check here — a load that puts unreadable
// rows in the database has only moved the problem.
//
// It can forge owner and creation, so it is Go-only: it is never reachable
// from an app's TypeScript, and it answers only to whoever administers the
// site.

import (
	"sort"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// ImportOpts steers one record's load.
type ImportOpts struct {
	// Deferred says a link is checked after the load rather than now, asked
	// with the DocType name and the fieldname. ImportOrder decides which.
	Deferred func(doctype, fieldname string) bool
}

// ImportResult is what one loaded record has to say for itself.
type ImportResult struct {
	Doctype string
	ID      string
	// Notes are findings that do not stop the record: a secret the export
	// never carried, an id that moved no counter.
	Notes []string
}

// ImportDoc writes one document as it was given, with no hooks and no effects.
func (c *Ctx) ImportDoc(doc Doc, opts ImportOpts) (*ImportResult, error) {
	d, err := c.St.DocType(doc.DocType())
	if err != nil {
		return nil, err
	}
	if err := c.checkWritable(d.Name); err != nil {
		return nil, err
	}
	if err := c.canImport(); err != nil {
		return nil, err
	}
	switch {
	case d.IsChild:
		return nil, cerr.Validation("{0} is a child table; its rows travel inside their parent", d.Name)
	case d.IsSingle:
		return nil, cerr.Validation("{0} is a Single; an export never carries one", d.Name)
	case d.Name == "Audit Event":
		return nil, cerr.Permission("Audit Event records are immutable and cannot be imported")
	}
	if doc.ID() == "" {
		return nil, cerr.Validation("{0}: the record has no id; an import keeps the identity it is given", d.Name)
	}
	if ds := doc.Docstatus(); ds != 0 && !d.Submittable {
		return nil, cerr.Validation("{0} {1}: docstatus {2} on a DocType that is not submittable", d.Name, doc.ID(), ds)
	} else if ds < 0 || ds > 2 {
		return nil, cerr.Validation("{0} {1}: invalid docstatus {2}", d.Name, doc.ID(), ds)
	}
	res := &ImportResult{Doctype: d.Name, ID: doc.ID()}

	// Metadata the export did not carry falls back to this run, which is the
	// honest answer: nobody else wrote those rows.
	now := time.Now()
	if doc["owner"] == nil || doc.Str("owner") == "" {
		doc["owner"] = c.User
	}
	if doc["modified_by"] == nil || doc.Str("modified_by") == "" {
		doc["modified_by"] = c.User
	}
	if doc["creation"] == nil {
		doc["creation"] = now
	}
	if doc["modified"] == nil {
		doc["modified"] = doc["creation"]
	}
	doc["docstatus"] = int64(doc.Docstatus())

	sortChildrenByIdx(c, d, doc)

	checks := fieldChecks{skipFetch: true, skipSecretMandatory: true}
	if opts.Deferred != nil {
		checks.deferLink = func(fd *meta.DocType, fieldname string) bool {
			// A child table's fields are asked under the parent's table
			// fieldname, which is how ImportOrder names them.
			if fd.IsChild {
				return opts.Deferred(fd.Name, db.Str(doc["__parentfield"])+"."+fieldname) ||
					opts.Deferred(d.Name, db.Str(doc["__parentfield"])+"."+fieldname)
			}
			return opts.Deferred(fd.Name, fieldname)
		}
	}
	res.Notes = append(res.Notes, secretNotes(c, d)...)
	if err := c.validateFields(d, doc, SaveOpts{IgnorePermissions: true}, checks); err != nil {
		return nil, err
	}
	if err := c.writeInsert(d, doc); err != nil {
		return nil, err
	}
	if err := c.insertChildrenAsIs(d, doc); err != nil {
		return nil, err
	}
	if err := c.advanceSeries(d, doc); err != nil {
		return nil, err
	}
	if err := c.markNotificationsDone(d, doc, now); err != nil {
		return nil, err
	}
	return res, nil
}

// canImport keeps the path with whoever administers the site. A scoped user is
// refused outright: scopes narrow what a user may see, and a path that writes
// owner and creation directly has no business honouring them halfway.
func (c *Ctx) canImport() error {
	if c.User == "Admin" {
		return nil
	}
	if !c.HasRole("System Manager") {
		return cerr.Permission("Importing is limited to Admin and System Manager")
	}
	perms, err := c.UserPermissions()
	if err != nil {
		return err
	}
	if len(perms) > 0 {
		return cerr.Permission("A user with access scopes cannot import")
	}
	return nil
}

// secretNotes reports the fields an export can never carry, so the run's report
// says what has to be set again by hand rather than leaving it to be noticed
// later.
func secretNotes(c *Ctx, d *meta.DocType) []string {
	var notes []string
	for _, f := range d.Fields {
		if f.Fieldtype != "Vault" && f.Fieldtype != "Password" {
			continue
		}
		notes = append(notes, c.T("{0} is not migrated: a {1} field never leaves its site", f.Label, f.Fieldtype))
	}
	return notes
}

// sortChildrenByIdx puts each child table in the order its rows say they are
// in. validateChildren renumbers idx from the row's position, so without this
// a line whose rows are not already in order would come out reordered. Gaps in
// a legacy idx are closed: the order is what carries meaning, not the number.
func sortChildrenByIdx(c *Ctx, d *meta.DocType, doc Doc) {
	for _, tf := range d.TableFields() {
		rows := doc.Children(tf.Fieldname)
		if len(rows) < 2 {
			continue
		}
		sort.SliceStable(rows, func(i, j int) bool {
			return toFloat(rows[i]["idx"]) < toFloat(rows[j]["idx"])
		})
		list := make([]any, len(rows))
		for i, row := range rows {
			list[i] = map[string]any(row)
		}
		doc[tf.Fieldname] = list
	}
}

// insertChildrenAsIs writes the child rows with the ids, idx and metadata they
// came with. writeChildren cannot be used: it renames a row whose id is taken,
// stamps modified with now, and sweeps rows the document no longer has — all
// right for a save, all wrong for a load.
func (c *Ctx) insertChildrenAsIs(d *meta.DocType, doc Doc) error {
	for _, tf := range d.TableFields() {
		child, err := c.St.DocType(tf.OptionsString())
		if err != nil {
			return err
		}
		for _, row := range doc.Children(tf.Fieldname) {
			if row.Str("id") == "" {
				row["id"] = randomID()
			}
			if row["owner"] == nil || row.Str("owner") == "" {
				row["owner"] = doc["owner"]
			}
			if row["modified_by"] == nil || row.Str("modified_by") == "" {
				row["modified_by"] = doc["modified_by"]
			}
			if row["creation"] == nil {
				row["creation"] = doc["creation"]
			}
			if row["modified"] == nil {
				row["modified"] = doc["modified"]
			}
			if err := c.writeInsert(child, row); err != nil {
				return err
			}
		}
	}
	return nil
}

// advanceSeries moves ddcore_series past an id the load wrote, so the next
// document created here does not collide with what was just imported. An id
// that does not fit the DocType's rule — a legacy key kept as it was — moves
// nothing.
func (c *Ctx) advanceSeries(d *meta.DocType, doc Doc) error {
	key, n, ok := seriesCounterFor(d, doc)
	if !ok {
		return nil
	}
	_, err := c.Q().Exec(c.Ctx, `INSERT INTO ddcore_series (prefix, current) VALUES ($1, $2)
		ON CONFLICT (prefix) DO UPDATE SET current = GREATEST(ddcore_series.current, EXCLUDED.current)`, key, n)
	return err
}

// markNotificationsDone claims the date occurrences an imported document has
// already passed. Without it the next sweep reminds people of invoices that
// came due years ago: the sweep has no high-water mark, on purpose. A date
// still ahead is left alone — that reminder is genuinely owed.
func (c *Ctx) markNotificationsDone(d *meta.DocType, doc Doc, now time.Time) error {
	loc := c.E.Location()
	for _, rule := range c.St.Notifications {
		if rule.Date == nil || rule.Doctype != d.Name {
			continue
		}
		field := d.Field(rule.Date.Field)
		if field == nil {
			continue
		}
		due := notificationDue(doc[field.Fieldname], field.Fieldtype, rule.Date.Days, loc)
		if due.IsZero() || due.After(now) {
			continue
		}
		if _, err := c.Q().Exec(c.Ctx, `INSERT INTO ddcore_notification_due (rule, reference_doctype, reference_id, due)
			VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, rule.Name, d.Name, doc.ID(), due); err != nil {
			return err
		}
	}
	return nil
}
