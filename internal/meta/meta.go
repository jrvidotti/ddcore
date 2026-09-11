// Package meta holds the DocType model: what the TS `defineDoctype` produces,
// after being executed in the JS runtime and serialized to JSON.
package meta

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Layout fieldtypes have no column.
var LayoutTypes = map[string]bool{"Section Break": true, "Column Break": true, "Tab Break": true, "HTML": true}

// ColumnType maps a fieldtype to its Postgres column type ("" = no column).
func ColumnType(ft string) string {
	switch ft {
	case "Data", "Email", "Small Text", "Text", "Text Editor", "Select", "Link", "Dynamic Link", "Attach", "Password":
		return "text"
	case "Int":
		return "bigint"
	case "Float":
		return "double precision"
	case "Currency", "Percent":
		return "numeric(21,9)"
	case "Check":
		return "boolean"
	case "Date", "Month":
		return "date"
	case "Datetime":
		return "timestamptz"
	case "Time":
		return "time"
	case "JSON":
		return "jsonb"
	}
	return ""
}

var ValidFieldTypes = []string{"Data", "Email", "Small Text", "Text", "Text Editor", "Int", "Float", "Currency", "Percent", "Check", "Date", "Month", "Datetime", "Time", "Select", "Link", "Dynamic Link", "Table", "Attach", "JSON", "Password", "Section Break", "Column Break", "Tab Break", "HTML"}

type Field struct {
	Fieldname          string `json:"fieldname,omitempty"`
	Fieldtype          string `json:"fieldtype"`
	Label              string `json:"label,omitempty"`
	Options            any    `json:"options,omitempty"` // string (Link/Table/Dynamic Link) or []string (Select)
	Reqd               bool   `json:"reqd,omitempty"`
	Unique             bool   `json:"unique,omitempty"`
	Default            any    `json:"default,omitempty"`
	ReadOnly           bool   `json:"readOnly,omitempty"`
	Hidden             bool   `json:"hidden,omitempty"`
	FetchFrom          string `json:"fetchFrom,omitempty"`
	DependsOn          string `json:"dependsOn,omitempty"`
	ReadOnlyDependsOn  string `json:"readOnlyDependsOn,omitempty"`
	MandatoryDependsOn string `json:"mandatoryDependsOn,omitempty"`
	AllowOnSubmit      bool   `json:"allowOnSubmit,omitempty"`
	InListView         bool   `json:"inListView,omitempty"`
	InStandardFilter   bool   `json:"inStandardFilter,omitempty"`
	SearchIndex        bool   `json:"searchIndex,omitempty"`
	Length             int    `json:"length,omitempty"`
	Precision          int    `json:"precision,omitempty"`
	Description        string `json:"description,omitempty"`
	Columns            int    `json:"columns,omitempty"`
	GridEditMode       string `json:"gridEditMode,omitempty"`
	Collapsible        bool   `json:"collapsible,omitempty"`
	Bold               bool   `json:"bold,omitempty"`
	IgnoreUserPerms    bool   `json:"-"`
	// OptionColors maps a Select's canonical (English) value to an indicator
	// colour. Keyed by the value, never by its label, so it is
	// language-independent by construction.
	OptionColors map[string]string `json:"optionColors,omitempty"`
	// OptionLabels is the display text of each entry in Options, in the same
	// order. Options itself stays canonical English — it is what the database
	// holds.
	//
	// It is normally filled only on the translated copy the API serves. A field
	// that sets it up front is declaring itself *self-describing*: its options
	// are not catalogue keys, they are neither translated nor collected by the
	// extractor. User.language is the one that does this, with autonyms.
	OptionLabels []string `json:"optionLabels,omitempty"`
	// RenamedFrom is the fieldname this field used to have, so Plan can emit a
	// RENAME COLUMN instead of adding an empty column beside the old one. A
	// list carries a chain of renames, oldest first, for a database that
	// skipped a release.
	RenamedFrom Names `json:"renamedFrom,omitempty"`
	// Convert authorises a column-type change Plan would otherwise refuse,
	// naming the fieldtype the column still holds. It carries no SQL: a
	// conversion a plain cast cannot express is what a patch is for.
	Convert *Convert `json:"convert,omitempty"`
	// App is the app whose catalogue owns this field's text. It is empty on a
	// field the DocType declares itself, and carries the extending app's name
	// on a field added — or whose label, description or Select options were
	// overridden — by extendDoctype. The i18n extractor keys on it: a label
	// written in one app must not become a missing key in another's CSV.
	App           string   `json:"app,omitempty"`
	_             struct{} // keep JSON tags exhaustive
	SelectOptions []string `json:"-"`
}

// Convert names the fieldtype whose column type the database still has. Naming
// it — rather than a bare "yes" — is what keeps the declaration from quietly
// authorising a different conversion two releases later.
type Convert struct {
	From string `json:"from"`
}

// Names is a `string | string[]` coming from TypeScript. A rename is written
// as one name in the common case and as a chain when a field was renamed more
// than once, so both spellings have to parse.
type Names []string

func (n *Names) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		if one == "" {
			*n = nil
		} else {
			*n = Names{one}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("renamedFrom must be a string or a list of strings: %w", err)
	}
	*n = many
	return nil
}

func (f *Field) OptionsString() string {
	if s, ok := f.Options.(string); ok {
		return s
	}
	return ""
}

type Perm struct {
	Role    string `json:"role"`
	Read    bool   `json:"read,omitempty"`
	Write   bool   `json:"write,omitempty"`
	Create  bool   `json:"create,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
	Submit  bool   `json:"submit,omitempty"`
	Cancel  bool   `json:"cancel,omitempty"`
	Amend   bool   `json:"amend,omitempty"`
	Report  bool   `json:"report,omitempty"`
	Export  bool   `json:"export,omitempty"`
	IfOwner bool   `json:"ifOwner,omitempty"`
}

func (p Perm) Has(ptype string) bool {
	switch ptype {
	case "read":
		return p.Read
	case "write":
		return p.Write
	case "create":
		return p.Create
	case "delete":
		return p.Delete
	case "submit":
		return p.Submit
	case "cancel":
		return p.Cancel
	case "amend":
		return p.Amend
	case "report":
		return p.Report
	case "export":
		return p.Export
	}
	return false
}

type Naming struct {
	Series string `json:"series,omitempty"`
	Field  string `json:"field,omitempty"`
	Hash   bool   `json:"hash,omitempty"`
	Prompt bool   `json:"prompt,omitempty"`
	Format string `json:"format,omitempty"`
}

type DocType struct {
	Name         string   `json:"name"`
	App          string   `json:"app"`
	Module       string   `json:"module,omitempty"`
	Label        string   `json:"label,omitempty"`
	Naming       Naming   `json:"naming"`
	Submittable  bool     `json:"submittable,omitempty"`
	IsChild      bool     `json:"isChild,omitempty"`
	IsSingle     bool     `json:"isSingle,omitempty"`
	TrackChanges bool     `json:"trackChanges,omitempty"`
	AllowRename  bool     `json:"allowRename,omitempty"`
	TitleField   string   `json:"titleField,omitempty"`
	SortField    string   `json:"sortField,omitempty"`
	SortOrder    string   `json:"sortOrder,omitempty"`
	SearchFields []string `json:"searchFields,omitempty"`
	Fields       []*Field `json:"fields"`
	Permissions  []Perm   `json:"permissions,omitempty"`
	Description  string   `json:"description,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	// FormApps are the apps shipping a <snake>.form.ts for this DocType, the
	// owner first and then each extension in load order. The desk loads them
	// all: form handlers accumulate, they do not replace one another.
	FormApps []string `json:"formApps,omitempty"`
	// ExtendedBy names the apps that extendDoctype'd this one, in load order.
	ExtendedBy []string `json:"extendedBy,omitempty"`
	// TextApp is the app whose catalogue owns this DocType's own label and
	// description, when an extension overrode them; empty means App. The
	// counterpart of Field.App, and used by the same extractor. It never
	// crosses the wire: what a reader sees is the translation, not its origin.
	TextApp    string   `json:"-"`
	SourceFile string   `json:"sourceFile,omitempty"`
	Controller bool     `json:"hasController,omitempty"`
	Methods    []string `json:"methods,omitempty"`
	PermHook   bool     `json:"hasPermissionHook,omitempty"`
	PermQuery  bool     `json:"hasPermissionQuery,omitempty"`
	// RenamedFrom is the DocType name this one used to have. Plan renames the
	// table and its indexes; the engine rewrites the stored references.
	RenamedFrom Names `json:"renamedFrom,omitempty"`

	fieldMap map[string]*Field
}

// Standard columns every table has.
var StdColumns = []string{"name", "owner", "creation", "modified", "modified_by", "docstatus"}
var ChildColumns = []string{"parent", "parenttype", "parentfield", "idx"}

func (d *DocType) TableName() string { return "tab_" + Snake(d.Name) }

// ResetFieldIndex drops the lazily built fieldname index. A copy of a DocType
// inherits the original's index, which points at the original's fields.
func (d *DocType) ResetFieldIndex() { d.fieldMap = nil }

func (d *DocType) Field(name string) *Field {
	if d.fieldMap == nil {
		d.fieldMap = map[string]*Field{}
		for _, f := range d.Fields {
			if f.Fieldname != "" {
				d.fieldMap[f.Fieldname] = f
			}
		}
	}
	return d.fieldMap[name]
}

// DataFields are the fields that map to a column.
func (d *DocType) DataFields() []*Field {
	var out []*Field
	for _, f := range d.Fields {
		if ColumnType(f.Fieldtype) != "" {
			out = append(out, f)
		}
	}
	return out
}

func (d *DocType) TableFields() []*Field {
	var out []*Field
	for _, f := range d.Fields {
		if f.Fieldtype == "Table" {
			out = append(out, f)
		}
	}
	return out
}

func (d *DocType) IsStdColumn(name string) bool {
	for _, c := range StdColumns {
		if c == name {
			return true
		}
	}
	if d.IsChild {
		for _, c := range ChildColumns {
			if c == name {
				return true
			}
		}
	}
	return false
}

// HasColumn reports whether `name` is a queryable column.
func (d *DocType) HasColumn(name string) bool {
	if d.IsStdColumn(name) {
		return true
	}
	f := d.Field(name)
	return f != nil && ColumnType(f.Fieldtype) != ""
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Snake turns "Reajuste Contrato" into "reajuste_contrato".
func Snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 && unicode.IsLower(rune(s[i-1])) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Trim(nonAlnum.ReplaceAllString(b.String(), "_"), "_")
}

// Registry holds all loaded DocTypes.
type Registry struct {
	DocTypes map[string]*DocType
}

func NewRegistry() *Registry { return &Registry{DocTypes: map[string]*DocType{}} }

func (r *Registry) Add(d *DocType) error {
	if _, dup := r.DocTypes[d.Name]; dup {
		return fmt.Errorf("DocType %q definido duas vezes", d.Name)
	}
	r.DocTypes[d.Name] = d
	return nil
}

func (r *Registry) Get(name string) (*DocType, bool) {
	d, ok := r.DocTypes[name]
	return d, ok
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.DocTypes))
	for n := range r.DocTypes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var fieldnameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate checks every DocType for internal consistency and cross-references.
func (r *Registry) Validate() error {
	var errs []string
	valid := map[string]bool{}
	for _, t := range ValidFieldTypes {
		valid[t] = true
	}
	for _, d := range r.DocTypes {
		e := func(msg string, a ...any) { errs = append(errs, d.Name+": "+fmt.Sprintf(msg, a...)) }
		seen := map[string]bool{}
		renamedFrom := map[string]string{} // old fieldname -> the field claiming it
		for _, f := range d.Fields {
			if !valid[f.Fieldtype] {
				e("invalid fieldtype %q on field %q", f.Fieldtype, f.Fieldname)
			}
			if LayoutTypes[f.Fieldtype] {
				continue
			}
			if !fieldnameRe.MatchString(f.Fieldname) {
				e("invalid fieldname %q (use ascii snake_case)", f.Fieldname)
			}
			if seen[f.Fieldname] {
				e("fieldname %q duplicado", f.Fieldname)
			}
			seen[f.Fieldname] = true
			if d.IsStdColumn(f.Fieldname) || f.Fieldname == "doctype" {
				e("fieldname %q is reserved", f.Fieldname)
			}
			// a Currency or Percent column is numeric(21,9), so nine is the
			// most it can hold. Caught here, at migrate, rather than as a
			// silent truncation on the first save that reaches it.
			if f.Precision < 0 || f.Precision > 9 {
				e("field %q: precision %d is out of range (0 to 9)", f.Fieldname, f.Precision)
			}
			switch f.Fieldtype {
			case "Link", "Table":
				target := f.OptionsString()
				if target == "" {
					e("field %q (%s) needs options naming the target DocType", f.Fieldname, f.Fieldtype)
				} else if t, ok := r.DocTypes[target]; !ok {
					e("field %q points at DocType %q, which does not exist", f.Fieldname, target)
				} else if f.Fieldtype == "Table" && !t.IsChild {
					e("field %q: %q is not isChild", f.Fieldname, target)
				}
			case "Select":
				if _, ok := f.Options.([]any); !ok {
					if _, ok := f.Options.(string); !ok {
						e("field %q (Select) needs options as a list", f.Fieldname)
					}
				}
			}
			for _, prev := range f.RenamedFrom {
				switch {
				case !fieldnameRe.MatchString(prev):
					e("renamedFrom %q on field %q: not a fieldname", prev, f.Fieldname)
				case prev == f.Fieldname:
					e("renamedFrom on field %q names the field itself", f.Fieldname)
				case d.IsStdColumn(prev) || prev == "doctype":
					e("renamedFrom %q on field %q is a reserved column", prev, f.Fieldname)
				case renamedFrom[prev] != "":
					// Two fields claiming the same old column would race for it.
					e("field %q and field %q both declare renamedFrom %q", renamedFrom[prev], f.Fieldname, prev)
				}
				renamedFrom[prev] = f.Fieldname
			}
			if cv := f.Convert; cv != nil {
				switch {
				case !valid[cv.From]:
					e("convert.from %q on field %q is not a fieldtype", cv.From, f.Fieldname)
				case ColumnType(cv.From) == "":
					e("convert.from %q on field %q has no column", cv.From, f.Fieldname)
				case ColumnType(cv.From) == ColumnType(f.Fieldtype):
					e("convert on field %q: %s and %s are the same column type, there is nothing to convert",
						f.Fieldname, cv.From, f.Fieldtype)
				}
			}
			if f.FetchFrom != "" {
				parts := strings.SplitN(f.FetchFrom, ".", 2)
				if len(parts) != 2 {
					e("fetchFrom %q on field %q must be link.field", f.FetchFrom, f.Fieldname)
				} else if lf := d.Field(parts[0]); lf == nil || (lf.Fieldtype != "Link" && lf.Fieldtype != "Dynamic Link") {
					e("fetchFrom %q on field %q: %q is not a Link", f.FetchFrom, f.Fieldname, parts[0])
				}
			}
		}
		if d.Naming.Field != "" && d.Field(d.Naming.Field) == nil {
			e("naming.field %q does not exist", d.Naming.Field)
		}
		// A field can be renamed and leave these behind pointing at a name
		// nothing answers to. They are the half of a rename the declaration
		// cannot do for you, so the meta refuses to load until they follow.
		//
		// All three may also name a standard column — `titleField: "name"` and
		// `searchFields: ["name"]` are both documented — so those are not a
		// dangling reference.
		named := func(what, fieldname string) {
			if fieldname != "" && !d.IsStdColumn(fieldname) && d.Field(fieldname) == nil {
				e("%s %q does not exist", what, fieldname)
			}
		}
		named("titleField", d.TitleField)
		named("sortField", d.SortField)
		for _, sf := range d.SearchFields {
			named("searchFields", sf)
		}
		if d.IsChild && len(d.Permissions) > 0 {
			e("a child DocType has no permissions")
		}
		// a → b while a is also a live field is fine: the rename runs first and
		// the new a is added after. a ↔ b is a swap, which no single migration
		// can do — one of the two names is always occupied.
		for prev, claimant := range renamedFrom {
			other := d.Field(prev)
			if other == nil {
				continue
			}
			for _, back := range other.RenamedFrom {
				if back == claimant {
					e("fields %q and %q swap names; a swap needs a third name and a release in between", prev, claimant)
				}
			}
		}
		for _, prev := range d.RenamedFrom {
			switch {
			case strings.TrimSpace(prev) == "":
				e("renamedFrom is empty")
			case prev == d.Name:
				e("renamedFrom names the DocType itself")
			case r.DocTypes[prev] != nil:
				e("renamedFrom %q is a DocType that still exists", prev)
			case Snake(prev) == Snake(d.Name):
				e("renamedFrom %q and %q are the same table, there is nothing to rename", prev, d.Name)
			}
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("invalid meta:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// SelectValues returns the Select options as strings.
func (f *Field) SelectValues() []string {
	switch o := f.Options.(type) {
	case []any:
		out := make([]string, 0, len(o))
		for _, v := range o {
			out = append(out, fmt.Sprint(v))
		}
		return out
	case []string:
		return o
	case string:
		return strings.Split(o, "\n")
	}
	return nil
}

// HasController reports whether the doctype has permission hooks in TS.
func (d *DocType) HasController() bool { return d.PermHook || d.PermQuery }

var asciiIdent = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidIdentAscii checks app names and similar identifiers.
func ValidIdentAscii(s string) bool { return asciiIdent.MatchString(s) }
