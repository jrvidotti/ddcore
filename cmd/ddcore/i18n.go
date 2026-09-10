package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/i18nx"
	"github.com/jrvidotti/ddcore/internal/js"
)

const i18nUsage = `ddcore i18n extract — rewrites translations/<lang>.csv from the code

  ddcore i18n extract [--app <name> | --all] [--lang pt-BR] [--check] [--prune]

    --app    only this app (default: every app plus the framework core)
    --all    every app plus the framework core (the default)
    --lang   which catalogue to write (default: the site's lang)
    --check  write nothing; list what is missing and what is stale, and exit
             non-zero if anything is missing
    --prune  drop catalogue entries the code no longer has
`

func cmdI18n(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(i18nUsage)
		return nil
	}
	sub, rest := args[0], args[1:]
	if sub != "extract" {
		return fmt.Errorf("unknown i18n subcommand: %s\n\n%s", sub, i18nUsage)
	}
	fs := newFlagSet("i18n extract")
	app := fs.String("app", "", "only this app")
	all := fs.Bool("all", false, "every app plus the core (the default)")
	lang := fs.String("lang", "", "catalogue language (default: the site's lang)")
	check := fs.Bool("check", false, "write nothing; report and exit non-zero if anything is missing")
	prune := fs.Bool("prune", false, "drop entries the code no longer has")
	if err := parseFlags(fs, rest); err != nil {
		return err
	}
	if *all {
		*app = ""
	}

	e, cfg, root, err := loadForExtract()
	if err != nil {
		return err
	}
	if e.DB != nil {
		defer e.DB.Close()
	}
	if *lang == "" {
		*lang = cfg.Lang
	}
	if *lang == "" || *lang == "en" {
		// English is the source language: every key already translates to
		// itself, so there is no catalogue to write.
		return fmt.Errorf("--lang: %q is the source language, it has no catalogue", "en")
	}

	targets := i18nx.Targets(e, root, *app)
	if len(targets) == 0 {
		return fmt.Errorf("no target to extract (--app %q?)", *app)
	}
	missingTotal := 0
	for _, t := range targets {
		set, err := i18nx.Extract(e, t)
		if err != nil {
			return err
		}
		path := t.CatalogPath(*lang)
		cat, err := i18nx.ReadCatalog(path, *lang)
		if err != nil {
			return err
		}
		missing, orphans := cat.Missing(set), cat.Orphans(set)
		missingTotal += len(missing)
		rel, _ := filepath.Rel(root, path)
		if rel == "" {
			rel = path
		}
		fmt.Printf("%s: %d keys, %d missing, %d orphan\n", rel, set.Len(), len(missing), len(orphans))
		for _, k := range missing {
			fmt.Printf("  missing  %-60q %s\n", k.Text, k.Ref())
		}
		for _, k := range orphans {
			fmt.Printf("  orphan   %q\n", k)
		}
		for _, d := range set.Dynamic {
			fmt.Printf("  dynamic  %s:%d (the value is its own key at run time)\n", d.File, d.Line)
		}
		if !*check {
			if err := cat.Write(set, *prune); err != nil {
				return err
			}
		}
	}
	if *check && missingTotal > 0 {
		return fmt.Errorf("%d untranslated key(s) in %s", missingTotal, *lang)
	}
	return nil
}

// loadForExtract builds the engine without a database: extraction reads the
// app sources and the registry they produce, exactly as `ddcore types` does,
// and never a row. It also returns the repo root, which is the directory
// holding ddcore.json.
func loadForExtract() (*engine.Engine, *config.File, string, error) {
	cfg, path, err := config.Load(".")
	if err != nil {
		return nil, nil, "", err
	}
	var apps []js.App
	for _, dir := range cfg.Apps {
		apps = append(apps, js.App{Name: filepath.Base(dir), Dir: dir})
	}
	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, nil, "", err
	}
	level := slog.LevelWarn
	if os.Getenv("DDCORE_DEBUG") != "" {
		level = slog.LevelDebug
	}
	e, err := engine.New(context.Background(), engine.Config{
		Apps: apps, SiteName: cfg.Site, Lang: cfg.Lang, Currency: cfg.Currency,
		Timezone: cfg.Timezone, DataDir: cfg.DataDir, LogLevel: level,
	})
	return e, cfg, root, err
}
