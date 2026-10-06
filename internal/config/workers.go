package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DefaultPool is the worker pool that serves the "default" queue and every
// queue no other pool is named for.
const DefaultPool = "default"

// Workers is `workers` in ddcore.json: how many job workers a process runs,
// and which queues they serve. A number is that many workers for every queue.
// An object is a pool per queue — {"default": 2, "bot": 2} — where each named
// pool serves its own queue only and "default", which it must name, serves
// the default queue and every queue not named. That is what keeps a queue of
// slow jobs from holding up the rest.
type Workers struct {
	n     int
	pools map[string]int // nil in the number form
}

// WorkerCount is the number form: n workers for every queue.
func WorkerCount(n int) Workers { return Workers{n: n} }

// WorkerPools is the object form, checked as Load checks the file.
func WorkerPools(pools map[string]int) (Workers, error) {
	if err := validatePools(pools); err != nil {
		return Workers{}, err
	}
	cp := make(map[string]int, len(pools))
	for q, n := range pools {
		cp[q] = n
	}
	return Workers{pools: cp}, nil
}

// Total is how many workers the process runs, over all its pools.
func (w Workers) Total() int {
	if w.pools == nil {
		return w.n
	}
	t := 0
	for _, n := range w.pools {
		t += n
	}
	return t
}

// Pools is the size of each pool by queue. The number form is one pool,
// "default", which then serves every queue.
func (w Workers) Pools() map[string]int {
	if w.pools == nil {
		return map[string]int{DefaultPool: w.n}
	}
	out := make(map[string]int, len(w.pools))
	for q, n := range w.pools {
		out[q] = n
	}
	return out
}

// Named reports whether the site names pools (the object form).
func (w Workers) Named() bool { return w.pools != nil }

// String is the pools as a reader wants them: "2", or "4 (default 2, bot 2)".
func (w Workers) String() string {
	if w.pools == nil {
		return fmt.Sprint(w.n)
	}
	names := make([]string, 0, len(w.pools))
	for q := range w.pools {
		names = append(names, q)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, q := range names {
		parts[i] = fmt.Sprintf("%s %d", q, w.pools[q])
	}
	return fmt.Sprintf("%d (%s)", w.Total(), strings.Join(parts, ", "))
}

func (w *Workers) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	if len(b) > 0 && b[0] == '{' {
		var pools map[string]int
		if err := json.Unmarshal(b, &pools); err != nil {
			return fmt.Errorf("workers: a number, or an object of queue names to numbers: %w", err)
		}
		v, err := WorkerPools(pools)
		if err != nil {
			return err
		}
		*w = v
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("workers: a number, or an object of queue names to numbers: %w", err)
	}
	if n < 0 {
		return fmt.Errorf("workers cannot be negative")
	}
	*w = Workers{n: n}
	return nil
}

func (w Workers) MarshalJSON() ([]byte, error) {
	if w.pools == nil {
		return json.Marshal(w.n)
	}
	return json.Marshal(w.pools)
}

func validatePools(pools map[string]int) error {
	if _, ok := pools[DefaultPool]; !ok {
		return fmt.Errorf(`workers: an object of pools must name "default", which serves every queue the others do not`)
	}
	for q, n := range pools {
		if strings.TrimSpace(q) == "" {
			return fmt.Errorf("workers: a pool needs a queue name")
		}
		if n < 0 {
			return fmt.Errorf("workers: pool %q cannot be negative", q)
		}
	}
	return nil
}
