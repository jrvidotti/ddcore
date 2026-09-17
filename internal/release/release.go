package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// repo is the source of the published releases — the same one install.sh
// downloads from.
const repo = "jrvidotti/ddcore"

// apiBase is a var so a test can point the lookup at an httptest server; there
// is no reason for a deployment to change it.
var apiBase = "https://api.github.com"

// httpClient has its own timeout rather than relying on the caller's context
// alone: this call is always incidental to whatever the operator actually ran,
// and it must never be the reason a command hangs.
var httpClient = &http.Client{Timeout: 5 * time.Second}

// SetEndpointForTest points the lookup at base and clears the cached answer,
// returning a function that restores both. Tests in other packages — the
// doctor's, for one — need it; production code never calls it.
func SetEndpointForTest(base string) (restore func()) {
	cache.Lock()
	defer cache.Unlock()
	prev := apiBase
	apiBase = base
	cache.tag, cache.err, cache.when = "", nil, time.Time{}
	return func() {
		cache.Lock()
		defer cache.Unlock()
		apiBase = prev
		cache.tag, cache.err, cache.when = "", nil, time.Time{}
	}
}

// Update is what the caller reports: the running version, the newest published
// release, and whether the second is ahead of the first.
type Update struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
}

// cache keeps one answer for an hour. The MCP server is long-lived and an agent
// may ask repeatedly; GitHub allows 60 unauthenticated requests an hour per
// address, and a framework that spends them on its own version check would
// deserve the rate limit it got.
var cache struct {
	sync.Mutex
	tag  string
	when time.Time
	err  error
}

const (
	cacheTTL = time.Hour
	// A failure is remembered for far less time than an answer: releases are
	// published a few times a month, but a network that is down now may be up
	// in a minute, and remembering "no" for an hour would make a long-lived
	// server silent about a release it could have seen.
	failTTL = 5 * time.Minute
)

// Latest is the tag of the newest published release, e.g. `v0.15.0`.
//
// The lock is held across the request on purpose: several callers arriving at
// once should produce one request, not one each.
func Latest(ctx context.Context) (string, error) {
	cache.Lock()
	defer cache.Unlock()
	if !cache.when.IsZero() {
		ttl := cacheTTL
		if cache.err != nil {
			ttl = failTTL
		}
		if time.Since(cache.when) < ttl {
			return cache.tag, cache.err
		}
	}
	tag, err := fetchLatest(ctx)
	cache.tag, cache.err, cache.when = tag, err, time.Now()
	return tag, err
}

func fetchLatest(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", fmt.Errorf("github returned no tag_name")
	}
	return body.TagName, nil
}

// Check compares the running version against the newest published release.
// It returns nil, nil when current is not a release — an unflagged `go build`
// or a `dev` binary has nothing meaningful to compare — so a caller can treat
// "no update" and "not applicable" the same way.
func Check(ctx context.Context, current string) (*Update, error) {
	if !engine.IsRelease(current) {
		return nil, nil
	}
	tag, err := Latest(ctx)
	if err != nil {
		return nil, err
	}
	newer, ok := engine.Newer(current, tag)
	if !ok {
		return nil, fmt.Errorf("latest release %q is not a version", tag)
	}
	return &Update{Current: current, Latest: tag, Available: newer}, nil
}
