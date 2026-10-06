package config

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// workers is a number for every queue, or a pool per queue that must name
// "default"; anything else stops the site at startup rather than running it
// with workers nobody asked for.
func TestWorkersUnmarshal(t *testing.T) {
	cases := []struct {
		body  string
		total int
		pools map[string]int
		named bool
		err   string
	}{
		{body: `{}`, total: 2, pools: map[string]int{"default": 2}},
		{body: `{"workers": 5}`, total: 5, pools: map[string]int{"default": 5}},
		{body: `{"workers": 0}`, total: 0, pools: map[string]int{"default": 0}},
		{body: `{"workers": null}`, total: 2, pools: map[string]int{"default": 2}},
		{body: `{"workers": {"default": 2, "bot": 3}}`, total: 5, pools: map[string]int{"default": 2, "bot": 3}, named: true},
		{body: `{"workers": {"default": 1, "bot": 0}}`, total: 1, pools: map[string]int{"default": 1, "bot": 0}, named: true},
		{body: `{"workers": {"bot": 2}}`, err: `must name "default"`},
		{body: `{"workers": -1}`, err: "cannot be negative"},
		{body: `{"workers": {"default": 2, "bot": -1}}`, err: `pool "bot" cannot be negative`},
		{body: `{"workers": {"default": 2, " ": 1}}`, err: "needs a queue name"},
		{body: `{"workers": "2"}`, err: "a number, or an object"},
		{body: `{"workers": {"default": "2"}}`, err: "a number, or an object"},
	}
	for _, c := range cases {
		clearMailEnv(t)
		f, _, err := Load(site(t, c.body))
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: want an error with %q, got %v", c.body, c.err, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.body, err)
			continue
		}
		if got := f.Workers.Total(); got != c.total {
			t.Errorf("%s: total %d, want %d", c.body, got, c.total)
		}
		if got := f.Workers.Pools(); !reflect.DeepEqual(got, c.pools) {
			t.Errorf("%s: pools %v, want %v", c.body, got, c.pools)
		}
		if f.Workers.Named() != c.named {
			t.Errorf("%s: named %v, want %v", c.body, f.Workers.Named(), c.named)
		}
	}
}

// A file `ddcore init` writes, or one Save rewrites, keeps the form it had.
func TestWorkersMarshalRoundTrips(t *testing.T) {
	for _, in := range []string{`2`, `{"bot":3,"default":2}`} {
		var w Workers
		if err := json.Unmarshal([]byte(in), &w); err != nil {
			t.Fatal(err)
		}
		out, err := json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, []byte(in)) {
			t.Errorf("%s came back as %s", in, out)
		}
	}
	if s := mustPools(t, map[string]int{"default": 2, "bot": 2}).String(); s != "4 (bot 2, default 2)" {
		t.Errorf("String: %q", s)
	}
}

// DDCORE_WORKERS is a number and beats the file, pools included.
func TestWorkersEnvOverride(t *testing.T) {
	clearMailEnv(t)
	t.Setenv("DDCORE_WORKERS", "6")
	f, _, err := Load(site(t, `{"workers": {"default": 2, "bot": 2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Workers.Named() || f.Workers.Total() != 6 {
		t.Errorf("DDCORE_WORKERS=6 gave %s", f.Workers)
	}
	t.Setenv("DDCORE_WORKERS", `{"default":1}`)
	if _, _, err := Load(site(t, `{}`)); err == nil {
		t.Error("DDCORE_WORKERS accepts a number only")
	}
}

func mustPools(t *testing.T, p map[string]int) Workers {
	t.Helper()
	w, err := WorkerPools(p)
	if err != nil {
		t.Fatal(err)
	}
	return w
}
