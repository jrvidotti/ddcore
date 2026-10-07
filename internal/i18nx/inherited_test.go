package i18nx

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

func writeApp(t *testing.T, name string, files map[string]string) js.App {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return js.App{Name: name, Dir: dir}
}

// The keys an app may override are the ones the apps loaded before it
// translate, in any language: the first app to have a key owns it, an
// earlier catalogue's own orphans are not keys, and the app itself and the
// apps after it own nothing it can override.
func TestInheritedKeys(t *testing.T) {
	a := writeApp(t, "a", map[string]string{
		"translations/pt-BR.csv": "Tenant,Conta,# x.ts:1\nOld,Velho,# orphan: no longer in the code\n",
		"translations/es.csv":    "Only Spanish,Sólo español,# x.ts:2\n",
	})
	b := writeApp(t, "b", map[string]string{
		"translations/pt-BR.csv": "Tenant,Organização,# overrides a\nMine,Meu,# b.ts:1\n",
	})
	c := writeApp(t, "c", map[string]string{
		"translations/pt-BR.csv": "Own,Próprio,# c.ts:1\n",
	})
	d := writeApp(t, "d", map[string]string{
		"translations/pt-BR.csv": "Later,Depois,# d.ts:1\n",
	})
	apps := []js.App{a, b, c, d}

	got := inheritedKeys(apps, "c")
	want := map[string]string{"Tenant": "a", "Only Spanish": "a", "Mine": "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inheritedKeys(c) = %v, want %v", got, want)
	}
	if got := inheritedKeys(apps, "a"); len(got) != 0 {
		t.Fatalf("the first app inherits nothing, got %v", got)
	}
}
