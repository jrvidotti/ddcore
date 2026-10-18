package main

import (
	"fmt"
	"path/filepath"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/i18nx"
)

const i18nUsage = `ddcore i18n extract — rewrites translations/<lang>.csv from the code

  ddcore i18n extract [--app <name> | --all] [--lang pt-BR] [--check] [--prune]

    --app    only this app (default: every app plus the framework core)
    --all    every app plus the framework core (the default)
    --lang   which catalogue to write (default: the site's lang)
    --check  write nothing; list what is missing and what is stale, and exit
             non-zero if anything is missing
    --prune  drop catalogue entries the code no longer has; an override of a
             key an earlier app (the core included) translates is kept
`

func cmdI18n(args []string) error {
	// -h is main's: it prints i18nUsage like any command with subcommands
	if len(args) == 0 {
		fmt.Print(i18nUsage)
		return nil
	}
	return dispatchWith([]subcommand{{"extract", i18nExtract}}, args, func(sub string) error {
		return fmt.Errorf("unknown i18n subcommand: %s\n\n%s", sub, i18nUsage)
	})
}

func i18nExtract(args []string) error {
	fs := newFlagSet("i18n extract")
	app := fs.String("app", "", "only this app")
	all := fs.Bool("all", false, "every app plus the core (the default)")
	lang := fs.String("lang", "", "catalogue language (default: the site's lang)")
	check := fs.Bool("check", false, "write nothing; report and exit non-zero if anything is missing")
	prune := fs.Bool("prune", false, "drop entries the code no longer has")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *all {
		*app = ""
	}

	// Without a database: extraction reads the app sources and the registry
	// they produce, exactly as `ddcore types` does, and never a row.
	cfg, _, err := config.Load(".")
	if err != nil {
		return err
	}
	e, err := newEngine(cfg, loadOpts{NoDB: true, Quiet: true})
	if err != nil {
		return err
	}
	root := e.Cfg.Root // the directory holding ddcore.json
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
		missing, orphans, overrides := cat.Missing(set), cat.Orphans(set), cat.Overrides(set)
		missingTotal += len(missing)
		rel, _ := filepath.Rel(root, path)
		if rel == "" {
			rel = path
		}
		fmt.Printf("%s: %d keys, %d missing, %d orphan, %d override\n", rel, set.Len(), len(missing), len(orphans), len(overrides))
		for _, k := range missing {
			fmt.Printf("  missing  %-60q %s\n", k.Text, k.Ref())
		}
		for _, k := range orphans {
			fmt.Printf("  orphan   %q\n", k)
		}
		for _, k := range overrides {
			fmt.Printf("  override %q (%s)\n", k, set.Inherited[k])
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
