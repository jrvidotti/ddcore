package api

import (
	"fmt"
	"strings"
	"testing"
)

func workspaceNames(list []map[string]any) string {
	var out []string
	for _, ws := range list {
		out = append(out, fmt.Sprint(ws["name"]))
	}
	return strings.Join(out, ",")
}

func TestSortWorkspaces_ByShownLabel(t *testing.T) {
	list := []map[string]any{
		{"name": "Vendas", "label": "Vendas"},
		{"name": "estoque", "label": "estoque"},
		{"name": "Agil", "label": "Ágil"},
		{"name": "Core"}, // no label: the desk shows the name
		{"name": "Banco", "label": "Banco"},
		{"name": "Zeta", "label": "Banco"}, // same label: the name breaks the tie
	}
	sortWorkspaces(list, "pt-BR")
	if got := workspaceNames(list); got != "Agil,Banco,Zeta,Core,estoque,Vendas" {
		t.Fatalf("order %s", got)
	}
}

func TestSortWorkspaces_UnknownLangStillSorts(t *testing.T) {
	list := []map[string]any{{"name": "B"}, {"name": "a"}}
	sortWorkspaces(list, "not a language")
	if got := workspaceNames(list); got != "a,B" {
		t.Fatalf("order %s", got)
	}
}
