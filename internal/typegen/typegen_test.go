package typegen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/cerne/internal/meta"
)

func TestWriteMaterializesEmbeddedSDKsForStandaloneApp(t *testing.T) {
	appDir := t.TempDir()
	reg := meta.NewRegistry()
	if err := reg.Add(&meta.DocType{
		Name:  "Tarefa",
		App:   "exemplo",
		Label: "Tarefa",
		Fields: []*meta.Field{
			{Fieldname: "titulo", Fieldtype: "Data", Label: "Título"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := Write(appDir, reg); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		".cerne/types.d.ts",
		".cerne/sdk/index.ts",
		".cerne/sdk/test.ts",
		".cerne/sdk/types.ts",
		".cerne/desk-sdk/index.ts",
	} {
		if _, err := os.Stat(filepath.Join(appDir, rel)); err != nil {
			t.Errorf("generated file %s: %v", rel, err)
		}
	}

	b, err := os.ReadFile(filepath.Join(appDir, "tsconfig.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		CompilerOptions struct {
			Paths map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"@cerne/sdk":      ".cerne/sdk/index.ts",
		"@cerne/sdk/test": ".cerne/sdk/test.ts",
		"@cerne/desk-sdk": ".cerne/desk-sdk/index.ts",
	}
	for module, path := range want {
		got := cfg.CompilerOptions.Paths[module]
		if len(got) != 1 || got[0] != path {
			t.Errorf("paths[%q] = %v, want [%q]", module, got, path)
		}
	}
}

func TestWritePreservesExistingTSConfig(t *testing.T) {
	appDir := t.TempDir()
	path := filepath.Join(appDir, "tsconfig.json")
	const custom = `{"extends":"./custom.json"}`
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(appDir, meta.NewRegistry()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != custom {
		t.Fatalf("existing tsconfig was overwritten: %s", b)
	}
}
