package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// jobTools registers the background-job and maintenance tools.
func (s *server) jobTools(srv *mcp.Server) {
	e := s.e

	// Job administration. These read and write ddcore_job, which is a framework
	// table and not a DocType, so list_docs cannot reach it.
	//
	// Payloads are absent here for the same reason they are absent from the HTTP
	// API: a queued password-reset mail job carries its recovery link in its
	// arguments, and `ddcore jobs show` is the one path to that.
	addTool(srv, "list_jobs", "Background jobs, newest first, filtered by status, queue, method or user. Arguments and results are not included.",
		func(ctx context.Context, in struct {
			Status string `json:"status,omitempty" jsonschema:"queued, running, done, failed or cancelled (comma-separated)"`
			Queue  string `json:"queue,omitempty"`
			Method string `json:"method,omitempty" jsonschema:"exact dotted method path"`
			User   string `json:"user,omitempty"`
			Limit  int    `json:"limit,omitempty"`
		}) (any, error) {
			f := engine.JobFilter{Status: engine.JobStatuses(in.Status), Queue: in.Queue, Method: in.Method, User: in.User, Limit: in.Limit}
			return e.ListJobs(ctx, f)
		})

	addTool(srv, "maintenance_status", "Whether the site is in maintenance mode (writes and jobs paused), since when, by whom and why.",
		func(ctx context.Context, in struct{}) (any, error) {
			return e.Maintenance(ctx), nil
		})

	addTool(srv, "maintenance_set", "Turns maintenance mode on or off. While on, servers refuse writes with 503, workers stop claiming jobs and the scheduler skips its runs; the CLI and this tool keep working. Recorded as an Audit Event.",
		func(ctx context.Context, in struct {
			Enabled bool   `json:"enabled"`
			Reason  string `json:"reason,omitempty" jsonschema:"shown to users in the desk banner"`
		}) (any, error) {
			return e.SetMaintenance(ctx, in.Enabled, in.Reason, "mcp")
		})

	addTool(srv, "get_job", "One background job, without its arguments or result.",
		func(ctx context.Context, in struct {
			ID int64 `json:"id"`
		}) (any, error) {
			return e.GetJob(ctx, in.ID, false)
		})

	addTool(srv, "retry_job", "Queues a failed or cancelled job again, as a new job. The original row is kept as the record of the failure.",
		func(ctx context.Context, in struct {
			ID    int64 `json:"id"`
			Force bool  `json:"force,omitempty" jsonschema:"retry again a job that was already retried"`
		}) (any, error) {
			return e.RetryJob(ctx, in.ID, in.Force)
		})

	addTool(srv, "cancel_job", "Stops a job. A queued job is cancelled outright; a running one is interrupted by its worker within a few seconds, and effects it has already had outside the database are not undone.",
		func(ctx context.Context, in struct {
			ID int64 `json:"id"`
		}) (any, error) {
			return e.CancelJob(ctx, in.ID, "Admin")
		})

	addTool(srv, "purge_jobs", "Deletes finished jobs past their retention window. Without arguments it uses the site's own windows; dry_run reports what would go.",
		func(ctx context.Context, in struct {
			DoneDays   *int `json:"done_days,omitempty" jsonschema:"days to keep done jobs; 0 keeps them forever"`
			FailedDays *int `json:"failed_days,omitempty" jsonschema:"days to keep failed and cancelled jobs; 0 keeps them forever"`
			DryRun     bool `json:"dry_run,omitempty"`
		}) (any, error) {
			o := e.SitePurgeOpts(in.DryRun)
			if in.DoneDays != nil {
				o.DoneDays = *in.DoneDays
			}
			if in.FailedDays != nil {
				o.FailedDays = *in.FailedDays
			}
			return e.PurgeJobs(ctx, o)
		})
}
