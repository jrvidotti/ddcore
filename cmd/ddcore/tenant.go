package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

const tenantUsage = `usage: ddcore tenant <command>

  list                                   the tenants of the site
  create <slug> [--title T] [--admin email [--name N]]
                                         create a tenant; --admin invites its first System Manager
  enable <slug> | disable <slug>         a disabled tenant refuses sign-ins, requests and jobs
  adopt <slug>                           move every row of the platform space into the tenant
                                         (a site that had one customer before it had tenancy)

Needs "tenancy": true in ddcore.json. To run any other command inside a
tenant, put --tenant <slug> before it: ddcore --tenant acme eval '…'
`

// cmdTenant administers the tenants of a site with tenancy. Everything here
// is the operator's: it runs as Admin in the platform space.
func cmdTenant(args []string) error {
	if len(args) == 0 {
		fmt.Print(tenantUsage)
		return nil
	}
	if os.Getenv("DDCORE_TENANT") != "" {
		return fmt.Errorf("tenant commands run in the platform space: drop --tenant")
	}
	return withEngine(func(e *engine.Engine, ctx context.Context) error {
		if !e.Cfg.Tenancy {
			return fmt.Errorf(`this site has no tenants: set "tenancy": true in ddcore.json and run ddcore migrate`)
		}
		switch args[0] {
		case "list":
			return tenantList(e, ctx)
		case "create":
			return tenantCreate(e, ctx, args[1:])
		case "enable", "disable":
			if len(args) < 2 {
				return fmt.Errorf("usage: ddcore tenant %s <slug>", args[0])
			}
			return tenantEnable(e, ctx, args[1], args[0] == "enable")
		case "adopt":
			if len(args) < 2 {
				return fmt.Errorf("usage: ddcore tenant adopt <slug>")
			}
			moved, err := e.AdoptPlatformRows(ctx, args[1])
			if err != nil {
				return err
			}
			tables := make([]string, 0, len(moved))
			for t := range moved {
				tables = append(tables, t)
			}
			sort.Strings(tables)
			for _, t := range tables {
				fmt.Printf("  %-32s %d\n", t, moved[t])
			}
			fmt.Printf("%s adopted the rows of the platform space (%d tables)\n", args[1], len(tables))
			return nil
		}
		return fmt.Errorf("unknown tenant command %q\n\n%s", args[0], tenantUsage)
	})
}

func tenantList(e *engine.Engine, ctx context.Context) error {
	return e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		rows, err := c.TenantList()
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Println("no tenants yet: ddcore tenant create <slug>")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SLUG\tTITLE\tSTATE")
		for _, r := range rows {
			state := "enabled"
			if r["enabled"] != true {
				state = "disabled"
			}
			fmt.Fprintf(w, "%v\t%v\t%s\n", r["id"], r["title"], state)
		}
		return w.Flush()
	})
}

func tenantCreate(e *engine.Engine, ctx context.Context, args []string) error {
	fs := newFlagSet("tenant create")
	title := fs.String("title", "", "the tenant's name (default: the slug)")
	admin := fs.String("admin", "", "e-mail of the tenant's first System Manager, who is sent an invitation")
	name := fs.String("name", "", "the administrator's name (default: the e-mail)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: ddcore tenant create <slug> [--title T] [--admin email [--name N]]")
	}
	slug := fs.Arg(0)
	if *title == "" {
		*title = slug
	}
	var rec *engine.Recovery
	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		doc, err := c.NewDoc("Site Tenant", engine.Doc{"slug": slug, "title": *title, "enabled": true})
		if err != nil {
			return err
		}
		// the Tenant controller runs every app's onTenantCreate inside it
		if _, err := c.Insert(doc, engine.SaveOpts{}); err != nil {
			return err
		}
		if *admin == "" {
			return nil
		}
		full := strings.TrimSpace(*name)
		if full == "" {
			full = *admin
		}
		return c.InTenant(slug, func(t *engine.Ctx) error {
			res, err := e.InviteUser(t, engine.Invitation{Email: *admin, FullName: full, Roles: []string{"System Manager"}})
			if err != nil {
				return err
			}
			rec = &engine.Recovery{}
			rec.Link, _ = res["link"].(string)
			rec.Expires, _ = res["expires"].(time.Time)
			return nil
		})
	})
	if err != nil {
		return err
	}
	fmt.Println("tenant created:", slug)
	if rec != nil {
		reportRecovery(*admin, rec)
	}
	return nil
}

func tenantEnable(e *engine.Engine, ctx context.Context, slug string, enabled bool) error {
	err := e.Run(ctx, "Admin", func(c *engine.Ctx) error {
		doc, err := c.GetDoc("Site Tenant", slug)
		if err != nil {
			return err
		}
		doc["enabled"] = enabled
		// through the document, so the Tenant controller tells every process
		_, err = c.Save(doc, engine.SaveOpts{})
		return err
	})
	if err != nil {
		return err
	}
	state := "disabled: it refuses sign-ins, requests and jobs"
	if enabled {
		state = "enabled"
	}
	fmt.Printf("%s is %s\n", slug, state)
	return nil
}
