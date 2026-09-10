// Package meta holds the DocType model: what the TS `defineDoctype` produces,
// after being executed in the JS runtime and serialized to JSON.
package meta

import (
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
	Fieldname          string   `json:"fieldname,omitempty"`
	Fieldtype          string   `json:"fieldtype"`
	Label              string   `json:"label,omitempty"`
	Options            any      `json:"options,omitempty"` // string (Link/Table/Dynamic Link) or []string (Select)
	Reqd               bool     `json:"reqd,omitempty"`
	Unique             bool     `json:"unique,omitempty"`
	Default            any      `json:"default,omitempty"`
	ReadOnly           bool     `json:"readOnly,omitempty"`
	Hidden             bool     `json:"hidden,omitempty"`
	FetchFrom          string   `json:"fetchFrom,omitempty"`
	DependsOn          string   `json:"dependsOn,omitempty"`
	ReadOnlyDependsOn  string   `json:"readOnlyDependsOn,omitempty"`
	MandatoryDependsOn string   `json:"mandatoryDependsOn,omitempty"`
	AllowOnSubmit      bool     `json:"allowOnSubmit,omitempty"`
	InListView         bool     `json:"inListView,omitempty"`
	InStandardFilter   bool     `json:"inStandardFilter,omitempty"`
	SearchIndex        bool     `json:"searchIndex,omitempty"`
	Length             int      `json:"length,omitempty"`
	Precision          int      `json:"precision,omitempty"`
	Description        string   `json:"description,omitempty"`
	Columns            int      `json:"columns,omitempty"`
	GridEditMode       string   `json:"gridEditMode,omitempty"`
	Collapsible        bool     `json:"collapsible,omitempty"`
	Bold               bool     `json:"bold,omitempty"`
	IgnoreUserPerms    bool     `json:"-"`
	_                  struct{} // keep JSON tags exhaustive
	SelectOptions      []string `json:"-"`
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
	HasForm      bool     `json:"hasForm,omitempty"` // app ships a *.form.ts
	SourceFile   string   `json:"sourceFile,omitempty"`
	Controller   bool     `json:"hasController,omitempty"`
	Methods      []string `json:"methods,omitempty"`
	PermHook     bool     `json:"hasPermissionHook,omitempty"`
	PermQuery    bool     `json:"hasPermissionQuery,omitempty"`

	fieldMap map[string]*Field
}

// Standard columns every table has.
var StdColumns = []string{"name", "owner", "creation", "modified", "modified_by", "docstatus"}
var ChildColumns = []string{"parent", "parenttype", "parentfield", "idx"}

func (d *DocType) TableName() string { return "tab_" + Snake(d.Name) }

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
		for _, f := range d.Fields {
			if !valid[f.Fieldtype] {
				e("fieldtype %q inválido no campo %q", f.Fieldtype, f.Fieldname)
			}
			if LayoutTypes[f.Fieldtype] {
				continue
			}
			if !fieldnameRe.MatchString(f.Fieldname) {
				e("fieldname %q inválido (use snake_case ascii)", f.Fieldname)
			}
			if seen[f.Fieldname] {
				e("fieldname %q duplicado", f.Fieldname)
			}
			seen[f.Fieldname] = true
			if d.IsStdColumn(f.Fieldname) || f.Fieldname == "doctype" {
				e("fieldname %q é reservado", f.Fieldname)
			}
			switch f.Fieldtype {
			case "Link", "Table":
				target := f.OptionsString()
				if target == "" {
					e("campo %q (%s) precisa de options com o DocType alvo", f.Fieldname, f.Fieldtype)
				} else if t, ok := r.DocTypes[target]; !ok {
					e("campo %q aponta para DocType inexistente %q", f.Fieldname, target)
				} else if f.Fieldtype == "Table" && !t.IsChild {
					e("campo %q: %q não é isChild", f.Fieldname, target)
				}
			case "Select":
				if _, ok := f.Options.([]any); !ok {
					if _, ok := f.Options.(string); !ok {
						e("campo %q (Select) precisa de options como lista", f.Fieldname)
					}
				}
			}
			if f.FetchFrom != "" {
				parts := strings.SplitN(f.FetchFrom, ".", 2)
				if len(parts) != 2 {
					e("fetchFrom %q do campo %q deve ser link.campo", f.FetchFrom, f.Fieldname)
				} else if lf := d.Field(parts[0]); lf == nil || (lf.Fieldtype != "Link" && lf.Fieldtype != "Dynamic Link") {
					e("fetchFrom %q do campo %q: %q não é Link", f.FetchFrom, f.Fieldname, parts[0])
				}
			}
		}
		if d.Naming.Field != "" && d.Field(d.Naming.Field) == nil {
			e("naming.field %q não existe", d.Naming.Field)
		}
		if d.IsChild && len(d.Permissions) > 0 {
			e("child DocType não tem permissions")
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("meta inválida:\n  %s", strings.Join(errs, "\n  "))
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
