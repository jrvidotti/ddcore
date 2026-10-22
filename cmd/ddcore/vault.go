package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const vaultUsage = `usage: ddcore vault <subcommand>

  rekey    re-encrypt every vault secret under DDCORE_SECRET_KEY (--dry-run, --batch N)

Rotating the master key: deploy with the new key in DDCORE_SECRET_KEY and the
old one in DDCORE_SECRET_KEY_PREVIOUS, run ddcore vault rekey, and remove
DDCORE_SECRET_KEY_PREVIOUS once ddcore doctor shows nothing under it. The
re-key crosses every tenant, is recorded as one Audit Event, and never prints
a value.`

func cmdVault(args []string) error {
	return dispatch(vaultUsage, []subcommand{
		{"rekey", vaultRekey},
	}, args)
}

func vaultRekey(args []string) error {
	fs := newFlagSet("vault rekey")
	dryRun := fs.Bool("dry-run", false, "count the secrets under each key and change nothing")
	batch := fs.Int("batch", 100, "secrets re-encrypted per transaction")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		res, err := e.VaultRekey(ctx, engine.VaultRekeyOpts{DryRun: *dryRun, BatchSize: *batch, Actor: cliActor()})
		if res != nil {
			printRekey(res)
		}
		return err
	})
}

func printRekey(res *engine.VaultRekeyResult) {
	if !res.DryRun {
		fmt.Printf("re-encrypted:         %d\n", res.Moved)
	}
	c := res.Census
	fmt.Printf("under current key:    %d\n", c.Current)
	if c.PreviousConfigured || c.Previous > 0 {
		fmt.Printf("under previous key:   %d\n", c.Previous)
	} else {
		fmt.Printf("under previous key:   - (%s is not set)\n", engine.PreviousKeyEnvName)
	}
	if len(c.Unreadable) > 0 {
		fmt.Printf("opened by neither:    %d (%s)\n", len(c.Unreadable), strings.Join(c.Unreadable, ", "))
	}
}
