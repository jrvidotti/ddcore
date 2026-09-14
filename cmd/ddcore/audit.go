package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const auditUsage = `usage: ddcore audit <subcommand>

  list       audit events, newest first (--action, --actor, --target, --outcome, --since, --limit, --json)
  purge      delete old audit events (--days, --dry-run)
`

func cmdAudit(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", auditUsage)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return auditList(rest)
	case "purge":
		return auditPurge(rest)
	default:
		return fmt.Errorf("unknown subcommand: %s\n\n%s", sub, auditUsage)
	}
}

type auditFilterFlags struct {
	action, actor, target, outcome, since *string
	limit                                 *int
}

func addAuditFilterFlags(fs *flag.FlagSet) auditFilterFlags {
	return auditFilterFlags{
		action:  fs.String("action", "", "action prefix or exact match (e.g. role.assign, vault.write)"),
		actor:   fs.String("actor", "", "the user or service that triggered the event"),
		target:  fs.String("target", "", "target document name or identifier"),
		outcome: fs.String("outcome", "", "Allowed or Denied"),
		since:   fs.String("since", "", "only events in the last duration, e.g. 24h, 7d"),
		limit:   fs.Int("limit", 20, "how many events"),
	}
}

func (f auditFilterFlags) filter() (engine.AuditFilter, error) {
	out := engine.AuditFilter{
		Action:     *f.action,
		Actor:      *f.actor,
		TargetName: *f.target,
		Outcome:    *f.outcome,
		Limit:      *f.limit,
	}
	if *f.since != "" {
		s := *f.since
		var d time.Duration
		var err error
		if strings.HasSuffix(s, "d") {
			daysStr := strings.TrimSuffix(s, "d")
			var days int
			if _, err = fmt.Sscanf(daysStr, "%d", &days); err == nil {
				d = time.Duration(days) * 24 * time.Hour
			}
		} else {
			d, err = time.ParseDuration(s)
		}
		if err != nil {
			return out, fmt.Errorf("--since: %w", err)
		}
		t := time.Now().Add(-d)
		out.Since = &t
	}
	return out, nil
}

func auditList(args []string) error {
	fs := newFlagSet("audit list")
	f := addAuditFilterFlags(fs)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	filter, err := f.filter()
	if err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		rows, err := e.ListAuditEvents(ctx, filter)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(rows)
		}
		if len(rows) == 0 {
			fmt.Println("no audit events match")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIME\tACTOR\tACTION\tOUTCOME\tTARGET\tIP\tDETAIL")
		for _, r := range rows {
			target := fmt.Sprintf("%v:%v", r["target_doctype"], r["target_name"])
			if r["target_doctype"] == nil || r["target_doctype"] == "" {
				target = fmt.Sprint(r["target_name"])
			}
			ip := ""
			if r["ip"] != nil {
				ip = fmt.Sprint(r["ip"])
			}
			detail := ""
			if r["detail"] != nil {
				detail = firstLine(r["detail"])
			}
			fmt.Fprintf(w, "%s\t%v\t%v\t%v\t%s\t%s\t%s\n",
				shortTime(r["creation"]), r["actor"], r["action"], r["outcome"],
				target, ip, detail)
		}
		return w.Flush()
	})
}

func auditPurge(args []string) error {
	fs := newFlagSet("audit purge")
	days := fs.Int("days", -1, "delete audit events older than N days (defaults to ops.auditRetentionDays)")
	dry := fs.Bool("dry-run", false, "report what would be deleted")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		d := *days
		if d < 0 {
			d = e.Cfg.Ops.AuditRetentionDays()
		}
		if d <= 0 {
			return fmt.Errorf("a retention window in days must be specified with --days or ops.auditRetentionDays (0 keeps forever)")
		}
		n, err := e.PurgeAuditEvents(ctx, d, *dry)
		if err != nil {
			return err
		}
		if *dry {
			fmt.Printf("would purge %d audit event(s) older than %d day(s)\n", n, d)
		} else {
			fmt.Printf("purged %d audit event(s) older than %d day(s)\n", n, d)
		}
		return nil
	})
}
