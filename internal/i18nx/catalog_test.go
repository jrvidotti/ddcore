package i18nx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogSetRefusesUnknownKeys(t *testing.T) {
	s := NewSet()
	s.Add("Save", "x.ts", 1)
	c := &Catalog{Path: filepath.Join(t.TempDir(), "pt-BR.csv"), Lang: "pt-BR", Trans: map[string]string{}}
	err := c.Set(s, map[string]string{"Save": "Salvar", "Save ": "Salvar", "Nope": "Não"})
	if err == nil {
		t.Fatal("expected an error for keys the code does not have")
	}
	for _, k := range []string{`"Save "`, `"Nope"`} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error should name %s: %v", k, err)
		}
	}
	if c.Trans["Save"] != "" {
		t.Error("a refused call must not apply any translation")
	}
	if _, err := os.Stat(c.Path); !os.IsNotExist(err) {
		t.Error("a refused call must not write the catalogue")
	}
}

func TestCatalogSetWritesCanonicalCSV(t *testing.T) {
	s := NewSet()
	s.Add("Save", "x.ts", 1)
	s.Add("Cancel", "x.ts", 2)
	c := &Catalog{Path: filepath.Join(t.TempDir(), "pt-BR.csv"), Lang: "pt-BR", Trans: map[string]string{}}
	if err := c.Set(s, map[string]string{"Save": "Salvar"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Set(s, map[string]string{"Cancel": "Cancelar"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := "Cancel,Cancelar,# x.ts:2\nSave,Salvar,# x.ts:1\n"
	if string(b) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b, want)
	}
	if missing := c.Missing(s); len(missing) != 0 {
		t.Errorf("nothing should be missing, got %v", missing)
	}
}
