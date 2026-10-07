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

// A key the app's code does not have but an earlier app's catalogue does is
// the app overriding that app's translation: not an orphan, and --prune keeps
// it (#106).
func TestCatalogOverrideOfAnEarlierAppsKey(t *testing.T) {
	s := NewSet()
	s.Add("Save", "x.ts", 1)
	s.Inherited["Tenant"] = "core"
	c := &Catalog{Path: filepath.Join(t.TempDir(), "pt-BR.csv"), Lang: "pt-BR", Trans: map[string]string{
		"Save": "Salvar", "Tenant": "Organização", "Gone": "Sumiu",
	}}
	if got := c.Orphans(s); len(got) != 1 || got[0] != "Gone" {
		t.Errorf("Orphans = %v, want [Gone]", got)
	}
	if got := c.Overrides(s); len(got) != 1 || got[0] != "Tenant" {
		t.Errorf("Overrides = %v, want [Tenant]", got)
	}

	if err := c.Write(s, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(c.Path)
	want := "Save,Salvar,# x.ts:1\nTenant,Organização,# overrides core\nGone,Sumiu,# orphan: no longer in the code\n"
	if string(b) != want {
		t.Fatalf("wrote:\n%s\nwant:\n%s", b, want)
	}

	if err := c.Write(s, true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(c.Path)
	want = "Save,Salvar,# x.ts:1\nTenant,Organização,# overrides core\n"
	if string(b) != want {
		t.Fatalf("with --prune wrote:\n%s\nwant:\n%s", b, want)
	}
}

func TestCatalogSetAcceptsAnOverride(t *testing.T) {
	s := NewSet()
	s.Add("Save", "x.ts", 1)
	s.Inherited["Tenant"] = "core"
	c := &Catalog{Path: filepath.Join(t.TempDir(), "pt-BR.csv"), Lang: "pt-BR", Trans: map[string]string{}}
	if err := c.Set(s, map[string]string{"Tenant": "Organização"}); err != nil {
		t.Fatalf("an earlier app's key is the app's to override: %v", err)
	}
	if err := c.Set(s, map[string]string{"Nope": "Não"}); err == nil {
		t.Fatal("a key neither the code nor an earlier app has must still be refused")
	}
	b, _ := os.ReadFile(c.Path)
	if want := "Save,,# x.ts:1\nTenant,Organização,# overrides core\n"; string(b) != want {
		t.Fatalf("wrote:\n%s\nwant:\n%s", b, want)
	}
}
