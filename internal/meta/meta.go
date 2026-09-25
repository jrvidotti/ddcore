// Package meta holds the DocType model: what the TS `defineDoctype` produces,
// after being executed in the JS runtime and serialized to JSON.
package meta

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Layout fieldtypes have no column.
var LayoutTypes = map[string]bool{"Section Break": true, "Tab Break": true, "HTML": true, "Report": true}

// ColumnType maps a fieldtype to its Postgres column type ("" = no column).
func ColumnType(ft string) string {
	switch ft {
	case "Data", "Email", "Small Text", "Text", "Text Editor", "Markdown Editor", "Code", "Color",
		"Select", "Link", "Dynamic Link", "Attach", "Attach Image", "Password":
		return "text"
	case "Int", "Duration", "Rating":
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

var ValidFieldTypes = []string{"Data", "Email", "Small Text", "Text", "Text Editor", "Markdown Editor", "Code", "Int", "Float", "Currency", "Percent", "Check", "Rating", "Duration", "Color", "Date", "Month", "Datetime", "Time", "Select", "Link", "Dynamic Link", "Table", "Table MultiSelect", "Attach", "Attach Image", "JSON", "Password", "Vault", "Section Break", "Tab Break", "HTML", "Report"}

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
	Width              string `json:"width,omitempty"`
	GridEditMode       string `json:"gridEditMode,omitempty"`
	// GridSort is the order a Table (or Report) grid shows its rows in, and
	// GridSortable lets the user change it by clicking a column. Both change
	// the display only: a child row's idx stays what the user saved.
	GridSort     *GridSort `json:"gridSort,omitempty"`
	GridSortable bool      `json:"gridSortable,omitempty"`
	// GridExport offers the grid's rows as CSV or XLSX, when the user may
	// export the DocType; GridSelect adds row checkboxes and batch actions.
	GridExport bool `json:"gridExport,omitempty"`
	GridSelect bool `json:"gridSelect,omitempty"`
	// GridFilters are preset toggles above a Table (or Report) grid, each
	// narrowing the rows on screen. Display only, like GridSort.
	GridFilters []GridFilter `json:"gridFilters,omitempty"`
	// GridIndex set to false hides a Table grid's `#` column (the row's idx);
	// unset, the column shows.
	GridIndex *bool `json:"gridIndex,omitempty"`
	// ReportFilters maps a Report field's report filter to the parent field
	// (or `id`) whose value it takes.
	ReportFilters map[string]string `json:"reportFilters,omitempty"`
	// Computed marks a field with no column: never stored, always read-only,
	// its value set by the controller's onLoad each time the form loads.
	Computed bool `json:"computed,omitempty"`
	// ShowFileName shows an Attach's file name next to its icon or thumbnail;
	// by default the desk shows only those, with the file's details on hover.
	ShowFileName bool `json:"showFileName,omitempty"`
	Collapsible  bool `json:"collapsible,omitempty"`
	Bold         bool `json:"bold,omitempty"`
	// Permlevel groups the field under the permission rows of the same level
	// (SEC-02). Level 0 follows the DocType's own permissions; a field at a
	// higher level is read and written only by a role granted that level.
	Permlevel       int  `json:"permlevel,omitempty"`
	IgnoreUserPerms bool `json:"-"`
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

// GridFilter is one preset filter of a grid: a toggle labelled Label that,
// when on, shows only the rows matching Filters (list-style tuples,
// `[["in_class", "=", 1]]`, or an object, `{ in_class: 1 }`). Toggles that
// are on combine with AND; Default turns one on when the form opens.
type GridFilter struct {
	Label   string          `json:"label"`
	Filters json.RawMessage `json:"filters"`
	Default bool            `json:"default,omitempty"`
}

// GridFilterOps are the operators a grid filter evaluates in the browser: the
// list's, without the tree ones, which need the database.
var GridFilterOps = map[string]bool{
	"=": true, "!=": true, ">": true, ">=": true, "<": true, "<=": true,
	"like": true, "not like": true, "in": true, "not in": true,
	"between": true, "is": true, "set": true, "not set": true,
}

// GridFilterFields parses a grid filter's Filters and returns the fields it
// names, or an error for a shape or an operator a grid cannot evaluate.
func GridFilterFields(raw json.RawMessage) ([]string, error) {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil && obj != nil {
		var out []string
		for k := range obj {
			out = append(out, k)
		}
		sort.Strings(out)
		if len(out) == 0 {
			return nil, fmt.Errorf("filters is empty")
		}
		return out, nil
	}
	var list [][]any
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("filters must be a list of [field, operator, value] or an object")
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("filters is empty")
	}
	var out []string
	for _, tp := range list {
		if len(tp) < 2 || len(tp) > 3 {
			return nil, fmt.Errorf("a filter is [field, operator, value] or [field, value], not %v", tp)
		}
		field, ok := tp[0].(string)
		if !ok || field == "" {
			return nil, fmt.Errorf("a filter's first element is a fieldname, not %v", tp[0])
		}
		if len(tp) == 3 {
			op, _ := tp[1].(string)
			if !GridFilterOps[strings.ToLower(op)] {
				return nil, fmt.Errorf("operator %q cannot filter a grid", op)
			}
		}
		out = append(out, field)
	}
	return out, nil
}

// GridSort is a grid's default order.
type GridSort struct {
	Field string `json:"field"`
	Order string `json:"order,omitempty"` // "asc" (the default) or "desc"
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

// DefaultRatingMax is how many stars a Rating has when `options` says nothing.
const DefaultRatingMax = 5

// MaxRatingMax is the largest rating a field may declare: past ten stars the
// control is unreadable and the value is really a Float.
const MaxRatingMax = 10

// RatingMax is how many stars the field holds. `options` carries it as a
// number, which JSON decodes as a float64, or as the string an app wrote by
// hand. Out-of-range values are refused by Validate, so this only has to agree
// with it on what the number is.
func (f *Field) RatingMax() int {
	switch o := f.Options.(type) {
	case float64:
		return int(o)
	case int:
		return o
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(o)); err == nil {
			return n
		}
	}
	return DefaultRatingMax
}

// DurationFlags are the display options a Duration accepts. They hide a unit in
// the control and in every formatted value; the stored number of seconds is
// never affected.
var DurationFlags = map[string]bool{"hideDays": true, "hideSeconds": true}

// DurationHides reports whether the field declares a display flag.
func (f *Field) DurationHides(flag string) bool {
	for _, v := range f.SelectValues() {
		if v == flag {
			return true
		}
	}
	return false
}

var codeLanguageRe = regexp.MustCompile(`^[a-z0-9+#._-]*$`)

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
	Import  bool   `json:"import,omitempty"` // load rows from a spreadsheet (Data Import)
	Share   bool   `json:"share,omitempty"`  // share one document with another user (SEC-03)
	IfOwner bool   `json:"ifOwner,omitempty"`
	// Permlevel is the field level this row grants. A row above level 0 grants
	// only read and write on that level's fields — never the document itself.
	Permlevel int `json:"permlevel,omitempty"`
}

// MaxPermlevel is the highest field permission level.
const MaxPermlevel = 9

// GloballySearchable reports whether global search looks into this DocType:
// never a child table or a Single; otherwise the explicit globalSearch flag,
// or, without one, whether the DocType declares a title or search fields.
func (d *DocType) GloballySearchable() bool {
	if d.IsChild || d.IsSingle {
		return false
	}
	if d.GlobalSearch != nil {
		return *d.GlobalSearch
	}
	// A virtual DocType's rows are its sources' documents, which global
	// search already finds; it only joins in when it opts in.
	if d.IsVirtual() {
		return false
	}
	return d.TitleField != "" || len(d.SearchFields) > 0
}

// TitleIsTranslatedID reports whether a document's display title is its id
// run through the catalogue: TranslateID set and no TitleField to read instead.
func (d *DocType) TitleIsTranslatedID() bool {
	return d.TranslateID && (d.TitleField == "" || d.TitleField == "id")
}

// HasRestrictedFields reports whether any field sits above permission level 0.
func (d *DocType) HasRestrictedFields() bool {
	for _, f := range d.Fields {
		if f.Permlevel > 0 {
			return true
		}
	}
	return false
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
	case "import":
		return p.Import
	case "share":
		return p.Share
	}
	return false
}

// IDGeneration is how a new document gets its id: from a series, a field's
// value, a format, a hash, or a prompt to the person creating it.
type IDGeneration struct {
	Series string `json:"series,omitempty"`
	Field  string `json:"field,omitempty"`
	Hash   bool   `json:"hash,omitempty"`
	Prompt bool   `json:"prompt,omitempty"`
	Format string `json:"format,omitempty"`
}

// MaxIdentifier is Postgres's identifier limit. A longer name is silently
// truncated, and a truncated index name never compares equal to the one the
// planner wants — so migrate would drop and recreate it on every run.
const MaxIdentifier = 63

// UniqueKey is a business key spanning more than one column, enforced by a
// partial unique index. Naming it — rather than deriving a name from the field
// list — is what keeps the index still when the fields are reordered or
// renamed, because the planner tells indexes apart by name. The name is also
// what a duplicate error quotes, so it should read as the key, not the columns.
type UniqueKey struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
}

// IndexSuffix is the suffix this key occupies in the table's index namespace.
// The `uk_` infix is what lets Plan drop the keys the meta no longer declares
// without ever reaching an index it does not own.
func (k UniqueKey) IndexSuffix() string { return "uk_" + k.Name }

type DocType struct {
	Name   string `json:"name"`
	App    string `json:"app"`
	Module string `json:"module,omitempty"`
	Label  string `json:"label,omitempty"`
	// IDLabel is what the desk calls the document's id (a catalogue key):
	// "Contract No." rather than "ID". Display only — the column, filters,
	// orderBy and the API still say `id`.
	IDLabel      string       `json:"idLabel,omitempty"`
	IDGeneration IDGeneration `json:"idGeneration"`
	Submittable  bool         `json:"submittable,omitempty"`
	IsChild      bool         `json:"isChild,omitempty"`
	IsSingle     bool         `json:"isSingle,omitempty"`
	// IsTree makes the documents a hierarchy (DAT-07): a self-referencing Link
	// holds the parent and `is_group` says which documents may have children.
	// ApplyTrees adds both fields when the DocType does not declare them.
	IsTree bool `json:"isTree,omitempty"`
	// ParentField is the Link field holding the parent; TreeParentField falls
	// back to `parent_<snake(name)>`.
	ParentField string `json:"parentField,omitempty"`
	// Virtual makes the DocType a read-only union of other DocTypes (DAT-07):
	// no table, no writes, and each row is a readable row of one source.
	Virtual      *VirtualDef `json:"virtual,omitempty"`
	TrackChanges bool        `json:"trackChanges,omitempty"`
	AllowRename  bool        `json:"allowRename,omitempty"`
	TitleField   string      `json:"titleField,omitempty"`
	// TranslateID makes the id a catalogue key for display: the desk shows
	// the translated id wherever it shows the document's title (a Link, a
	// grid cell, the list, the form header) while the stored value stays the
	// canonical English id. Meant for DocTypes whose ids are fixed keys
	// declared in code, such as Role; it has no effect with a TitleField.
	TranslateID bool `json:"translateId,omitempty"`
	// ImageField names the Attach Image (or Attach) field that pictures a
	// document; the Desk's Cards view shows it, with an initials avatar when
	// it is empty or unreadable.
	ImageField   string   `json:"imageField,omitempty"`
	SortField    string   `json:"sortField,omitempty"`
	SortOrder    string   `json:"sortOrder,omitempty"`
	SearchFields []string `json:"searchFields,omitempty"`
	// LinkSubtitle lists the fields a Link dropdown shows under each title,
	// in order; nil keeps the default (id and searchFields). "id" is allowed.
	LinkSubtitle []string `json:"linkSubtitle,omitempty"`
	// GlobalSearch opts a DocType in (true) or out (false) of the Desk's
	// global search; nil leaves it to GloballySearchable's default.
	GlobalSearch *bool `json:"globalSearch,omitempty"`
	// UniqueKeys are the compound business keys, one partial unique index each.
	UniqueKeys  []UniqueKey `json:"uniqueKeys,omitempty"`
	Fields      []*Field    `json:"fields"`
	Permissions []Perm      `json:"permissions,omitempty"`
	Description string      `json:"description,omitempty"`
	Icon        string      `json:"icon,omitempty"`
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

// Standard columns every table has. The order is the one db.stdColumns builds,
// and a unit test holds the two lists together.
var StdColumns = []string{"id", "owner", "creation", "modified", "modified_by", "docstatus"}
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

// UniqueKey returns the compound key by name, or nil.
func (d *DocType) UniqueKey(name string) *UniqueKey {
	for i := range d.UniqueKeys {
		if d.UniqueKeys[i].Name == name {
			return &d.UniqueKeys[i]
		}
	}
	return nil
}

// HasColumnField reports whether the field is stored in a column: its
// fieldtype has one and it is not computed.
func HasColumnField(f *Field) bool {
	return f != nil && !f.Computed && ColumnType(f.Fieldtype) != ""
}

// DataFields are the fields that map to a column.
func (d *DocType) DataFields() []*Field {
	var out []*Field
	for _, f := range d.Fields {
		if HasColumnField(f) {
			out = append(out, f)
		}
	}
	return out
}

// IsTableType says whether a fieldtype stores its value as child rows: a
// Table, or a Table MultiSelect, which is the same rows edited as a list of
// links.
func IsTableType(ft string) bool { return ft == "Table" || ft == "Table MultiSelect" }

// MultiSelectLinkField is the one Link field of a Table MultiSelect's child
// DocType, the field that holds each chosen value. Nil when the DocType has
// none or more than one, which Validate refuses.
func (d *DocType) MultiSelectLinkField() *Field {
	var link *Field
	for _, f := range d.Fields {
		if f.Fieldtype == "Link" {
			if link != nil {
				return nil
			}
			link = f
		}
	}
	return link
}

func (d *DocType) TableFields() []*Field {
	var out []*Field
	for _, f := range d.Fields {
		if IsTableType(f.Fieldtype) {
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
	return HasColumnField(d.Field(name))
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

// ObsoleteFieldtypes are fieldtypes ddcore no longer has, mapped to what
// replaced them. They are dropped when an app loads instead of failing its
// validation, so a DocType written against an older version still runs.
var ObsoleteFieldtypes = map[string]string{
	"Column Break": "remove it; a form field now sizes itself with `width`",
}

// DropObsoleteFields removes every field of an obsolete fieldtype, returning
// one message per DocType that had any, for the caller to log.
func (r *Registry) DropObsoleteFields() []string {
	var warnings []string
	for _, name := range r.Names() {
		d := r.DocTypes[name]
		kept := make([]*Field, 0, len(d.Fields))
		dropped := map[string]int{}
		for _, f := range d.Fields {
			if _, obsolete := ObsoleteFieldtypes[f.Fieldtype]; obsolete {
				dropped[f.Fieldtype]++
				continue
			}
			kept = append(kept, f)
		}
		if len(dropped) == 0 {
			continue
		}
		d.Fields = kept
		for _, ft := range sortedKeys(dropped) {
			warnings = append(warnings, fmt.Sprintf("%s: dropped %d %q field(s) — %s", d.Name, dropped[ft], ft, ObsoleteFieldtypes[ft]))
		}
	}
	return warnings
}

// Validate checks every DocType for internal consistency and cross-references.
func (r *Registry) Validate() error {
	var errs []string
	valid := map[string]bool{}
	for _, t := range ValidFieldTypes {
		valid[t] = true
	}
	for _, d := range r.DocTypes {
		e := func(msg string, a ...any) { errs = append(errs, d.Name+": "+fmt.Sprintf(msg, a...)) }
		if d.IsSingle && (d.IsChild || d.Submittable || d.AllowRename || d.IDGeneration != (IDGeneration{})) {
			e("Single DocTypes cannot be child tables, submittable, renamable, or declare idGeneration")
		}
		seen := map[string]bool{}
		renamedFrom := map[string]string{} // old fieldname -> the field claiming it
		for _, f := range d.Fields {
			if !valid[f.Fieldtype] {
				e("invalid fieldtype %q on field %q", f.Fieldtype, f.Fieldname)
			}
			r.validateGrid(d, f, e)
			if LayoutTypes[f.Fieldtype] {
				continue
			}
			if !fieldnameRe.MatchString(f.Fieldname) {
				e("invalid fieldname %q (use ascii snake_case)", f.Fieldname)
			}
			if seen[f.Fieldname] {
				e("duplicate fieldname %q", f.Fieldname)
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
			if f.Width != "" && f.Width != "sm" && f.Width != "md" && f.Width != "lg" && f.Width != "full" {
				e("field %q: invalid width %q (must be sm, md, lg, or full)", f.Fieldname, f.Width)
			}
			switch f.Fieldtype {
			case "Link", "Table", "Table MultiSelect":
				target := f.OptionsString()
				if target == "" {
					e("field %q (%s) needs options naming the target DocType", f.Fieldname, f.Fieldtype)
				} else if t, ok := r.DocTypes[target]; !ok {
					e("field %q points at DocType %q, which does not exist", f.Fieldname, target)
				} else if IsTableType(f.Fieldtype) && !t.IsChild {
					e("field %q: %q is not isChild", f.Fieldname, target)
				} else if f.Fieldtype == "Table MultiSelect" {
					// The control edits one value per row, so the row has to be
					// that value and nothing a person would otherwise fill in.
					if t.MultiSelectLinkField() == nil {
						e("field %q (Table MultiSelect): %q needs exactly one Link field", f.Fieldname, target)
					}
					for _, cf := range t.Fields {
						if cf.Fieldtype != "Link" && cf.Reqd && cf.Default == nil && !LayoutTypes[cf.Fieldtype] {
							e("field %q (Table MultiSelect): %q.%s is required and has no default, which the control cannot fill", f.Fieldname, target, cf.Fieldname)
						}
					}
				}
			case "Vault":
				// `{name}` stood for the document key before 0.17. Now it would
				// only match a field called `name`, and without one the key
				// would carry the literal text — every document sharing one
				// secret. Caught here rather than on the first save.
				if strings.Contains(f.OptionsString(), "{name}") && d.Field("name") == nil {
					e("field %q (Vault): the key template says {name}, which is {id} since 0.17", f.Fieldname)
				}
			case "Select":
				if _, ok := f.Options.([]any); !ok {
					if _, ok := f.Options.(string); !ok {
						e("field %q (Select) needs options as a list", f.Fieldname)
					}
				}
			case "Rating":
				// The number of stars decides what a stored value means, so a
				// typo here would silently rescale the whole column.
				if f.Options != nil {
					if n := f.RatingMax(); n < 1 || n > MaxRatingMax {
						e("field %q (Rating): options is the number of stars, 1 to %d, not %v", f.Fieldname, MaxRatingMax, f.Options)
					}
				}
			case "Duration":
				for _, flag := range f.SelectValues() {
					if !DurationFlags[flag] {
						e("field %q (Duration): unknown option %q (hideDays, hideSeconds)", f.Fieldname, flag)
					}
				}
			case "Code":
				if lang := f.OptionsString(); !codeLanguageRe.MatchString(lang) {
					e("field %q (Code): options is the language, lowercase (%q)", f.Fieldname, lang)
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
				case prev == "name":
					// `name` was the document key before 0.17 and migrate has
					// already moved it to `id` by the time a rename is planned,
					// so this would find no column to move. A field may be
					// called `name` now; it just cannot claim the old key.
					e("renamedFrom %q on field %q: `name` was the document key before 0.17, and migrate moves it to `id` on its own", prev, f.Fieldname)
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
			if f.Computed {
				switch {
				case d.IsVirtual():
					e("field %q: a virtual DocType cannot have computed fields", f.Fieldname)
				case ColumnType(f.Fieldtype) == "" || f.Fieldtype == "Password":
					e("field %q: a %s cannot be computed", f.Fieldname, f.Fieldtype)
				case f.FetchFrom != "" || f.Unique || f.Reqd:
					e("field %q: a computed field cannot be fetchFrom, unique or reqd", f.Fieldname)
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
		if d.IDLabel != "" && strings.TrimSpace(d.IDLabel) == "" {
			e("idLabel is blank")
		}
		if d.IDGeneration.Field != "" && d.Field(d.IDGeneration.Field) == nil {
			e("idGeneration.field %q does not exist", d.IDGeneration.Field)
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
		// No permlevel check: a restricted photo only shows as the avatar.
		if d.ImageField != "" {
			if f := d.Field(d.ImageField); f == nil {
				e("imageField %q does not exist", d.ImageField)
			} else if f.Fieldtype != "Attach Image" && f.Fieldtype != "Attach" {
				e("imageField %q is a %s, not an Attach Image or Attach field", d.ImageField, f.Fieldtype)
			}
		}
		for _, sf := range d.SearchFields {
			named("searchFields", sf)
		}
		for _, lf := range d.LinkSubtitle {
			named("linkSubtitle", lf)
		}
		validateUniqueKeys(d, e)
		validateTree(d, e)
		validateVirtual(r, d, e)
		validateFieldPermissions(r, d, e)
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

// validateUniqueKeys checks the compound business keys. Everything here is
// refused at load rather than at migrate: a key that cannot become an index is
// a declaration that would silently enforce nothing.
func validateUniqueKeys(d *DocType, e func(string, ...any)) {
	if len(d.UniqueKeys) == 0 {
		return
	}
	switch {
	case d.IsChild:
		e("uniqueKeys: a child DocType has no business key of its own — declare it on the parent")
		return
	case d.IsSingle:
		e("uniqueKeys: a single DocType holds one document, so there is nothing to keep unique")
		return
	case d.IsVirtual():
		e("uniqueKeys: a virtual DocType has no table to hold an index")
		return
	}
	names := map[string]bool{}
	sets := map[string]string{}
	for _, k := range d.UniqueKeys {
		switch {
		case !fieldnameRe.MatchString(k.Name):
			e("uniqueKeys %q is not a valid name (use ascii snake_case)", k.Name)
			continue
		case names[k.Name]:
			e("uniqueKeys %q is declared twice", k.Name)
			continue
		}
		names[k.Name] = true
		// The index is named after the key, so the key's name is what has to
		// fit — and a name that only fits after truncation is one migrate
		// would rebuild forever.
		if n := len(d.TableName()) + 1 + len(k.IndexSuffix()); n > MaxIdentifier {
			e("uniqueKeys %q: its index name would be %d bytes, past the %d Postgres keeps — shorten the key name", k.Name, n, MaxIdentifier)
		}
		// A field of that name would want the very same index name.
		if f := d.Field(k.IndexSuffix()); f != nil {
			e("uniqueKeys %q: field %q would claim the same index name", k.Name, f.Fieldname)
		}
		if len(k.Fields) < 2 {
			e("uniqueKeys %q spans %d field(s); a key spans two or more — one field is `unique: true`", k.Name, len(k.Fields))
			continue
		}
		seen, ok := map[string]bool{}, true
		for _, fn := range k.Fields {
			f := d.Field(fn)
			switch {
			case seen[fn]:
				e("uniqueKeys %q lists field %q twice", k.Name, fn)
				ok = false
			case d.IsStdColumn(fn) || fn == "doctype":
				e("uniqueKeys %q: %q is a standard column, not a business field", k.Name, fn)
				ok = false
			case f == nil:
				e("uniqueKeys %q: field %q does not exist", k.Name, fn)
				ok = false
			case !HasColumnField(f):
				e("uniqueKeys %q: field %q is a %s, which has no column of its own", k.Name, fn, f.Fieldtype)
				ok = false
			}
			seen[fn] = true
		}
		if !ok {
			continue
		}
		// Order decides the index, never the constraint: two keys over the
		// same fields in any order are two names for one index.
		cols := append([]string(nil), k.Fields...)
		sort.Strings(cols)
		set := strings.Join(cols, "\x00")
		if prev, dup := sets[set]; dup {
			e("uniqueKeys %q and %q cover the same fields", prev, k.Name)
			continue
		}
		sets[set] = k.Name
	}
}

// validateFieldPermissions checks permlevel declarations (SEC-02). A field that
// identifies or describes a document outside its own form — its name, title,
// search text — is shown to everyone who may see the document, so it cannot
// be restricted; nor can a level-0 field copy a restricted value in.
func validateFieldPermissions(r *Registry, d *DocType, e func(string, ...any)) {
	for _, f := range d.Fields {
		if f.Permlevel < 0 || f.Permlevel > MaxPermlevel {
			e("field %q: permlevel %d is out of range (0 to %d)", f.Fieldname, f.Permlevel, MaxPermlevel)
		}
		if f.Permlevel > 0 && LayoutTypes[f.Fieldtype] {
			e("field %q: a %s holds no value, so it has no permlevel", f.Fieldname, f.Fieldtype)
		}
		if f.FetchFrom != "" && f.Permlevel == 0 {
			parts := strings.SplitN(f.FetchFrom, ".", 2)
			if len(parts) == 2 {
				if lf := d.Field(parts[0]); lf != nil && lf.Fieldtype == "Link" {
					if t, ok := r.DocTypes[lf.OptionsString()]; ok {
						if src := t.Field(parts[1]); src != nil && src.Permlevel > 0 {
							e("field %q: fetchFrom %q copies a permlevel %d field into a permlevel 0 one", f.Fieldname, f.FetchFrom, src.Permlevel)
						}
					}
				}
			}
		}
	}
	level0 := func(what, fieldname string) {
		if f := d.Field(fieldname); f != nil && f.Permlevel > 0 {
			e("%s %q has permlevel %d; it identifies the document and must be permlevel 0", what, fieldname, f.Permlevel)
		}
	}
	level0("titleField", d.TitleField)
	level0("idGeneration.field", d.IDGeneration.Field)
	for _, sf := range d.SearchFields {
		level0("searchFields", sf)
	}
	for _, lf := range d.LinkSubtitle {
		level0("linkSubtitle", lf)
	}
	for _, name := range idFormatFields(d.IDGeneration.Format) {
		level0("idGeneration.format", name)
	}
	for _, p := range d.Permissions {
		if p.Permlevel < 0 || p.Permlevel > MaxPermlevel {
			e("permission for %q: permlevel %d is out of range (0 to %d)", p.Role, p.Permlevel, MaxPermlevel)
			continue
		}
		if p.Permlevel > 0 && (p.Create || p.Delete || p.Submit || p.Cancel || p.Amend || p.Report || p.Export || p.Import || p.Share || p.IfOwner) {
			e("permission for %q at permlevel %d may only grant read and write", p.Role, p.Permlevel)
		}
	}
}

var idFormatRe = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

func idFormatFields(format string) []string {
	var out []string
	for _, m := range idFormatRe.FindAllStringSubmatch(format, -1) {
		out = append(out, m[1])
	}
	return out
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

// validateGrid checks the grid properties (gridSort, gridSortable,
// gridExport, gridSelect), which only a Table or a Report field has,
// gridIndex, which only a Table has, and a
// Report field's own report and filters.
func (r *Registry) validateGrid(d *DocType, f *Field, e func(string, ...any)) {
	isGrid := f.Fieldtype == "Table" || f.Fieldtype == "Report"
	if !isGrid && (f.GridSort != nil || f.GridSortable || f.GridExport || f.GridSelect) {
		e("field %q: gridSort, gridSortable, gridExport and gridSelect are for a Table or a Report field, not a %s", f.Fieldname, f.Fieldtype)
	}
	if f.Fieldtype != "Table" && f.GridIndex != nil {
		e("field %q: gridIndex is for a Table field, not a %s", f.Fieldname, f.Fieldtype)
	}
	if !isGrid && f.GridFilters != nil {
		e("field %q: gridFilters is for a Table or a Report field, not a %s", f.Fieldname, f.Fieldtype)
	}
	if isGrid {
		var child *DocType
		if f.Fieldtype == "Table" {
			child = r.DocTypes[f.OptionsString()]
		}
		for i, gf := range f.GridFilters {
			if strings.TrimSpace(gf.Label) == "" {
				e("field %q: gridFilters[%d] needs a label", f.Fieldname, i)
			}
			fields, err := GridFilterFields(gf.Filters)
			if err != nil {
				e("field %q: gridFilters[%d]: %v", f.Fieldname, i, err)
				continue
			}
			// a Report's columns come from its execute: only a Table's
			// fields can be checked here
			for _, fn := range fields {
				if child == nil || fn == "idx" || child.IsStdColumn(fn) {
					continue
				}
				if cf := child.Field(fn); cf == nil || LayoutTypes[cf.Fieldtype] || IsTableType(cf.Fieldtype) {
					e("field %q: gridFilters[%d] filters on %q, which is not a field of %q", f.Fieldname, i, fn, child.Name)
				}
			}
		}
	}
	if f.Fieldtype != "Report" && f.ReportFilters != nil {
		e("field %q: reportFilters is for a Report field, not a %s", f.Fieldname, f.Fieldtype)
	}
	if gs := f.GridSort; gs != nil && isGrid {
		if gs.Order != "" && gs.Order != "asc" && gs.Order != "desc" {
			e("field %q: gridSort.order %q must be asc or desc", f.Fieldname, gs.Order)
		}
		if gs.Field == "" {
			e("field %q: gridSort needs a field", f.Fieldname)
		} else if f.Fieldtype == "Table" && gs.Field != "idx" {
			// a Report's columns come from its execute, so only a Table's
			// sort field can be checked here
			if t := r.DocTypes[f.OptionsString()]; t != nil {
				if cf := t.Field(gs.Field); cf == nil || LayoutTypes[cf.Fieldtype] || IsTableType(cf.Fieldtype) {
					e("field %q: gridSort.field %q is not a field of %q", f.Fieldname, gs.Field, t.Name)
				}
			}
		}
	}
	if f.Fieldtype != "Report" {
		return
	}
	if !fieldnameRe.MatchString(f.Fieldname) {
		e("invalid fieldname %q on a Report field (use ascii snake_case)", f.Fieldname)
	}
	if f.OptionsString() == "" {
		e("field %q (Report) needs options naming the report", f.Fieldname)
	}
	if d.IsChild {
		e("field %q: a child DocType cannot have a Report field", f.Fieldname)
	}
	for filter, from := range f.ReportFilters {
		if from != "id" && !d.IsStdColumn(from) {
			if pf := d.Field(from); pf == nil || LayoutTypes[pf.Fieldtype] || IsTableType(pf.Fieldtype) {
				e("field %q: reportFilters.%s takes %q, which is not a field of %q", f.Fieldname, filter, from, d.Name)
			}
		}
	}
}
