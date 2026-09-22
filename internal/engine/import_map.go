package engine

// The mapping an import may be given (DAT-01): a JSON file that renames
// DocTypes and fields, drops what the target does not have, sets constants,
// remaps values, ids and users, and says what to do about a document that is
// already there.
//
// It is data, not code: a migration is rehearsed many times, and a mapping that
// can be diffed between rehearsals is worth more than a script that cannot.

import (
	"bytes"
	"encoding/json"

	"github.com/jrvidotti/ddcore/internal/db"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// OnExisting is what to do when the target already holds the document.
type OnExisting string

const (
	// OnExistingIdentical skips a document whose field values already match,
	// and reports a conflict when they do not. It is the default because
	// `migrate` seeds Roles, Admin and Guest before any import runs, and a
	// full export carries those same rows.
	OnExistingIdentical OnExisting = "identical"
	// OnExistingSkip leaves whatever is there, whatever it holds.
	OnExistingSkip OnExisting = "skip"
	// OnExistingError refuses every collision.
	OnExistingError OnExisting = "error"
)

// ImportMap is the parsed mapping file.
type ImportMap struct {
	// IgnoreUnknown reads a source field the target DocType does not declare
	// as nothing, instead of as a record error.
	IgnoreUnknown bool                     `json:"ignoreUnknown"`
	Users         map[string]string        `json:"users"`
	Exclude       []string                 `json:"exclude"`
	Doctypes      map[string]*ImportDocMap `json:"doctypes"`
}

// ImportDocMap is one DocType's rules; Children carries the same rules for a
// child table, keyed by the source fieldname.
type ImportDocMap struct {
	To         string                    `json:"to"`
	Skip       bool                      `json:"skip"`
	Fields     map[string]*string        `json:"fields"` // null or "" drops the field
	Set        map[string]any            `json:"set"`
	Values     map[string]map[string]any `json:"values"`
	IDs        map[string]string         `json:"ids"`
	OnExisting OnExisting                `json:"onExisting"`
	Children   map[string]*ImportDocMap  `json:"children"`
}

// ParseImportMap reads a mapping file.
func ParseImportMap(b []byte) (*ImportMap, error) {
	var m ImportMap
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, cerr.Validation("The mapping is not readable: {0}", err)
	}
	for name, dm := range m.Doctypes {
		if err := checkOnExisting(name, dm); err != nil {
			return nil, err
		}
	}
	return &m, nil
}

func checkOnExisting(name string, dm *ImportDocMap) error {
	switch dm.OnExisting {
	case "", OnExistingIdentical, OnExistingSkip, OnExistingError:
	default:
		return cerr.Validation("{0}: onExisting is {1}; it is one of identical, skip or error", name, dm.OnExisting)
	}
	for field, cm := range dm.Children {
		if err := checkOnExisting(name+"."+field, cm); err != nil {
			return err
		}
	}
	return nil
}

func (m *ImportMap) doctype(source string) *ImportDocMap {
	if m == nil {
		return nil
	}
	return m.Doctypes[source]
}

// Target is what the source DocType is called on this site.
func (m *ImportMap) Target(source string) string {
	if dm := m.doctype(source); dm != nil && dm.To != "" {
		return dm.To
	}
	return source
}

// Excluded reports whether the source DocType is left out of the load.
func (m *ImportMap) Excluded(source string) bool {
	if m == nil {
		return false
	}
	for _, n := range m.Exclude {
		if n == source {
			return true
		}
	}
	if dm := m.doctype(source); dm != nil && dm.Skip {
		return true
	}
	return false
}

// OnExisting is the collision policy for one source DocType.
func (m *ImportMap) OnExisting(source string) OnExisting {
	if dm := m.doctype(source); dm != nil && dm.OnExisting != "" {
		return dm.OnExisting
	}
	return OnExistingIdentical
}

// RemapID is the id a source document has on this site. Link fields, Dynamic
// Links and the reference columns go through it too, so a remapped id stays
// consistent wherever it is mentioned.
func (m *ImportMap) RemapID(source, id string) string {
	if dm := m.doctype(source); dm != nil {
		if to, ok := dm.IDs[id]; ok {
			return to
		}
	}
	return id
}

// RemapUser is the same for a user: `owner`, `modified_by` and every Link to
// User.
func (m *ImportMap) RemapUser(user string) string {
	if m == nil {
		return user
	}
	if to, ok := m.Users[user]; ok {
		return to
	}
	return user
}

// Apply returns the target DocType and a new document with the mapping applied.
// The source document is left as it was: a record that fails is replayed from
// the same line in careful mode.
func (m *ImportMap) Apply(source string, doc Doc) (string, Doc) {
	target := m.Target(source)
	out := applyDocMap(m, m.doctype(source), source, doc)
	if target != "" {
		out["doctype"] = target
	}
	return target, out
}

func applyDocMap(m *ImportMap, dm *ImportDocMap, source string, doc Doc) Doc {
	out := make(Doc, len(doc))
	children := map[string]*ImportDocMap{}
	if dm != nil {
		children = dm.Children
	}
	for k, v := range doc {
		field := k
		if dm != nil {
			if to, ok := dm.Fields[k]; ok {
				if to == nil || *to == "" {
					continue
				}
				field = *to
			}
		}
		if rows, ok := v.([]any); ok {
			if cm, mapped := children[k]; mapped || isChildRows(rows) {
				if mapped && cm.To != "" {
					field = cm.To
				}
				out[field] = applyChildRows(m, cm, rows)
				continue
			}
		}
		out[field] = v
	}
	if dm != nil {
		for k, v := range dm.Set {
			out[k] = v
		}
		for field, values := range dm.Values {
			if cur, ok := out[field]; ok {
				if to, hit := values[db.Str(cur)]; hit {
					out[field] = to
				}
			}
		}
		if id, ok := out["id"].(string); ok {
			out["id"] = m.RemapID(source, id)
		}
	}
	for _, field := range []string{"owner", "modified_by"} {
		if u, ok := out[field].(string); ok {
			out[field] = m.RemapUser(u)
		}
	}
	return out
}

func applyChildRows(m *ImportMap, cm *ImportDocMap, rows []any) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		child, ok := row.(map[string]any)
		if !ok {
			out = append(out, row)
			continue
		}
		source := db.Str(child["doctype"])
		mapped := applyDocMap(m, cm, source, Doc(child))
		if cm != nil && cm.To != "" {
			// A child table's `to` renames the parent's field, not the child
			// DocType; its own name comes from the target's meta.
			delete(mapped, "doctype")
		}
		out = append(out, map[string]any(mapped))
	}
	return out
}

// isChildRows reports whether a value looks like a child table rather than a
// JSON field that happens to hold a list.
func isChildRows(rows []any) bool {
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			return false
		}
		if _, has := m["doctype"]; !has {
			if _, has := m["parentfield"]; !has {
				return false
			}
		}
	}
	return len(rows) > 0
}
