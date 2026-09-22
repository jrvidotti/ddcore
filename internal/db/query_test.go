package db

import (
	"strings"
	"testing"
)

func col(field string) string {
	if field == "id" || field == "territory" {
		return `"t".` + Ident(field)
	}
	return ""
}

func treeFilter(op string, value any) Filter {
	return Filter{Field: "id", Op: op, Value: value,
		Tree: &TreeRef{Table: "tab_territory", ParentCol: "parent_territory"}}
}

func TestTreeOperatorsRenderRecursiveSubquery(t *testing.T) {
	cases := []struct {
		op    string
		want  []string
		avoid []string
	}{
		{op: "descendants of",
			want: []string{`"t"."id" IN (WITH RECURSIVE`, `n."parent_territory" = ANY($1)`, `JOIN d ON n."parent_territory" = d.id`}},
		{op: "descendants of (inclusive)",
			want: []string{`SELECT unnest($1::text[])`, `JOIN d ON n."parent_territory" = d.id`}},
		{op: "ancestors of",
			want: []string{`SELECT n."parent_territory" FROM "tab_territory" n WHERE n."id" = ANY($1)`, `JOIN a ON n."id" = a.id`}},
		{op: "not descendants of",
			want: []string{`"t"."id" IS NULL`, `"t"."id"::text = ''`, `"t"."id" NOT IN (WITH RECURSIVE`}},
		{op: "not ancestors of",
			want: []string{`NOT IN (WITH RECURSIVE a(id)`}},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			b := &Builder{}
			got, err := b.Where([]Filter{treeFilter(tc.op, "Brazil")}, col)
			if err != nil {
				t.Fatalf("where: %v", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Fatalf("missing %q in:\n%s", w, got)
				}
			}
			if len(b.Args) != 1 {
				t.Fatalf("args = %v", b.Args)
			}
			if roots, ok := b.Args[0].([]string); !ok || len(roots) != 1 || roots[0] != "Brazil" {
				t.Fatalf("roots arg = %#v", b.Args[0])
			}
		})
	}
}

func TestTreeOperatorAcceptsList(t *testing.T) {
	b := &Builder{}
	if _, err := b.Where([]Filter{treeFilter("descendants of", []any{"Brazil", "", "Chile"})}, col); err != nil {
		t.Fatalf("where: %v", err)
	}
	roots, _ := b.Args[0].([]string)
	if len(roots) != 2 || roots[1] != "Chile" {
		t.Fatalf("roots = %#v (empty values are dropped)", b.Args[0])
	}
}

// A document key may contain a comma, so a single string is one id — unlike
// `in`, which splits.
func TestTreeOperatorDoesNotSplitOnComma(t *testing.T) {
	b := &Builder{}
	if _, err := b.Where([]Filter{treeFilter("descendants of", "North, South")}, col); err != nil {
		t.Fatalf("where: %v", err)
	}
	roots, _ := b.Args[0].([]string)
	if len(roots) != 1 || roots[0] != "North, South" {
		t.Fatalf("roots = %#v", b.Args[0])
	}
}

func TestTreeOperatorEmptyValue(t *testing.T) {
	for op, want := range map[string]string{
		"descendants of":     "FALSE",
		"ancestors of":       "FALSE",
		"not descendants of": "TRUE",
		"not ancestors of":   "TRUE",
	} {
		b := &Builder{}
		got, err := b.Where([]Filter{treeFilter(op, []any{})}, col)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		if got != want {
			t.Fatalf("%s rendered %q, want %q", op, got, want)
		}
		if len(b.Args) != 0 {
			t.Fatalf("%s bound arguments for an empty value", op)
		}
	}
}

// Without a Tree the operator has no hierarchy to walk. Refusing beats the old
// behaviour, which compiled `descendants of` into `=`.
func TestTreeOperatorWithoutTreeRefails(t *testing.T) {
	b := &Builder{}
	_, err := b.Where([]Filter{{Field: "id", Op: "descendants of", Value: "Brazil"}}, col)
	if err == nil {
		t.Fatal("a tree operator without a TreeRef was accepted")
	}
	if !strings.Contains(err.Error(), "tree") {
		t.Fatalf("error does not say what is missing: %v", err)
	}
}

// The operator applies to a Link column as well as to the tree's own id.
func TestTreeOperatorOnLinkColumn(t *testing.T) {
	b := &Builder{}
	got, err := b.Where([]Filter{{Field: "territory", Op: "descendants of (inclusive)", Value: "Brazil",
		Tree: &TreeRef{Table: "tab_territory", ParentCol: "parent_territory"}}}, col)
	if err != nil {
		t.Fatalf("where: %v", err)
	}
	if !strings.HasPrefix(got, `"t"."territory" IN (WITH RECURSIVE`) {
		t.Fatalf("got %s", got)
	}
}
