package main

// `ddcore import` — the loading side of a migration (DAT-01).
//
// It reads what `ddcore export` wrote and puts it into this site, keeping the
// identities and the metadata and replaying none of the effects. The four
// subcommands are the four questions an operator asks, in order: what would
// this load (`plan`), does it load (`run --dry-run`), load it (`run`, resumed
// with `--resume`), and does the site now hold what the export held
// (`reconcile`).

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"crypto/sha256"
	"encoding/hex"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

const importUsage = `ddcore import — load an export directory into this site

  plan       what would be loaded, in what order, and what is left out
  run        load it (--dry-run to rehearse, --resume <id> to continue)
  status     the runs this site has seen: import status [<id>]
  reconcile  compare the site with the export it was loaded from

Options:
  --map f.json      rename DocTypes and fields, drop, set, remap ids and users
  --dry-run         validate everything and write nothing
  --batch N         lines per transaction (default 500)
  --only a,b        load just these DocTypes
  --include a,b     load these even though they are excluded by default
  --resume <id>     continue a run that stopped
  --max-batches N   stop after N batches, leaving the run resumable
  --maintenance     pause the site for the load, and let it back in afterwards
  --verify-bytes    reconcile: read every stored attachment back
  --json            print the report as JSON
  --report f.json   write the report to a file

A load keeps ids, owners and timestamps, runs no controller hook, and queues no
webhook, notification or email: the documents are history, and their effects
already happened. Two runs over the same directory load it once.`

func cmdImport(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", importUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "plan", "run", "reconcile":
		return importCommand(sub, rest)
	case "status":
		return importStatus(rest)
	default:
		return fmt.Errorf("unknown subcommand: %s\n\n%s", sub, importUsage)
	}
}

func importFlags() (*flag.FlagSet, *importOpts) {
	fs := newFlagSet("import")
	o := &importOpts{}
	fs.StringVar(&o.mapFile, "map", "", "mapping file")
	fs.BoolVar(&o.dryRun, "dry-run", false, "write nothing")
	fs.IntVar(&o.batch, "batch", 0, "lines per transaction")
	fs.StringVar(&o.only, "only", "", "load just these DocTypes")
	fs.StringVar(&o.include, "include", "", "load these although they are excluded by default")
	fs.StringVar(&o.resume, "resume", "", "continue this run")
	fs.IntVar(&o.maxBatches, "max-batches", 0, "stop after this many batches")
	fs.BoolVar(&o.maintenance, "maintenance", false, "pause the site for the load")
	fs.BoolVar(&o.verifyBytes, "verify-bytes", false, "read every stored attachment back")
	fs.BoolVar(&o.asJSON, "json", false, "print the report as JSON")
	fs.StringVar(&o.report, "report", "", "write the report to this file")
	return fs, o
}

type importOpts struct {
	mapFile     string
	dryRun      bool
	batch       int
	only        string
	include     string
	resume      string
	maxBatches  int
	maintenance bool
	verifyBytes bool
	asJSON      bool
	report      string
}

func importCommand(sub string, args []string) error {
	fs, o := importFlags()
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("give one export directory\n\n%s", importUsage)
	}
	dir := fs.Arg(0)

	a := engine.ImportArgs{
		Dir: dir, DryRun: o.dryRun || sub == "plan", Batch: o.batch, Resume: o.resume,
		MaxBatches: o.maxBatches, VerifyBytes: o.verifyBytes, Actor: cliActor(),
		Only: splitList(o.only), Include: splitList(o.include),
	}
	if o.mapFile != "" {
		b, err := os.ReadFile(o.mapFile)
		if err != nil {
			return err
		}
		m, err := engine.ParseImportMap(b)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		a.Map, a.MapSHA = m, hex.EncodeToString(sum[:])
	}

	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if o.maintenance && !a.DryRun {
		prev := e.Maintenance(ctx)
		if !prev.Enabled {
			if _, err := e.SetMaintenance(ctx, true, "Import in progress", cliActor()); err != nil {
				return err
			}
			defer func() {
				if _, err := e.SetMaintenance(context.Background(), false, "", cliActor()); err != nil {
					fmt.Fprintln(os.Stderr, "warning: could not leave maintenance mode:", db.RedactError(err))
				}
			}()
			fmt.Println("maintenance on; waiting for servers to pause…")
			sleepCtx(ctx, 3*time.Second)
		}
	}

	if sub == "reconcile" {
		rep, err := e.ImportReconcile(ctx, a)
		if err != nil {
			return err
		}
		if err := importReport(o, rep); err != nil {
			return err
		}
		printReconciliation(o, rep)
		if !rep.OK {
			return fmt.Errorf("the site and the export do not agree: %d mismatches", len(rep.Mismatches))
		}
		return nil
	}

	run, err := e.Import(ctx, a)
	if err != nil {
		return err
	}
	if err := importReport(o, run); err != nil {
		return err
	}
	printRun(sub, o, run)
	if run.Status == engine.ImportCompletedWithErrors {
		return fmt.Errorf("the load finished with findings; see `ddcore import status %s`", run.ID)
	}
	return nil
}

func importStatus(args []string) error {
	fs := newFlagSet("import status")
	asJSON := fs.Bool("json", false, "print as JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		if fs.NArg() == 1 {
			run, err := e.ImportRunByID(ctx, fs.Arg(0))
			if err != nil {
				return err
			}
			errs, err := e.ImportRunErrors(ctx, run.ID, 50)
			if err != nil {
				return err
			}
			run.Errors = errs
			if *asJSON {
				return printJSON(run)
			}
			printRun("status", &importOpts{}, run)
			return nil
		}
		runs, err := e.ImportRuns(ctx, 20)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(runs)
		}
		if len(runs) == 0 {
			fmt.Println("no import has run on this site")
			return nil
		}
		fmt.Printf("%-12s %-22s %-12s %s\n", "ID", "STARTED", "STATUS", "DIRECTORY")
		for _, r := range runs {
			fmt.Printf("%-12s %-22s %-12s %s\n", r.ID, r.Started.Format("2006-01-02 15:04:05"), r.Status, r.Dir)
		}
		return nil
	})
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func importReport(o *importOpts, v any) error {
	if o.report == "" {
		return nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(o.report, append(b, '\n'), 0o644)
}

func printRun(sub string, o *importOpts, run *engine.ImportRun) {
	if o.asJSON {
		printJSON(run)
		return
	}
	if sub == "plan" {
		fmt.Printf("would load, in this order: %s\n", strings.Join(run.Order, " → "))
	}
	fmt.Printf("run %s — %s\n\n", run.ID, run.Status)
	names := make([]string, 0, len(run.Counts))
	for n := range run.Counts {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) > 0 {
		fmt.Printf("%-28s %7s %8s %7s %6s\n", "DOCTYPE", "ROWS", "LOADED", "SKIPPED", "ERRORS")
		for _, n := range names {
			c := run.Counts[n]
			fmt.Printf("%-28s %7d %8d %7d %6d\n", n, c.Rows, c.Loaded, c.Skipped, c.Errors)
		}
		fmt.Println()
	}
	for _, x := range run.Excluded {
		fmt.Printf("left out: %-24s %s\n", x.Doctype, x.Reason)
	}
	for _, n := range dedupe(run.Notes) {
		fmt.Println("note:", n)
	}
	for _, x := range run.Errors {
		fmt.Printf("error: %s line %d %s: %s\n", x.Doctype, x.Line, x.ID, x.Message)
	}
	for _, d := range run.Dangling {
		fmt.Printf("dangling: %s %s.%s → %s %s\n", d.Doctype, d.ID, d.Field, d.Target, d.TargetID)
	}
	if run.Status == engine.ImportPaused {
		fmt.Printf("\npaused; continue with: ddcore import run %s --resume %s\n", run.Dir, run.ID)
	}
}

func printReconciliation(o *importOpts, rep *engine.ImportReconciliation) {
	if o.asJSON {
		printJSON(rep)
		return
	}
	names := make([]string, 0, len(rep.Doctypes))
	for n := range rep.Doctypes {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Printf("%-28s %10s %10s %8s\n", "DOCTYPE", "IN EXPORT", "ON SITE", "FILES")
	for _, n := range names {
		d := rep.Doctypes[n]
		fmt.Printf("%-28s %10d %10d %8d\n", n, d.SourceRows, d.TargetRows, d.Files)
	}
	if len(rep.Mismatches) == 0 {
		fmt.Println("\nthe site holds what the export held")
		return
	}
	fmt.Println()
	for _, m := range rep.Mismatches {
		fmt.Printf("%-10s %-24s %-32s export %s, site %s\n", m.Kind, m.Doctype, m.Detail, m.Source, m.Target)
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
