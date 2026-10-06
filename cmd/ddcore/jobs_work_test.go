package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
)

// `jobs work` starts every pool of ddcore.json, or with --queue only the ones
// it names, sized by the file or by --workers.
func TestJobsWorkPools(t *testing.T) {
	site, err := config.WorkerPools(map[string]int{"default": 2, "bot": 3, "idle": 0})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		site   config.Workers
		queues string
		n      int
		pools  map[string]int
		start  []string
		err    string
	}{
		{site: site, n: -1, pools: map[string]int{"default": 2, "bot": 3, "idle": 0}},
		{site: config.WorkerCount(4), n: -1, pools: map[string]int{"default": 4}},
		{site: site, queues: "bot", n: -1, pools: map[string]int{"default": 2, "bot": 3, "idle": 0}, start: []string{"bot"}},
		{site: site, queues: "bot, default", n: 1, pools: map[string]int{"default": 1, "bot": 1, "idle": 0}, start: []string{"bot", "default"}},
		{site: site, queues: "new", n: 2, pools: map[string]int{"default": 2, "bot": 3, "idle": 0, "new": 2}, start: []string{"new"}},
		{site: config.WorkerCount(4), queues: "bot", n: 2, pools: map[string]int{"default": 4, "bot": 2}, start: []string{"bot"}},
		{site: site, queues: "new", n: -1, err: "has no pool"},
		{site: site, queues: "idle", n: -1, err: "no workers"},
		{site: site, queues: "bot", n: 0, err: "no workers"},
		{site: site, queues: " , ", n: -1, err: "name at least one"},
		{site: site, n: 3, err: "--workers sizes the pools --queue names"},
	}
	for _, c := range cases {
		pools, start, err := workPools(c.site, c.queues, c.n)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("--queue %q --workers %d: want %q, got %v", c.queues, c.n, c.err, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("--queue %q --workers %d: %v", c.queues, c.n, err)
			continue
		}
		if !reflect.DeepEqual(pools, c.pools) || !reflect.DeepEqual(start, c.start) {
			t.Errorf("--queue %q --workers %d: pools %v start %v, want %v %v", c.queues, c.n, pools, start, c.pools, c.start)
		}
	}
	// the site's own pools are not changed by a process's flags
	if p := site.Pools(); p["bot"] != 3 || len(p) != 3 {
		t.Errorf("site pools changed: %v", p)
	}
}

// doctor prints the total and, when ddcore.json names pools, each of them.
func TestDoctorPrintsWorkerPools(t *testing.T) {
	r := &doctorReport{Workers: 2, Ops: config.DefaultOps()}
	if out := renderDoctor(t, r); !strings.Contains(out, "workers:    2\n") {
		t.Errorf("number form:\n%s", out)
	}
	r = &doctorReport{Workers: 4, WorkerPools: map[string]int{"default": 2, "bot": 2}, Ops: config.DefaultOps()}
	if out := renderDoctor(t, r); !strings.Contains(out, "workers:    4 (bot 2, default 2)") {
		t.Errorf("pools:\n%s", out)
	}
}
