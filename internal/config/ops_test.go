package config

import "testing"

// A partial `ops` block means "that one number, the rest as shipped". Left
// alone, the fields it omits stay at zero, and a zero threshold is not "no
// threshold" — it is an alarm that fires on the first job.
func TestOpsPolicyFillsWhatAPartialBlockOmits(t *testing.T) {
	got := OpsPolicy{QueueBacklog: 500}.WithDefaults()
	if got.QueueBacklog != 500 {
		t.Fatalf("the value the site wrote was overwritten: %d", got.QueueBacklog)
	}
	d := DefaultOps()
	if got.WindowMinutes != d.WindowMinutes || got.JobFailures != d.JobFailures ||
		got.ReadyTimeoutMs != d.ReadyTimeoutMs || got.SlowRequestMs != d.SlowRequestMs {
		t.Fatalf("omitted fields did not fall back to the defaults: %+v", got)
	}
	if err := got.validate(); err != nil {
		t.Fatalf("a filled policy must validate: %v", err)
	}
}

// A threshold nobody can honour must be refused at boot, not quietly replaced
// with another one — the same rule the rounding mode and the access policy
// already follow.
func TestOpsPolicyRefusesWhatItCannotHonour(t *testing.T) {
	negative := DefaultOps()
	negative.QueueBacklog = -1
	slow := DefaultOps()
	slow.ReadyTimeoutMs = 60_000

	for name, p := range map[string]OpsPolicy{"negative backlog": negative, "probe too slow": slow} {
		if err := p.validate(); err == nil {
			t.Fatalf("%s was accepted: %+v", name, p)
		}
	}
}

func TestOpsPolicyDurations(t *testing.T) {
	p := DefaultOps()
	if p.Window().Minutes() != float64(p.WindowMinutes) {
		t.Fatalf("window is %v for %d minutes", p.Window(), p.WindowMinutes)
	}
	if p.ReadyTimeout().Milliseconds() != int64(p.ReadyTimeoutMs) {
		t.Fatalf("ready timeout is %v for %d ms", p.ReadyTimeout(), p.ReadyTimeoutMs)
	}
	if p.SlowRequest().Milliseconds() != int64(p.SlowRequestMs) {
		t.Fatalf("slow request is %v for %d ms", p.SlowRequest(), p.SlowRequestMs)
	}
}

// Retention is the one threshold where zero has to mean something, and it means
// the opposite of what it means everywhere else here: "keep forever", not "keep
// nothing". Read as a cutoff it would empty the table, so the field is a pointer
// and it is filled outside the loop that rewrites zeroes into defaults.
func TestJobRetentionDistinguishesUnsetFromKeepForever(t *testing.T) {
	shipped := OpsPolicy{}.WithDefaults()
	if shipped.DoneRetentionDays() <= 0 || shipped.FailedRetentionDays() <= 0 {
		t.Fatalf("a site that says nothing gets the shipped windows: %+v", shipped)
	}
	// Failures are the evidence, and outlive the successes.
	if shipped.FailedRetentionDays() <= shipped.DoneRetentionDays() {
		t.Errorf("failed jobs should be kept longer than done ones: %d vs %d",
			shipped.FailedRetentionDays(), shipped.DoneRetentionDays())
	}

	forever := 0
	p := OpsPolicy{JobRetentionDays: &forever, JobRetentionFailedDays: &forever}.WithDefaults()
	if p.DoneRetentionDays() != 0 || p.FailedRetentionDays() != 0 {
		t.Fatalf("an explicit zero was overwritten by the defaults: %+v", p)
	}
	if err := p.validate(); err != nil {
		t.Fatalf("keeping jobs forever is a legitimate choice: %v", err)
	}

	negative := -1
	bad := DefaultOps()
	bad.JobRetentionDays = &negative
	if err := bad.validate(); err == nil {
		t.Error("a negative retention window was accepted")
	}
}
