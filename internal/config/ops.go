package config

import (
	"fmt"
	"time"
)

// OpsPolicy is what this site considers healthy: how deep a queue may get, how
// old the oldest job may be, how many failures in a window are ordinary.
//
// It lives in ddcore.json and not in the environment for the same reason
// AuthPolicy does — staging has to call a backlog a backlog exactly like
// production, or the rehearsal is not a rehearsal. Nothing here is a secret and
// nothing varies per machine.
//
// None of it ever changes an HTTP status. A readiness probe that failed on a
// deep queue would have the orchestrator restart the very workers draining it;
// these numbers decide what a report calls a warning, and what `ddcore doctor
// --strict` exits non-zero for.
type OpsPolicy struct {
	// WindowMinutes is how far back the failure counts look.
	WindowMinutes int `json:"windowMinutes"`
	// QueueBacklog is how many runnable jobs may wait before it is worth saying
	// so. A job scheduled for tomorrow is not backlog and is not counted.
	QueueBacklog int `json:"queueBacklog"`
	// QueueAgeSeconds is how long the oldest runnable job may wait.
	QueueAgeSeconds int `json:"queueAgeSeconds"`
	JobFailures     int `json:"jobFailures"`
	ErrorLogEntries int `json:"errorLogEntries"`
	// ReadyTimeoutMs bounds the readiness probe: a probe that hangs is a probe
	// that lies, because the caller reads "no answer yet" as "still thinking".
	ReadyTimeoutMs int `json:"readyTimeoutMs"`
	// SlowRequestMs is when a request stops being ordinary traffic and earns a
	// log line of its own however quiet the access log is set.
	SlowRequestMs int `json:"slowRequestMs"`
}

// DefaultOps is the policy a site gets when it says nothing.
func DefaultOps() OpsPolicy {
	return OpsPolicy{
		WindowMinutes: 15, QueueBacklog: 100, QueueAgeSeconds: 300,
		JobFailures: 5, ErrorLogEntries: 20, ReadyTimeoutMs: 2000, SlowRequestMs: 2000,
	}
}

func (o OpsPolicy) Window() time.Duration {
	return time.Duration(o.WindowMinutes) * time.Minute
}
func (o OpsPolicy) ReadyTimeout() time.Duration {
	return time.Duration(o.ReadyTimeoutMs) * time.Millisecond
}
func (o OpsPolicy) SlowRequest() time.Duration {
	return time.Duration(o.SlowRequestMs) * time.Millisecond
}

func (o OpsPolicy) validate() error {
	for _, c := range []struct {
		name string
		v    int
	}{
		{"windowMinutes", o.WindowMinutes},
		{"queueBacklog", o.QueueBacklog},
		{"queueAgeSeconds", o.QueueAgeSeconds},
		{"jobFailures", o.JobFailures},
		{"errorLogEntries", o.ErrorLogEntries},
		{"readyTimeoutMs", o.ReadyTimeoutMs},
		{"slowRequestMs", o.SlowRequestMs},
	} {
		if c.v <= 0 {
			return fmt.Errorf("ops.%s must be greater than zero", c.name)
		}
	}
	// A probe allowed to wait a minute is not a probe: the orchestrator's own
	// timeout fires first and the site is marked down without ever being asked.
	if o.ReadyTimeoutMs > 10_000 {
		return fmt.Errorf("ops.readyTimeoutMs must be 10000 or less")
	}
	return nil
}

// WithDefaults fills the fields a partial `ops` block left out. A site that
// writes only `{"queueBacklog": 500}` means "that one number, the rest as
// shipped" — not "every other alarm at zero".
func (o OpsPolicy) WithDefaults() OpsPolicy {
	d := DefaultOps()
	for _, f := range []struct{ v, def *int }{
		{&o.WindowMinutes, &d.WindowMinutes},
		{&o.QueueBacklog, &d.QueueBacklog},
		{&o.QueueAgeSeconds, &d.QueueAgeSeconds},
		{&o.JobFailures, &d.JobFailures},
		{&o.ErrorLogEntries, &d.ErrorLogEntries},
		{&o.ReadyTimeoutMs, &d.ReadyTimeoutMs},
		{&o.SlowRequestMs, &d.SlowRequestMs},
	} {
		if *f.v <= 0 {
			*f.v = *f.def
		}
	}
	return o
}
