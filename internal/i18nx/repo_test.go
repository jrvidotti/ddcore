package i18nx_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/i18nx"
	"github.com/jrvidotti/ddcore/internal/js"
)

// repoRoot walks up to the directory holding ddcore.json.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, config.Name)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("no ddcore.json above the test: not a checkout")
		}
		dir = parent
	}
}

// TestCatalogPtBRComplete is the guard the CI gate is built on: it runs the
// real extractor over this checkout and fails listing what is missing and what
// is stale.
//
// Write `__("Salvar")` and this breaks, naming the file and the line — which
// is the point, because the failure it replaces is silent: an untranslated key
// simply renders as English on a Portuguese screen and nothing else goes wrong.
//
// It needs no database: extraction runs the app loader, exactly like
// `ddcore types`.
func TestCatalogPtBRComplete(t *testing.T) {
	root := repoRoot(t)
	cfg, _, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var apps []js.App
	for _, dir := range cfg.Apps {
		apps = append(apps, js.App{Name: filepath.Base(dir), Dir: dir})
	}
	e, err := engine.New(context.Background(), engine.Config{
		Apps: apps, SiteName: cfg.Site, Lang: cfg.Lang, Currency: cfg.Currency,
		Timezone: cfg.Timezone, LogLevel: slog.LevelError,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.Log = slog.New(slog.NewTextHandler(io.Discard, nil))

	targets := i18nx.Targets(e, root, "")
	if len(targets) == 0 {
		t.Fatal("no extraction target found")
	}
	for _, target := range targets {
		set, err := i18nx.Extract(e, target)
		if err != nil {
			t.Fatalf("%s: %v", target.App, err)
		}
		path := target.CatalogPath("pt-BR")
		cat, err := i18nx.ReadCatalog(path, "pt-BR")
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		if missing := cat.Missing(set); len(missing) > 0 {
			var b strings.Builder
			for _, k := range missing {
				b.WriteString("\n  " + k.Ref() + "\n    " + k.Text)
			}
			t.Errorf("%s: %d untranslated key(s). Run `make i18n` and fill them in:%s", rel, len(missing), b.String())
		}
		if orphans := cat.Orphans(set); len(orphans) > 0 {
			t.Errorf("%s: %d stale key(s), no longer in the code. Run `make i18n`:\n  %s",
				rel, len(orphans), strings.Join(orphans, "\n  "))
		}
	}
}
