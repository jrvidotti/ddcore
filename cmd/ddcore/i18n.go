package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
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
	if args[0] != "extract" {
		return fmt.Errorf("unknown i18n subcommand: %s\n\n%s", args[0], i18nUsage)
	}
	return i18nExtract(args[1:])
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
	ec, err := engineConfig(cfg, loadOpts{NoDB: true, Quiet: true})
	if err != nil {
		return err
	}
	e, err := engine.New(context.Background(), ec)
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
	// What the run found is printed even when it stopped part-way on an error.
	r, err := i18nx.BuildReport(e, targets, *lang, i18nx.ReportOpts{Write: !*check, Prune: *prune})
	for _, t := range r.Targets {
		printI18nTarget(t)
	}
	if err != nil {
		return err
	}
	if *check && r.Missing > 0 {
		return fmt.Errorf("%d untranslated key(s) in %s", r.Missing, *lang)
	}
	return nil
}

// printI18nTarget prints one catalogue's report: the summary line, then what
// needs a look.
func printI18nTarget(t i18nx.TargetReport) {
	fmt.Printf("%s: %d keys, %d missing, %d orphan, %d override\n", filepath.FromSlash(t.File), t.Keys, len(t.Missing), len(t.Orphans), len(t.Overrides))
	for _, k := range t.Missing {
		fmt.Printf("  missing  %-60q %s\n", k.Key, k.Ref)
	}
	for _, k := range t.Orphans {
		fmt.Printf("  orphan   %q\n", k)
	}
	for _, o := range t.Overrides {
		fmt.Printf("  override %q (%s)\n", o.Key, o.App)
	}
	for _, d := range t.Dynamic {
		fmt.Printf("  dynamic  %s (the value is its own key at run time)\n", d)
	}
}
