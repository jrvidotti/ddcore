package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
)

const webhooksUsage = `usage: ddcore webhooks <subcommand>

  list     recent deliveries, newest first (--status, --webhook, --limit)
  replay   send finished deliveries again, same webhook-id: webhooks replay <delivery>...

A replay is recorded as an Audit Event, like one pressed in the desk. Payloads
are not printed: they are copies of documents, and live in the desk for a
System Manager to read.`

func cmdWebhooks(args []string) error {
	return dispatch(webhooksUsage, []subcommand{
		{"list", webhooksList},
		{"replay", webhooksReplay},
	}, args)
}

func webhooksList(args []string) error {
	fs := newFlagSet("webhooks list")
	status := fs.String("status", "", "Queued, Retrying, Sent or Failed")
	webhook := fs.String("webhook", "", "only this Webhook")
	limit := fs.Int("limit", 20, "how many deliveries")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withEngine(loadOpts{}, func(e *engine.Engine, ctx context.Context) error {
		// the operator's list: every tenant's deliveries
		rows, err := db.Select(ctx, e.DB.Sys, `SELECT id, creation, webhook, event, status, attempts, response_status,
			reference_doctype, reference_id, error
			FROM tab_webhook_delivery
			WHERE ($1 = '' OR status = $1) AND ($2 = '' OR webhook = $2)
			ORDER BY creation DESC LIMIT $3`, *status, *webhook, *limit)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "DELIVERY\tCREATED\tEVENT\tSTATUS\tTRIES\tHTTP\tREFERENCE\tERROR")
		for _, r := range rows {
			ref := db.Str(r["reference_doctype"])
			if ref != "" {
				ref += " " + db.Str(r["reference_id"])
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\t%v\t%s\t%s\n", db.Str(r["id"]), shortTime(r["creation"]), db.Str(r["event"]),
				db.Str(r["status"]), r["attempts"], nilDash(r["response_status"]), ref, firstLine(r["error"]))
		}
		return tw.Flush()
	})
}

func webhooksReplay(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ddcore webhooks replay <delivery>...")
	}
	return withEngine(loadOpts{}, func(e *engine.Engine, ctx context.Context) error {
		failed := 0
		for _, name := range args {
			// Admin, as every administrative CLI command acts: whoever
			// runs this already holds the database.
			err := e.Run(ctx, "Admin", func(c *engine.Ctx) error { return c.ReplayWebhook(name) })
			if err != nil {
				failed++
				fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
				continue
			}
			fmt.Printf("%s: queued\n", name)
		}
		if failed > 0 {
			return fmt.Errorf("%d of %d deliveries were not replayed", failed, len(args))
		}
		return nil
	})
}

func nilDash(v any) any {
	if v == nil {
		return "-"
	}
	return v
}
