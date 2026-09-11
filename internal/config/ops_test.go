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
