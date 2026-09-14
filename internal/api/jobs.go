package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// Job administration is not for ordinary users. A listing names every method
// the site runs and the user each one runs as, and the actions stop work and
// run it again — so every route here sits behind the same role as the health
// report, checked inside the transaction rather than at the router.
func requireJobAdmin(c *engine.Ctx, r *http.Request) error {
	if !c.HasRole("System Manager") {
		target := urlParam(r, "id")
		c.AuditDenied("job.admin", "Job", target, nil)
		return cerr.Permission("Administering jobs requires the System Manager role")
	}
	return nil
}

// jobID reads the {id} route parameter.
func jobID(r *http.Request) (int64, error) {
	n, err := strconv.ParseInt(urlParam(r, "id"), 10, 64)
	if err != nil {
		return 0, cerr.Validation("{0} is not a job id", urlParam(r, "id"))
	}
	return n, nil
}

// jobFilter builds the listing filter from the query string, following the
// conventions /api/resource already uses for limit and start.
func jobFilter(r *http.Request) (engine.JobFilter, error) {
	q := r.URL.Query()
	f := engine.JobFilter{
		Queue:  q.Get("queue"),
		Method: q.Get("method"),
		User:   q.Get("user"),
		Limit:  atoiOr(q.Get("limit"), 20),
		Start:  atoiOr(q.Get("start"), 0),
	}
	if s := q.Get("status"); s != "" {
		for _, v := range strings.Split(s, ",") {
			if v = strings.TrimSpace(v); v != "" {
				f.Status = append(f.Status, v)
			}
		}
	}
	if s := q.Get("since"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return f, cerr.Validation("since: {0} is not a duration", s)
		}
		t := time.Now().Add(-d)
		f.Since = &t
	}
	return f, nil
}

func atoiOr(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}

// listJobs answers the queue. The rows never carry args or result: ddcore_job
// holds live secrets — the framework's own mail job is enqueued with the
// rendered message, so a queued password-reset carries its recovery link — and
// this is the one job surface a browser reaches. `ddcore jobs show` is the way
// to a payload, where the caller already holds the database.
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := requireJobAdmin(c, r); err != nil {
			return nil, err
		}
		f, err := jobFilter(r)
		if err != nil {
			return nil, err
		}
		rows, err := s.E.ListJobs(c.Ctx, f)
		if err != nil {
			return nil, err
		}
		if r.URL.Query().Get("with_count") == "" {
			return rows, nil
		}
		n, err := s.E.CountJobs(c.Ctx, f)
		if err != nil {
			return nil, err
		}
		return map[string]any{"rows": rows, "count": n}, nil
	})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := requireJobAdmin(c, r); err != nil {
			return nil, err
		}
		id, err := jobID(r)
		if err != nil {
			return nil, err
		}
		// false: never the payload, for the reason on listJobs.
		return s.E.GetJob(c.Ctx, id, false)
	})
}

func (s *Server) jobStats(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := requireJobAdmin(c, r); err != nil {
			return nil, err
		}
		window := s.E.Cfg.Ops.Window()
		if m := atoiOr(r.URL.Query().Get("window"), 0); m > 0 {
			window = time.Duration(m) * time.Minute
		}
		return s.E.JobStats(c.Ctx, window)
	})
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := requireJobAdmin(c, r); err != nil {
			return nil, err
		}
		id, err := jobID(r)
		if err != nil {
			return nil, err
		}
		var body struct {
			Force bool `json:"force"`
		}
		decodeBody(r, &body)
		return s.E.RetryJob(c.Ctx, id, body.Force, c.User)
	})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := requireJobAdmin(c, r); err != nil {
			return nil, err
		}
		id, err := jobID(r)
		if err != nil {
			return nil, err
		}
		// The caller's own name, so the row records who stopped the work.
		return s.E.CancelJob(c.Ctx, id, c.User)
	})
}

// purgeJobs runs retention on demand. Left unsaid, the windows are the site's
// own, the same ones the nightly sweep uses.
func (s *Server) purgeJobs(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		if err := requireJobAdmin(c, r); err != nil {
			return nil, err
		}
		var body struct {
			DoneDays   *int `json:"doneDays"`
			FailedDays *int `json:"failedDays"`
			DryRun     bool `json:"dryRun"`
		}
		decodeBody(r, &body)
		o := engine.PurgeOpts{
			DoneDays:   s.E.Cfg.Ops.DoneRetentionDays(),
			FailedDays: s.E.Cfg.Ops.FailedRetentionDays(),
			DryRun:     body.DryRun,
		}
		if body.DoneDays != nil {
			o.DoneDays = *body.DoneDays
		}
		if body.FailedDays != nil {
			o.FailedDays = *body.FailedDays
		}
		return s.E.PurgeJobs(c.Ctx, o, c.User)
	})
}

// decodeBody reads an optional JSON body: these actions all work with none.
func decodeBody(r *http.Request, v any) {
	if r.Body == nil {
		return
	}
	json.NewDecoder(r.Body).Decode(v)
}
