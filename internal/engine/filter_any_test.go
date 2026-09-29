package engine

import (
	"context"
	"slices"
	"testing"
)

// An `any` filter item ORs its groups inside the ANDed filters, so a list can
// need an OR of its own beside the one a search takes in orFilters (#35).
func TestFilterAnyGroups(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, p := range []Doc{
			{"nome": "Ana", "cpf": "1", "tipo": "PF", "limite": 10},
			{"nome": "Bia", "cpf": "2", "tipo": "PJ", "limite": 20},
			{"nome": "Caio", "cpf": "3", "tipo": "PJ", "limite": 30},
			{"nome": "Duda", "cpf": "4", "tipo": "PF", "limite": 40},
		} {
			if _, err := c.Insert(mustDoc(t, c, "Pessoa", p), SaveOpts{}); err != nil {
				return err
			}
		}
		ped := mustDoc(t, c, "Pedido", Doc{"cliente": "Duda"})
		ped["itens"] = []any{map[string]any{"descricao": "Mesa", "qtd": 1, "valor": 5}}
		_, err := c.Insert(ped, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// limite >= 20 AND (tipo = PF OR (tipo = PJ AND limite <= 20)) → Bia, Duda
	group := map[string]any{"any": []any{
		[]any{[]any{"tipo", "=", "PF"}},
		map[string]any{"tipo": "PJ", "limite": []any{"<=", 20}},
	}}
	filters := []any{[]any{"limite", ">=", 20}, group}
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		ids := func(args ListArgs) []string {
			t.Helper()
			args.Fields, args.OrderBy = []string{"id"}, "id asc"
			rows, err := c.GetList("Pessoa", args)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, r := range rows {
				out = append(out, r["id"].(string))
			}
			return out
		}
		if got := ids(ListArgs{Filters: filters}); !slices.Equal(got, []string{"Bia", "Duda"}) {
			t.Fatalf("any = %v", got)
		}
		// both ORs hold at once: the group's and the search's
		if got := ids(ListArgs{Filters: filters, OrFilters: []any{[]any{"nome", "like", "%u%"}, []any{"cpf", "=", "9"}}}); !slices.Equal(got, []string{"Duda"}) {
			t.Fatalf("any with orFilters = %v", got)
		}
		if n, err := c.Count("Pessoa", filters); err != nil || n != 2 {
			t.Fatalf("count = %d %v", n, err)
		}
		if got := ids(ListArgs{Filters: []any{map[string]any{"any": []any{}}}}); len(got) != 0 {
			t.Fatalf("an any without groups matches nothing: %v", got)
		}
		// a child table condition inside a group
		rows, err := c.GetList("Pedido", ListArgs{Fields: []string{"cliente"}, Filters: []any{map[string]any{"any": []any{
			[]any{[]any{"Item Pedido", "descricao", "=", "Mesa"}},
			[]any{[]any{"cliente", "=", "ninguém"}},
		}}}})
		if err != nil || len(rows) != 1 || rows[0]["cliente"] != "Duda" {
			t.Fatalf("child any = %v %v", rows, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The export's keyset pages keep the caller's filters as parsed, an `any`
// group included.
func TestExportKeepsAnyFilter(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 30)
	col, err := exportAs(t, e, "Admin", ExportArgs{Doctype: "Nota", Batch: 4, Filters: []any{map[string]any{"any": []any{
		[]any{[]any{"valor", "<", 3}},
		[]any{[]any{"valor", ">=", 27}},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.docs) != 6 {
		t.Fatalf("exported %v", col.names())
	}
}
