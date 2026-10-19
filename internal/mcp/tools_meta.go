package mcp

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	ddcore "github.com/jrvidotti/ddcore"
	"github.com/jrvidotti/ddcore/docs"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/release"
	"github.com/jrvidotti/ddcore/internal/scaffold"
)

// metaTools registers the tools over the apps and their metadata. Most of
// them work without the database (see offlineTools).
func (s *server) metaTools(srv *mcp.Server) {
	e := s.e

	addTool(srv, "list_doctypes", "Lists all loaded DocTypes (name, app, label, isChild, isSingle, isVirtual, submittable, file).",
		func(ctx context.Context, in struct {
			App string `json:"app,omitempty" jsonschema:"filter by app"`
		}) (any, error) {
			reg := e.Current().Meta
			var out []map[string]any
			for _, n := range reg.Names() {
				d := reg.DocTypes[n]
				if in.App != "" && d.App != in.App {
					continue
				}
				out = append(out, map[string]any{"name": n, "app": d.App, "label": d.Label, "isChild": d.IsChild, "isSingle": d.IsSingle, "isVirtual": d.IsVirtual(), "submittable": d.Submittable, "fields": len(d.Fields), "file": d.SourceFile})
			}
			return out, nil
		})

	addTool(srv, "get_doctype", "The full meta of a DocType (fields, permissions, naming, controller methods, source path).",
		func(ctx context.Context, in struct {
			Name string `json:"name" jsonschema:"DocType name"`
		}) (any, error) {
			st := e.Current()
			d, err := st.DocType(in.Name)
			if err != nil {
				return nil, err
			}
			app := st.App(d.App)
			path := d.SourceFile
			if app.Embedded == nil && path != "" {
				rel := strings.TrimPrefix(path, d.App+".")
				path = filepath.Join(app.Dir, strings.ReplaceAll(rel, ".", "/")+".ts")
				// module path uses dots for dirs AND the ".doctype" suffix
				path = strings.Replace(path, "/doctype.ts", ".doctype.ts", 1)
			}
			out := map[string]any{"doctype": d, "file": path, "table": d.TableName()}
			if d.IsVirtual() {
				delete(out, "table") // a union of its sources, with no table of its own
			}
			return out, nil
		})

	addTool(srv, "scaffold_doctype", scaffoldDescription,
		func(ctx context.Context, in struct {
			App  string               `json:"app" jsonschema:"app name (directory)"`
			Spec scaffold.DoctypeSpec `json:"spec" jsonschema:"DocType definition"`
		}) (any, error) {
			st := e.Current()
			app := st.App(in.App)
			if app.Name == "" || app.Embedded != nil {
				return nil, cerr.NotFound("app {0} does not exist (apps: {1})", in.App, st.AppOrder())
			}
			files, err := scaffold.Doctype(app.Dir, app.Name, in.Spec)
			if err != nil {
				return nil, err
			}
			if err := e.Load(); err != nil {
				return nil, fmt.Errorf("files created (%v) but the meta is invalid: %w", files, err)
			}
			return map[string]any{"files": files, "next": "run migrate to create the table"}, nil
		})

	addTool(srv, "extend_doctype", extendDescription(),
		func(ctx context.Context, in extendInput) (any, error) {
			return extendDoctype(e, in)
		})

	addTool(srv, "validate_meta", "Recompiles apps and validates metadata (without touching the database). Use after editing .ts files.",
		func(ctx context.Context, in struct{}) (any, error) {
			if err := e.Load(); err != nil {
				return nil, err
			}
			st := e.Current()
			return map[string]any{"ok": true, "doctypes": len(st.Meta.DocTypes), "whitelisted": st.WhitelistedPaths()}, nil
		})

	addTool(srv, "migrate", "Applies the pending DDL, installs new apps and runs patches, in one transaction: beforeSchema patches, DDL, fixtures, afterSchema patches, then the drops. Refuses a rename or conversion it cannot make safely rather than guessing. With dry_run it only reports the plan.",
		func(ctx context.Context, in struct {
			DryRun bool `json:"dry_run,omitempty"`
			Prune  bool `json:"prune,omitempty" jsonschema:"drops the columns and tables the meta no longer declares, and only the empty ones"`
		}) (any, error) {
			if err := e.Load(); err != nil {
				return nil, err
			}
			if in.DryRun {
				plan, err := e.Plan(ctx, in.Prune)
				if err != nil {
					return nil, err
				}
				return map[string]any{"report": db.Report(plan), "ddl": db.SQL(plan), "patches": e.PendingPatches()}, nil
			}
			res, err := e.Migrate(ctx, in.Prune)
			if err != nil {
				return nil, err
			}
			_, typesErr := s.writeTypes()
			out := map[string]any{
				"report": db.Report(res.DDL), "ddl": db.SQL(res.DDL), "patches": res.Patches,
				"installed": res.Installed, "renames": res.Renames, "recorded": res.Recorded,
			}
			if res.AdminPassword != "" {
				// Shown once, like the CLI does: the database keeps only the hash.
				out["adminPassword"] = res.AdminPassword
			}
			if typesErr != nil {
				// The schema is migrated whatever happens to the typings.
				out["typesError"] = typesErr.Error()
			}
			return out, nil
		})

	addTool(srv, "import", "Loads a `ddcore export` directory into this site (DAT-01), keeping ids, owners and timestamps and replaying no effects: no controller hook, webhook, notification or email. action is plan (what would be loaded, writing nothing), run (load it; dry_run rehearses in one rolled-back transaction), status (the runs this site has seen, or one of them with its errors) or reconcile (compare the site with the export: rows, child rows, statuses, files and money). Two runs over the same directory load it once; a run that stops leaves a cursor, and passing its id as resume continues it. max_batches stops after that many batches so a long load advances over several calls. On a site with tenancy, tenant loads into that tenant instead of the platform space (leaving out shared DocTypes and the Admin and Guest accounts), and scopes status and reconcile to it.",
		func(ctx context.Context, in struct {
			Action      string   `json:"action" jsonschema:"one of plan, run, status, reconcile"`
			Dir         string   `json:"dir,omitempty" jsonschema:"the export directory, as ddcore export wrote it"`
			Map         string   `json:"map,omitempty" jsonschema:"path to a mapping file that renames doctypes and fields, drops, sets constants and remaps ids and users"`
			DryRun      bool     `json:"dry_run,omitempty" jsonschema:"validate everything and write nothing"`
			Batch       int      `json:"batch,omitempty" jsonschema:"lines per transaction (default 500)"`
			MaxBatches  int      `json:"max_batches,omitempty" jsonschema:"stop after this many batches, leaving the run resumable"`
			Only        []string `json:"only,omitempty" jsonschema:"load just these DocTypes"`
			Resume      string   `json:"resume,omitempty" jsonschema:"continue this run"`
			RunID       string   `json:"run_id,omitempty" jsonschema:"status: the run to report on"`
			VerifyBytes bool     `json:"verify_bytes,omitempty" jsonschema:"reconcile: read every stored attachment back"`
			Tenant      string   `json:"tenant,omitempty" jsonschema:"on a site with tenancy: the tenant to load into, or whose runs to list; empty is the platform space"`
		}) (any, error) {
			if in.Tenant != "" {
				if err := e.CheckTenant(ctx, in.Tenant); err != nil {
					return nil, err
				}
			}
			// the space this call works in: the tenant named, or the platform
			ctx = engine.WithTenant(ctx, in.Tenant)
			if in.Action == "status" {
				if in.RunID != "" {
					return e.ImportRunStatus(ctx, in.RunID)
				}
				return e.ImportRuns(ctx, 20)
			}
			if in.Dir == "" {
				return nil, cerr.Validation("give the export directory")
			}
			a := engine.ImportArgs{
				Dir: in.Dir, DryRun: in.DryRun || in.Action == "plan", Batch: in.Batch,
				MaxBatches: in.MaxBatches, Only: in.Only, Resume: in.Resume,
				VerifyBytes: in.VerifyBytes, Actor: "mcp", BypassMaintenance: true, Tenant: in.Tenant,
			}
			if in.Map != "" {
				if err := a.UseMapFile(in.Map); err != nil {
					return nil, err
				}
			}
			switch in.Action {
			case "reconcile":
				return e.ImportReconcile(ctx, a)
			case "plan", "run", "":
				return e.Import(ctx, a)
			default:
				return nil, cerr.Validation("action is plan, run, status or reconcile, not %q", in.Action)
			}
		})

	addTool(srv, "i18n_extract", "Rewrites translations/<lang>.csv from the code and the metadata (`ddcore i18n extract`), keeping every translation already there, and reports per app what is missing, orphan, an override of an earlier app's key (kept, even with prune) or dynamic. Run it after adding a label, then fill the missing keys with set_translations. With check it writes nothing.",
		func(ctx context.Context, in i18nExtractIn) (any, error) {
			return s.i18nExtract(in)
		})

	addTool(srv, "set_translations", "Fills an app's translations/<lang>.csv: applies English key → translation pairs, rewrites the catalogue in its canonical form and reloads it. Every key must exist in the code (a typo is refused, nothing written); the result lists what is still missing, so iterate until it is empty.",
		func(ctx context.Context, in setTranslationsIn) (any, error) {
			return s.setTranslations(in)
		})

	addTool(srv, "generate_types", "Generates .ddcore/types.d.ts (TS interfaces per DocType) in each app.",
		func(ctx context.Context, in struct{}) (any, error) {
			return s.writeTypes()
		})

	addTool(srv, "reload", "Rebuilds and reloads the apps (`ddcore dev` already does this when a file is saved).",
		func(ctx context.Context, in struct{}) (any, error) {
			if err := e.Load(); err != nil {
				return nil, err
			}
			return "reloaded", nil
		})

	addTool(srv, "list_apps", "Loaded apps, directories, whitelisted functions, reports, workspaces and scheduler.",
		func(ctx context.Context, in struct{}) (any, error) {
			st := e.Current()
			var apps []map[string]any
			for _, n := range st.AppOrder() {
				a := st.Snap.Apps[n]
				apps = append(apps, map[string]any{"name": n, "title": a.Title, "dir": a.Dir, "scheduler": a.Scheduler, "desk": a.Desk})
			}
			reports := slices.Sorted(maps.Keys(st.Snap.Reports))
			ws := slices.Sorted(maps.Keys(st.Snap.Workspaces))
			return map[string]any{"apps": apps, "whitelisted": st.WhitelistedPaths(), "reports": reports, "workspaces": ws}, nil
		})

	addTool(srv, "whats_new", "What changed in ddcore since the running version: the changelog entries above it, the newest published release, and whether an upgrade is available.",
		func(ctx context.Context, in struct {
			Since string `json:"since,omitempty" jsonschema:"changelog entries newer than this version (default: the running one)"`
		}) (any, error) {
			since := in.Since
			if since == "" {
				since = engine.Version
			}
			out := map[string]any{
				"ddcore":    engine.Version,
				"since":     since,
				"changelog": release.Since(release.Full(ddcore.Changelog, docs.ChangelogFS), since),
			}
			// A failed lookup is not an error: the changelog is the half of the
			// answer that matters, and it is already in hand.
			if u, err := release.Check(ctx, engine.Version); err == nil && u != nil {
				out["latest"], out["updateAvailable"] = u.Latest, u.Available
			}
			return out, nil
		})
}

const scaffoldDescription = "Creates the files for a new DocType in an app (doctype.ts and optionally controller/form/test). Run migrate afterwards. Fieldtypes: Data, Email, Small Text, Text, Text Editor (rich text, sanitized HTML), Markdown Editor, Code (options: language), Int, Float, Currency, Percent, Rating (options: stars, default 5), Duration (seconds; options: hideDays, hideSeconds), Color, Check, Date, Month, Datetime, Time, Select (options: list), Autocomplete (free text; options: suggestions), Barcode (options: Code128 (default), EAN-13 or QR), Signature (PNG data URL drawn on the form; not unique, indexed, listed or a title/search/sort field), Geolocation (GeoJSON FeatureCollection drawn on a map, [lon, lat]; not unique, indexed or a title/search/sort field), Link (options: DocType), Dynamic Link (options: field with DocType), Table (options: child DocType), Table MultiSelect (options: child DocType with exactly one Link field), Attach, Attach Image, JSON, Password, Vault, Section Break, Tab Break, HTML, Report (options: the name of a defineReport, drawn as a grid inside the form)."
