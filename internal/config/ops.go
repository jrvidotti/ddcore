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
	// JobRetentionDays and JobRetentionFailedDays are how long a finished job is
	// kept. Failures are kept longer than successes: they are the evidence, and
	// they are read long after the fact.
	//
	// Pointers, and not plain ints, because zero has to mean "keep forever" here
	// while it means "unset" in every field above. An int cannot hold both
	// answers, and WithDefaults would silently turn "forever" into thirty days —
	// the worst failure a retention setting can have. They are filled below
	// outside the loop that rewrites zeroes, which cannot express this.
	JobRetentionDays       *int `json:"jobRetentionDays"`
	JobRetentionFailedDays *int `json:"jobRetentionFailedDays"`
	// WebhookRetentionDays is how long a finished Webhook Delivery is kept. Its
	// payload is a copy of a document as it was, so keeping it for ever keeps
	// data an erasure elsewhere was meant to remove. Zero keeps for ever.
	WebhookRetentionDays *int `json:"webhookRetentionDays"`
	// AuditEventRetentionDays is how long an Audit Event is kept. Zero keeps for ever.
	AuditEventRetentionDays *int `json:"auditRetentionDays"`
}

// DoneRetentionDays and FailedRetentionDays resolve the pointers into the one
// number a sweep needs, where zero means "keep forever". Callers read these
// rather than the fields, so a nil pointer can never reach a query as a cutoff.
func (o OpsPolicy) DoneRetentionDays() int            { return derefOr(o.JobRetentionDays, 0) }
func (o OpsPolicy) FailedRetentionDays() int          { return derefOr(o.JobRetentionFailedDays, 0) }
func (o OpsPolicy) WebhookDeliveryRetentionDays() int { return derefOr(o.WebhookRetentionDays, 0) }
func (o OpsPolicy) AuditRetentionDays() int           { return derefOr(o.AuditEventRetentionDays, 0) }

func derefOr(p *int, d int) int {
	if p == nil {
		return d
	}
	return *p
}

// DefaultOps is the policy a site gets when it says nothing.
func DefaultOps() OpsPolicy {
	done, failed, webhooks, audit := 7, 30, 30, 0
	return OpsPolicy{
		WindowMinutes: 15, QueueBacklog: 100, QueueAgeSeconds: 300,
		JobFailures: 5, ErrorLogEntries: 20, ReadyTimeoutMs: 2000, SlowRequestMs: 2000,
		JobRetentionDays: &done, JobRetentionFailedDays: &failed, WebhookRetentionDays: &webhooks,
		AuditEventRetentionDays: &audit,
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
	// Retention admits zero, which is "keep forever". Only a negative window is
	// meaningless, and it is refused here rather than read as a date in the
	// future by whichever query gets it.
	for _, c := range []struct {
		name string
		v    *int
	}{
		{"jobRetentionDays", o.JobRetentionDays},
		{"jobRetentionFailedDays", o.JobRetentionFailedDays},
		{"webhookRetentionDays", o.WebhookRetentionDays},
		{"auditRetentionDays", o.AuditEventRetentionDays},
	} {
		if c.v != nil && *c.v < 0 {
			return fmt.Errorf("ops.%s must be zero (keep forever) or greater", c.name)
		}
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
	// Separately, because the loop above cannot tell "the site wrote 0" from
	// "the site wrote nothing", and for retention those are opposite answers.
	if o.JobRetentionDays == nil {
		o.JobRetentionDays = d.JobRetentionDays
	}
	if o.JobRetentionFailedDays == nil {
		o.JobRetentionFailedDays = d.JobRetentionFailedDays
	}
	if o.WebhookRetentionDays == nil {
		o.WebhookRetentionDays = d.WebhookRetentionDays
	}
	if o.AuditEventRetentionDays == nil {
		o.AuditEventRetentionDays = d.AuditEventRetentionDays
	}
	return o
}
