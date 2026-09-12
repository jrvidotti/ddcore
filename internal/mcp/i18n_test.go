package mcp

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
)

// i18nEngine builds a database-less engine over a temporary checkout with one
// app, exactly the way `ddcore i18n extract` does: extraction reads sources
// and metadata, never a row.
func i18nEngine(t *testing.T) (*engine.Engine, string) {
	t.Helper()
	root := t.TempDir()
	app := filepath.Join(root, "apps", "loja")
	write := func(rel, src string) {
		os.MkdirAll(filepath.Join(app, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(app, rel), []byte(src), 0o644)
	}
	write("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "loja", title: "Shop" });`)
	write("doctypes/produto/produto.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Produto", label: "Product",
  fields: [
    { fieldname: "codigo", fieldtype: "Data", label: "Code", reqd: true },
    { fieldname: "nome", fieldtype: "Data", label: "Name" },
  ] });`)
	e, err := engine.New(context.Background(), engine.Config{
		Apps: []js.App{{Name: "loja", Dir: app}}, Lang: "pt-BR", Root: root, LogLevel: slog.LevelError,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	return e, filepath.Join(app, "translations", "pt-BR.csv")
}

func missingKeys(r i18nReport, app string) []string {
	var out []string
	for _, t := range r.Targets {
		if t.App != app {
			continue
		}
		for _, k := range t.Missing {
			out = append(out, k.Key)
		}
	}
	return out
}

func TestI18nExtractReportsAndWrites(t *testing.T) {
	e, csvPath := i18nEngine(t)
	s := &server{e: e}

	r, err := s.i18nExtract(i18nExtractIn{App: "loja", Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Lang != "pt-BR" {
		t.Errorf("lang should default to the site's, got %q", r.Lang)
	}
	if got, want := strings.Join(missingKeys(r, "loja"), ","), "Code,Name,Product,Shop"; got != want {
		t.Errorf("missing = %s, want %s", got, want)
	}
	if _, err := os.Stat(csvPath); !os.IsNotExist(err) {
		t.Error("check must not write the catalogue")
	}

	if _, err := s.i18nExtract(i18nExtractIn{App: "loja"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal("extract should create the catalogue:", err)
	}
	if !strings.HasPrefix(string(b), "Code,,# loja.doctypes.produto.produto.doctype") {
		t.Errorf("unexpected catalogue:\n%s", b)
	}
	if _, err := s.i18nExtract(i18nExtractIn{App: "loja", Lang: "en"}); err == nil {
		t.Error("en is the source language and must be refused")
	}
	if _, err := s.i18nExtract(i18nExtractIn{App: "nope"}); err == nil {
		t.Error("an unknown app must be refused")
	}
}

func TestSetTranslationsWritesAndReloads(t *testing.T) {
	e, csvPath := i18nEngine(t)
	s := &server{e: e}

	r, err := s.setTranslations(setTranslationsIn{App: "loja", Translations: map[string]string{"Code": "Código", "Name": "Nome"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(missingKeys(r, "loja"), ","), "Product,Shop"; got != want {
		t.Errorf("missing after set = %s, want %s", got, want)
	}
	b, _ := os.ReadFile(csvPath)
	if !strings.Contains(string(b), "Code,Código,# loja.doctypes") {
		t.Errorf("catalogue should carry the translation:\n%s", b)
	}
	if got := e.Current().I18n.T("pt-BR", "Code"); got != "Código" {
		t.Errorf("the running catalogue should be reloaded, T(Code) = %q", got)
	}

	_, err = s.setTranslations(setTranslationsIn{App: "loja", Translations: map[string]string{"Product": "Produto", "Typo": "x"}})
	if err == nil || !strings.Contains(err.Error(), `"Typo"`) {
		t.Fatalf("an unknown key must be refused by name, got %v", err)
	}
	b, _ = os.ReadFile(csvPath)
	if strings.Contains(string(b), "Produto") {
		t.Error("a refused call must write nothing")
	}
	if _, err := s.setTranslations(setTranslationsIn{App: "loja"}); err == nil {
		t.Error("an empty translations map must be refused")
	}
}
