package engine

import (
	"context"
	"testing"
)

// A Link with a linkTitleField shows that field of the target as the title,
// keyed apart from the target's own title; the search returns, matches and
// ranks on it; and no endpoint reads a column no Link declares.
func TestLinkTitleField(t *testing.T) {
	e := setup(t)
	e.Current().Meta.DocTypes["Pedido"].Field("cliente").LinkTitleField = "cpf"
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, p := range []Doc{{"nome": "Maria", "cpf": "111"}, {"nome": "Joana", "cpf": "222"}} {
			doc, _ := c.NewDoc("Pessoa", p)
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		ped, _ := c.NewDoc("Pedido", Doc{"cliente": "Maria"})
		if _, err := c.Insert(ped, SaveOpts{}); err != nil {
			return err
		}
		titles := c.ResolveLinkTitles("Pedido", Doc{"cliente": "Maria"})
		if titles["Pessoa|cpf"]["Maria"] != "111" {
			t.Errorf("cliente's title is its cpf: %v", titles)
		}
		if _, plain := titles["Pessoa"]; plain {
			t.Errorf("the overridden Link must not fill the target's own key: %v", titles)
		}

		m, err := c.LinkTitlesBy("Pessoa", "cpf", []string{"Joana", "Nobody"})
		if err != nil || m["Joana"] != "222" || m["Nobody"] != "Nobody" {
			t.Errorf("LinkTitlesBy = %v, %v", m, err)
		}
		if _, err := c.LinkTitlesBy("Pessoa", "email", []string{"Joana"}); err == nil {
			t.Error("a column no Link declares must be refused")
		}

		rows, err := c.LinkSearchTitled("Pessoa", "cpf", "222", nil, 20)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["id"] != "Joana" || rows[0]["cpf"] != "222" {
			t.Errorf("search by the title field = %v", rows)
		}
		if _, err := c.LinkSearchTitled("Pessoa", "segredo", "", nil, 20); err == nil {
			t.Error("a search must refuse a column no Link declares")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
