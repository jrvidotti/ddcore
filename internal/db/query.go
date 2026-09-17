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
}

var validOps = map[string]string{
	"=": "=", "!=": "<>", ">": ">", ">=": ">=", "<": "<", "<=": "<=",
	"like": "ILIKE", "not like": "NOT ILIKE", "in": "IN", "not in": "NOT IN",
	"between": "BETWEEN", "is": "IS", "descendants of": "=", "set": "set", "not set": "not set",
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
