package db

import (
	"fmt"
	"regexp"
	"strings"
)

// Filter is one condition: [field, op, value] — field may be "Child DocType.field"
// resolved by the caller into a table join.
type Filter struct {
	Field   string
	Op      string
	Value   any
	IfField string // apply this filter only when IfField equals IfValue
	IfValue any
	// Any holds groups of filters: a row matches when every filter of at least
	// one group does. It is how the framework ORs its own permission branches
	// (a role grant or a document share) and is never parsed from a request.
	Any [][]Filter
	// Tree is the hierarchy a tree operator walks (DAT-07). The caller resolves
	// it from the meta — the column being filtered is a tree's own id or a Link
	// pointing at one — so a request never names a table itself.
	Tree *TreeRef
}

// TreeRef is the table a tree operator recurses over: rows are joined from
// ParentCol to the document key, `id`.
type TreeRef struct {
	Table     string
	ParentCol string
}

// TreeOps are the operators that walk a hierarchy. Each needs Filter.Tree.
var TreeOps = map[string]bool{
	"descendants of": true, "descendants of (inclusive)": true, "not descendants of": true,
	"ancestors of": true, "not ancestors of": true,
}

var validOps = map[string]string{
	"=": "=", "!=": "<>", ">": ">", ">=": ">=", "<": "<", "<=": "<=",
	"like": "ILIKE", "not like": "NOT ILIKE", "in": "IN", "not in": "NOT IN",
	"between": "BETWEEN", "is": "IS", "set": "set", "not set": "not set",
	// The tree operators render as a recursive subquery, not as an SQL
	// operator; the value here only marks them valid. Before 0.17 "descendants
	// of" was mapped to "=", which answered a hierarchy question with an
	// exact match — silently wrong rather than refused.
	"descendants of": "tree", "descendants of (inclusive)": "tree", "not descendants of": "tree",
	"ancestors of": "tree", "not ancestors of": "tree",
}

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

const accentFrom = "ÁÀÃÂÄáàãâäÉÈÊËéèêëÍÌÎÏíìîïÓÒÕÔÖóòõôöÚÙÛÜúùûüÇçÑñ"
const accentTo = "AAAAAaaaaaEEEEeeeeIIIIiiiiOOOOOoooooUUUUuuuuCcNn"

// AccentInsensitive wraps a text SQL expression for accent-insensitive
// comparisons without requiring PostgreSQL's optional unaccent extension.
func AccentInsensitive(expr string) string {
	return fmt.Sprintf("lower(translate(%s::text, '%s', '%s'))", expr, accentFrom, accentTo)
}

// FoldAccents is AccentInsensitive in Go: it lowercases s and strips the
// same accents, so code ranking rows can compare text the way the query
// matched it.
func FoldAccents(s string) string {
	to := []rune(accentTo)
	var b strings.Builder
	for _, r := range s {
		if i := strings.IndexRune(accentFrom, r); i >= 0 {
			r = to[len([]rune(accentFrom[:i]))]
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// Ident quotes a Postgres identifier after validating it.
func Ident(s string) string {
	if !identRe.MatchString(s) {
		panic(fmt.Sprintf("invalid identifier: %q", s))
	}
	return `"` + s + `"`
}

func ValidIdent(s string) bool { return identRe.MatchString(s) }

// Builder accumulates SQL fragments with numbered placeholders.
type Builder struct {
	Args []any
}

func (b *Builder) Arg(v any) string {
	b.Args = append(b.Args, v)
	return fmt.Sprintf("$%d", len(b.Args))
}

// Where renders filters into a WHERE clause. `col` maps a field name into a
// qualified column expression, returning "" if unknown.
func (b *Builder) Where(filters []Filter, col func(field string) string) (string, error) {
	var parts []string
	for _, f := range filters {
		c := col(f.Field)
		if c == "" {
			return "", fmt.Errorf("unknown field in filter: %q", f.Field)
		}
		op := strings.ToLower(strings.TrimSpace(f.Op))
		if op == "" {
			op = "="
		}
		sqlOp, ok := validOps[op]
		if !ok {
			return "", fmt.Errorf("invalid operator: %q", f.Op)
		}
		switch {
		case TreeOps[op]:
			w, err := b.treeWhere(c, op, f)
			if err != nil {
				return "", err
			}
			parts = append(parts, w)
			continue
		}
		switch op {
		case "in", "not in":
			vals, ok := f.Value.([]any)
			if !ok {
				if s, isStr := f.Value.(string); isStr {
					for _, p := range strings.Split(s, ",") {
						vals = append(vals, strings.TrimSpace(p))
					}
				} else {
					vals = []any{f.Value}
				}
			}
			if len(vals) == 0 {
				if op == "in" {
					parts = append(parts, "FALSE")
				} else {
					parts = append(parts, "TRUE")
				}
				continue
			}
			ph := make([]string, len(vals))
			for i, v := range vals {
				ph[i] = b.Arg(v)
			}
			parts = append(parts, fmt.Sprintf("%s %s (%s)", c, sqlOp, strings.Join(ph, ", ")))
		case "between":
			vals, _ := f.Value.([]any)
			if len(vals) != 2 {
				return "", fmt.Errorf("between requires [start, end]")
			}
			parts = append(parts, fmt.Sprintf("%s BETWEEN %s AND %s", c, b.Arg(vals[0]), b.Arg(vals[1])))
		case "is", "set", "not set":
			v := strings.ToLower(fmt.Sprint(f.Value))
			if op == "set" || (op == "is" && v == "set") {
				parts = append(parts, fmt.Sprintf("(%s IS NOT NULL AND %s::text <> '')", c, c))
			} else {
				parts = append(parts, fmt.Sprintf("(%s IS NULL OR %s::text = '')", c, c))
			}
		case "=", "!=":
			if f.Value == nil {
				if op == "=" {
					parts = append(parts, fmt.Sprintf("(%s IS NULL OR %s::text = '')", c, c))
				} else {
					parts = append(parts, fmt.Sprintf("(%s IS NOT NULL AND %s::text <> '')", c, c))
				}
				continue
			}
			parts = append(parts, fmt.Sprintf("%s %s %s", c, sqlOp, b.Arg(f.Value)))
		case "like", "not like":
			parts = append(parts, fmt.Sprintf("%s %s %s", AccentInsensitive(c), sqlOp, AccentInsensitive(b.Arg(f.Value))))
		default:
			parts = append(parts, fmt.Sprintf("%s %s %s", c, sqlOp, b.Arg(f.Value)))
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, " AND "), nil
}

// treeWhere renders a tree operator: the set of ids the hierarchy answers with
// becomes a recursive subquery, and the filtered column is tested against it.
//
// An empty value renders FALSE (or TRUE for a negated operator), the same rule
// an empty `in` follows: no root means no descendants to match.
func (b *Builder) treeWhere(c, op string, f Filter) (string, error) {
	if f.Tree == nil {
		return "", fmt.Errorf("operator %q needs a field that is a tree DocType's id or a Link to one", op)
	}
	var roots []string
	switch v := f.Value.(type) {
	case nil:
	case []any:
		for _, x := range v {
			if s := Str(x); s != "" {
				roots = append(roots, s)
			}
		}
	case []string:
		for _, s := range v {
			if s != "" {
				roots = append(roots, s)
			}
		}
	default:
		// One id, never comma-split: a document key may contain a comma.
		if s := Str(v); s != "" {
			roots = append(roots, s)
		}
	}
	negated := strings.HasPrefix(op, "not ")
	if len(roots) == 0 {
		if negated {
			return "TRUE", nil
		}
		return "FALSE", nil
	}
	set := b.treeSet(f.Tree, op, roots)
	if negated {
		// Frappe's ifnull semantics: a row with no value is outside the
		// subtree, so a negated operator keeps it.
		return fmt.Sprintf("(%s IS NULL OR %s::text = '' OR %s NOT IN (%s))", c, c, c, set), nil
	}
	return fmt.Sprintf("%s IN (%s)", c, set), nil
}

// treeSet is the recursive subquery listing the ids an operator selects.
//
// UNION — not UNION ALL — is what makes it terminate: each id enters the result
// once, so a cycle (rows written by raw SQL, or a hierarchy built before the
// DocType declared isTree) stops instead of recursing forever.
func (b *Builder) treeSet(t *TreeRef, op string, roots []string) string {
	tab, parent, key := Ident(t.Table), Ident(t.ParentCol), Ident("id")
	arg := b.Arg(roots)
	switch op {
	case "ancestors of", "not ancestors of":
		return fmt.Sprintf(
			`WITH RECURSIVE a(id) AS (`+
				`SELECT n.%[2]s FROM %[1]s n WHERE n.%[3]s = ANY(%[4]s) AND COALESCE(n.%[2]s::text, '') <> '' `+
				`UNION SELECT n.%[2]s FROM %[1]s n JOIN a ON n.%[3]s = a.id WHERE COALESCE(n.%[2]s::text, '') <> ''`+
				`) SELECT a.id FROM a`,
			tab, parent, key, arg)
	case "descendants of (inclusive)":
		// Seeded with the values themselves, so the inclusive form still
		// matches an id that no row answers to — the same as `in`.
		return fmt.Sprintf(
			`WITH RECURSIVE d(id) AS (`+
				`SELECT unnest(%[4]s::text[]) `+
				`UNION SELECT n.%[3]s FROM %[1]s n JOIN d ON n.%[2]s = d.id`+
				`) SELECT d.id FROM d`,
			tab, parent, key, arg)
	default: // "descendants of", "not descendants of"
		return fmt.Sprintf(
			`WITH RECURSIVE d(id) AS (`+
				`SELECT n.%[3]s FROM %[1]s n WHERE n.%[2]s = ANY(%[4]s) `+
				`UNION SELECT n.%[3]s FROM %[1]s n JOIN d ON n.%[2]s = d.id`+
				`) SELECT d.id FROM d`,
			tab, parent, key, arg)
	}
}

// ParseFilters accepts the JSON shapes `[[f,op,v],...]`, `[[f,v],...]`,
// `{f: v, g: [op, v]}` and returns Filters. Field may be "DocType.field".
func ParseFilters(v any) ([]Filter, error) {
	var out []Filter
	switch x := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		for k, val := range x {
			if arr, ok := val.([]any); ok && len(arr) == 2 {
				if op, ok := arr[0].(string); ok && validOps[strings.ToLower(op)] != "" {
					out = append(out, Filter{Field: k, Op: op, Value: arr[1]})
					continue
				}
			}
			out = append(out, Filter{Field: k, Op: "=", Value: val})
		}
	case []any:
		for _, item := range x {
			arr, ok := item.([]any)
			if !ok {
				return nil, fmt.Errorf("invalid filter: %v", item)
			}
			switch len(arr) {
			case 2:
				out = append(out, Filter{Field: fmt.Sprint(arr[0]), Op: "=", Value: arr[1]})
			case 3:
				out = append(out, Filter{Field: fmt.Sprint(arr[0]), Op: fmt.Sprint(arr[1]), Value: arr[2]})
			case 4:
				out = append(out, Filter{Field: fmt.Sprint(arr[0]) + "." + fmt.Sprint(arr[1]), Op: fmt.Sprint(arr[2]), Value: arr[3]})
			default:
				return nil, fmt.Errorf("invalid filter: %v", item)
			}
		}
	default:
		return nil, fmt.Errorf("filters must be a list or object")
	}
	return out, nil
}

// ParseOrderBy validates "field asc, other desc".
func ParseOrderBy(s string, col func(string) string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	var parts []string
	for _, p := range strings.Split(s, ",") {
		toks := strings.Fields(strings.TrimSpace(p))
		if len(toks) == 0 {
			continue
		}
		c := col(strings.TrimPrefix(strings.TrimSuffix(toks[0], "`"), "`"))
		if c == "" {
			return "", fmt.Errorf("unknown field in the ordering: %q", toks[0])
		}
		dir := "ASC"
		if len(toks) > 1 {
			if d := strings.ToUpper(toks[1]); d == "DESC" {
				dir = "DESC"
			} else if d != "ASC" {
				return "", fmt.Errorf("invalid ordering: %q", p)
			}
		}
		parts = append(parts, c+" "+dir)
	}
	return strings.Join(parts, ", "), nil
}
