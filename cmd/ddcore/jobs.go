package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const jobsUsage = `usage: ddcore jobs <subcommand>

  list       the queue, newest first (--status, --queue, --method, --user, --since, --limit, --json)
  show       one job, with its arguments and result: jobs show <id> [--json]
  stats      per-queue and per-method metrics (--window minutes, --json)
  retry      queue a failed job again: jobs retry <id>... | --failed [--queue q] [--since 1h]
             [--limit N] [--force] [--dry-run]
  cancel     stop a job: jobs cancel <id>...
  purge      delete old finished jobs (--done-days, --failed-days, --dry-run)
  scheduled  what the scheduler would run
  run        run a function inline, without queueing it: jobs run <fn>
  work       start workers and the scheduler

Arguments and results are printed by ` + "`show`" + ` only, and never by the HTTP API:
a queued password-reset mail carries its own recovery link in its arguments.`

func cmdJobs(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", jobsUsage)
	}
	sub, rest := args[0], args[1:]

	// `scheduled` and `run` and `work` keep their old shape; everything else is
	// a query or a write against ddcore_job.
	switch sub {
	case "list":
		return jobsList(rest)
	case "show":
		return jobsShow(rest)
	case "stats":
		return jobsStats(rest)
	case "retry":
		return jobsRetry(rest)
	case "cancel":
		return jobsCancel(rest)
	case "purge":
		return jobsPurge(rest)
	case "scheduled":
		return withEngine(func(e *engine.Engine, _ context.Context) error {
			for _, s := range e.ScheduledMethods() {
				fmt.Println(s)
			}
			return nil
		})
	case "run":
		if len(rest) < 1 {
			return fmt.Errorf("usage: ddcore jobs run <fn>")
		}
		return withEngine(func(e *engine.Engine, ctx context.Context) error {
			res, err := e.RunJob(ctx, "Administrator", rest[0], nil)
			if err != nil {
				return err
			}
			fmt.Println(string(res))
			return nil
		})
	case "work":
		return jobsWork()
	default:
		return fmt.Errorf("unknown subcommand: %s\n\n%s", sub, jobsUsage)
	}
}

// withEngine opens the engine, runs fn and closes the pool. Every jobs
// subcommand needs the same three lines and none of them needs the dev server.
func withEngine(fn func(*engine.Engine, context.Context) error) error {
	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	return fn(e, context.Background())
}

func jobsWork() error {
	e, cfg, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	for i := 0; i < cfg.Workers; i++ {
		go e.Worker(ctx, i)
	}
	if cfg.Scheduler {
		e.StartScheduler(ctx)
	}
	<-ctx.Done()
	return nil
}

// jobFilterFlags declares the filter options shared by `list` and `retry
// --failed`, so the two cannot drift into selecting different jobs from the
// same words.
type jobFilterFlags struct {
	status, queue, method, user, since *string
	limit                              *int
}

func addFilterFlags(fs *flag.FlagSet) jobFilterFlags {
	return jobFilterFlags{
		status: fs.String("status", "", "queued, running, done, failed or cancelled (comma-separated)"),
		queue:  fs.String("queue", "", "queue name"),
		method: fs.String("method", "", "exact dotted method path"),
		user:   fs.String("user", "", "the user the job runs as"),
		since:  fs.String("since", "", "only jobs enqueued in the last duration, e.g. 90m or 24h"),
		limit:  fs.Int("limit", 20, "how many jobs"),
	}
}

func (f jobFilterFlags) filter() (engine.JobFilter, error) {
	out := engine.JobFilter{
		Queue: *f.queue, Method: *f.method, User: *f.user, Limit: *f.limit,
	}
	if *f.status != "" {
		for _, s := range strings.Split(*f.status, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out.Status = append(out.Status, s)
			}
		}
	}
	if *f.since != "" {
		d, err := time.ParseDuration(*f.since)
		if err != nil {
			return out, fmt.Errorf("--since: %w", err)
		}
		t := time.Now().Add(-d)
		out.Since = &t
	}
	return out, nil
}

func jobsList(args []string) error {
	fs := newFlagSet("jobs list")
	f := addFilterFlags(fs)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	filter, err := f.filter()
	if err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		rows, err := e.ListJobs(ctx, filter)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(rows)
		}
		if len(rows) == 0 {
			fmt.Println("no jobs match")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tQUEUE\tMETHOD\tATTEMPTS\tENQUEUED\tERROR")
		for _, r := range rows {
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v/%v\t%s\t%s\n",
				r["id"], r["status"], r["queue"], r["method"],
				r["attempts"], r["max_attempts"], shortTime(r["enqueued"]), firstLine(r["error"]))
		}
		return w.Flush()
	})
}

func jobsShow(args []string) error {
	fs := newFlagSet("jobs show")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ids, err := jobIDs(fs.Args())
	if err != nil {
		return err
	}
	if len(ids) != 1 {
		return fmt.Errorf("usage: ddcore jobs show <id>")
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		// The only read path that returns the payload, and deliberately the one
		// that needs the database to reach at all.
		j, err := e.GetJob(ctx, ids[0], true)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(j)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, k := range []string{"id", "method", "queue", "status", "user", "enqueued",
			"run_after", "started", "finished", "attempts", "max_attempts", "timeout_seconds",
			"request_id", "cancel_requested", "cancelled_by", "retry_of", "retried_as",
			"error", "args", "result"} {
			if v, ok := j[k]; ok && v != nil {
				fmt.Fprintf(w, "%s\t%v\n", k, v)
			}
		}
		return w.Flush()
	})
}

func jobsStats(args []string) error {
	fs := newFlagSet("jobs stats")
	window := fs.Int("window", 60, "window in minutes for the per-method numbers")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		s, err := e.JobStats(ctx, time.Duration(*window)*time.Minute)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(s)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "QUEUE\tQUEUED\tRUNNING\tDONE\tFAILED\tCANCELLED")
		for _, q := range s.Queues {
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\n",
				q.Queue, q.Queued, q.Running, q.Done, q.Failed, q.Cancelled)
		}
		fmt.Fprintf(w, "\nMETHOD (last %dm)\tRUNS\tFAILURES\tAVG s\tMAX s\n", s.WindowMinutes)
		for _, m := range s.Methods {
			fmt.Fprintf(w, "%s\t%d\t%d\t%.1f\t%.1f\n",
				m.Method, m.Runs, m.Failures, m.AvgSeconds, m.MaxSeconds)
		}
		return w.Flush()
	})
}

func jobsRetry(args []string) error {
	fs := newFlagSet("jobs retry")
	f := addFilterFlags(fs)
	failed := fs.Bool("failed", false, "retry every failed job matching the filters")
	force := fs.Bool("force", false, "retry again a job that was already retried")
	dry := fs.Bool("dry-run", false, "report what would be retried")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ids, err := jobIDs(fs.Args())
	if err != nil {
		return err
	}
	if len(ids) == 0 && !*failed {
		return fmt.Errorf("usage: ddcore jobs retry <id>... | --failed [filters]")
	}
	if len(ids) > 0 && *failed {
		return fmt.Errorf("give ids or --failed, not both")
	}
	filter, err := f.filter()
	if err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		if *failed {
			// Only failures are swept up in bulk. Cancelled jobs were stopped on
			// purpose, and bringing them all back is never what "retry the
			// failures" meant.
			filter.Status = []string{"failed"}
			rows, err := e.ListJobs(ctx, filter)
			if err != nil {
				return err
			}
			for _, r := range rows {
				ids = append(ids, int64(toFloat64(r["id"])))
			}
			if len(ids) == 0 {
				fmt.Println("no failed job matches")
				return nil
			}
		}
		for _, id := range ids {
			if *dry {
				fmt.Printf("would retry %d\n", id)
				continue
			}
			act, err := e.RetryJob(ctx, id, *force)
			if err != nil {
				// One refusal must not abandon the rest of a bulk recovery.
				fmt.Fprintf(os.Stderr, "job %d: %v\n", id, err)
				continue
			}
			fmt.Printf("job %d queued again as %d\n", id, act.NewID)
		}
		return nil
	})
}

func jobsCancel(args []string) error {
	fs := newFlagSet("jobs cancel")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ids, err := jobIDs(fs.Args())
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("usage: ddcore jobs cancel <id>...")
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		for _, id := range ids {
			act, err := e.CancelJob(ctx, id, "Administrator")
			if err != nil {
				fmt.Fprintf(os.Stderr, "job %d: %v\n", id, err)
				continue
			}
			switch act.Status {
			case "cancelled":
				fmt.Printf("job %d cancelled before it ran\n", id)
			default:
				// Not "cancelled": the worker has been told and has not answered.
				fmt.Printf("job %d is running; asked it to stop\n", id)
			}
		}
		return nil
	})
}

func jobsPurge(args []string) error {
	fs := newFlagSet("jobs purge")
	done := fs.Int("done-days", -1, "delete done jobs finished more than N days ago")
	failed := fs.Int("failed-days", -1, "delete failed and cancelled jobs finished more than N days ago")
	dry := fs.Bool("dry-run", false, "report what would be deleted")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		// Unset on the command line means "use what the site configured", which
		// is also what the nightly sweep uses. -1 and not 0 because 0 is a real
		// answer here: keep forever.
		o := engine.PurgeOpts{
			DoneDays:   e.Cfg.Ops.DoneRetentionDays(),
			FailedDays: e.Cfg.Ops.FailedRetentionDays(),
			DryRun:     *dry,
		}
		if *done >= 0 {
			o.DoneDays = *done
		}
		if *failed >= 0 {
			o.FailedDays = *failed
		}
		n, err := e.PurgeJobs(ctx, o)
		if err != nil {
			return err
		}
		verb := "deleted"
		if *dry {
			verb = "would delete"
		}
		fmt.Printf("%s %d done and %d failed/cancelled job(s)\n", verb, n.Done, n.Failed)
		return nil
	})
}

func jobIDs(args []string) ([]int64, error) {
	var out []int64
	for _, a := range args {
		n, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a job id", a)
		}
		out = append(out, n)
	}
	return out, nil
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// shortTime keeps the listing to one screen; the full timestamp is in `show`.
//
// db.Select hands timestamps back as RFC3339 strings, not time.Time, so the
// string case is the one that actually runs.
func shortTime(v any) string {
	switch t := v.(type) {
	case time.Time:
		return t.Local().Format("2006-01-02 15:04")
	case string:
		if p, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return p.Local().Format("2006-01-02 15:04")
		}
		return t
	}
	return ""
}

// firstLine keeps a multi-line stack from breaking the table.
func firstLine(v any) string {
	s, _ := v.(string)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}
