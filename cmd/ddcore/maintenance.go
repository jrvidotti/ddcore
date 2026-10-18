package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const maintenanceUsage = `Usage: ddcore maintenance <on|off|status> [options]

  on      pause the site: HTTP writes answer 503, workers stop claiming jobs,
          the scheduler skips its runs (--reason "text" is shown in the desk)
  off     resume
  status  print the current state (--json)

The flag lives in the database, so every server and worker sees it within a
couple of seconds. This CLI keeps writing while it is on: that is the window
backup, restore and migrate work in.`

func cmdMaintenance(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", maintenanceUsage)
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("maintenance " + sub)
	reason := fs.String("reason", "", "why the site is paused, shown to users")
	asJSON := fs.Bool("json", false, "JSON output")
	if err := parseFlags(fs, rest); err != nil {
		return err
	}
	return withEngine(loadOpts{}, func(e *engine.Engine, ctx context.Context) error {
		// report prints the state the subcommand got or set
		report := func(get func() (engine.MaintenanceState, error)) func([]string) error {
			return func([]string) error {
				st, err := get()
				if err != nil {
					return err
				}
				if *asJSON {
					return json.NewEncoder(os.Stdout).Encode(st)
				}
				printMaintenance(st)
				return nil
			}
		}
		return dispatchWith([]subcommand{
			{"on", report(func() (engine.MaintenanceState, error) { return e.SetMaintenance(ctx, true, *reason, cliActor()) })},
			{"off", report(func() (engine.MaintenanceState, error) { return e.SetMaintenance(ctx, false, "", cliActor()) })},
			{"status", report(func() (engine.MaintenanceState, error) { return e.Maintenance(ctx), nil })},
		}, []string{sub}, func(sub string) error {
			return fmt.Errorf("unknown subcommand: %s\n\n%s", sub, maintenanceUsage)
		})
	})
}

func printMaintenance(st engine.MaintenanceState) {
	if !st.Enabled {
		fmt.Println("maintenance: off")
		return
	}
	line := "maintenance: ON"
	if st.Since != nil {
		line += fmt.Sprintf(" since %s (%s)", st.Since.Format(time.RFC3339), time.Since(*st.Since).Round(time.Second))
	}
	if st.Actor != "" {
		line += " by " + st.Actor
	}
	fmt.Println(line)
	if st.Reason != "" {
		fmt.Println("reason:", st.Reason)
	}
}

// cliActor names who ran a command in an audit row: the operating-system user
// behind the terminal, which is the only identity a CLI has.
func cliActor() string {
	for _, k := range []string{"DDCORE_ACTOR", "USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return "cli:" + v
		}
	}
	return "cli"
}
