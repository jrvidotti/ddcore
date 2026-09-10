package db

import (
	"fmt"
	"regexp"
	"strings"
)

// Filter is one condition: [field, op, value] — field may be "Child DocType.field"
// resolved by the caller into a table join.
type Filter struct {
	Field string
	Op    string
	Value any
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

// Ident quotes a Postgres identifier after validating it.
func Ident(s string) string {
	if !identRe.MatchString(s) {
		panic(fmt.Sprintf("identificador inválido: %q", s))
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
			return "", fmt.Errorf("campo desconhecido no filtro: %q", f.Field)
		}
		op := strings.ToLower(strings.TrimSpace(f.Op))
		if op == "" {
			op = "="
		}
		sqlOp, ok := validOps[op]
		if !ok {
			return "", fmt.Errorf("operador inválido: %q", f.Op)
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
				return "", fmt.Errorf("between precisa de [inicio, fim]")
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
					out = append(out, Filter{k, op, arr[1]})
					continue
				}
			}
			out = append(out, Filter{k, "=", val})
		}
	case []any:
		for _, item := range x {
			arr, ok := item.([]any)
			if !ok {
				return nil, fmt.Errorf("filtro inválido: %v", item)
			}
			switch len(arr) {
			case 2:
				out = append(out, Filter{fmt.Sprint(arr[0]), "=", arr[1]})
			case 3:
				out = append(out, Filter{fmt.Sprint(arr[0]), fmt.Sprint(arr[1]), arr[2]})
			case 4:
				out = append(out, Filter{fmt.Sprint(arr[0]) + "." + fmt.Sprint(arr[1]), fmt.Sprint(arr[2]), arr[3]})
			default:
				return nil, fmt.Errorf("filtro inválido: %v", item)
			}
		}
	default:
		return nil, fmt.Errorf("filtros devem ser lista ou objeto")
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
			return "", fmt.Errorf("campo desconhecido na ordenação: %q", toks[0])
		}
		dir := "ASC"
		if len(toks) > 1 {
			if d := strings.ToUpper(toks[1]); d == "DESC" {
				dir = "DESC"
			} else if d != "ASC" {
				return "", fmt.Errorf("ordenação inválida: %q", p)
			}
		}
		parts = append(parts, c+" "+dir)
	}
	return strings.Join(parts, ", "), nil
}
