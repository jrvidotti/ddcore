package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// serving stands up a fake GitHub returning body with status, and points the
// lookup at it for the duration of the test.
func serving(t *testing.T, status int, body string) *int {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if want := "/repos/jrvidotti/ddcore/releases/latest"; r.URL.Path != want {
			t.Errorf("path %q, want %q", r.URL.Path, want)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(SetEndpointForTest(srv.URL))
	return &calls
}

func TestCheckReportsANewerRelease(t *testing.T) {
	serving(t, http.StatusOK, `{"tag_name":"v0.15.0"}`)
	u, err := Check(context.Background(), "v0.14.0")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if u == nil || !u.Available {
		t.Fatalf("want an available update, got %+v", u)
	}
	if u.Latest != "v0.15.0" || u.Current != "v0.14.0" {
		t.Errorf("got %+v", u)
	}
}

func TestCheckIsQuietWhenUpToDate(t *testing.T) {
	serving(t, http.StatusOK, `{"tag_name":"v0.15.0"}`)
	for _, current := range []string{"v0.15.0", "v0.15.0-3-gabc123", "v0.16.0"} {
		u, err := Check(context.Background(), current)
		if err != nil {
			t.Fatalf("%s: %v", current, err)
		}
		if u == nil || u.Available {
			t.Errorf("%s: want no available update, got %+v", current, u)
		}
	}
}

// A binary with no release version — an unflagged `go build`, a `dev` build —
// has nothing to compare, and must not even ask.
func TestCheckSkipsANonRelease(t *testing.T) {
	calls := serving(t, http.StatusOK, `{"tag_name":"v0.15.0"}`)
	for _, current := range []string{"dev", "latest", "abc1234"} {
		u, err := Check(context.Background(), current)
		if err != nil || u != nil {
			t.Errorf("%s: want nil, nil; got %+v, %v", current, u, err)
		}
	}
	if *calls != 0 {
		t.Errorf("want no request, got %d", *calls)
	}
}

func TestCheckFailsQuietlyOnABadAnswer(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"not found", `{}`, http.StatusNotFound},
		{"rate limited", `{}`, http.StatusForbidden},
		{"no tag", `{"tag_name":""}`, http.StatusOK},
		{"not json", `<html>`, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serving(t, tc.status, tc.body)
			if _, err := Check(context.Background(), "v0.14.0"); err == nil {
				t.Errorf("want an error, got none")
			}
		})
	}
}

// A tag that is not a version must not be reported as an upgrade.
func TestCheckRejectsAnUnparseableTag(t *testing.T) {
	serving(t, http.StatusOK, `{"tag_name":"nightly"}`)
	if u, err := Check(context.Background(), "v0.14.0"); err == nil {
		t.Errorf("want an error, got %+v", u)
	}
}

// The cache exists so a long-lived MCP server does not spend GitHub's
// unauthenticated budget on repeated questions.
func TestLatestIsCached(t *testing.T) {
	calls := serving(t, http.StatusOK, `{"tag_name":"v0.15.0"}`)
	for i := 0; i < 3; i++ {
		if _, err := Latest(context.Background()); err != nil {
			t.Fatalf("Latest: %v", err)
		}
	}
	if *calls != 1 {
		t.Errorf("want 1 request, got %d", *calls)
	}
}

// A failure must not silence the check for the full hour an answer is kept:
// the network that was down a minute ago may be up now.
func TestAFailureIsRememberedBriefly(t *testing.T) {
	fail := true
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.15.0"}`))
	}))
	defer srv.Close()
	defer SetEndpointForTest(srv.URL)()

	if _, err := Latest(context.Background()); err == nil {
		t.Fatal("want the first call to fail")
	}
	// Still inside the short window: no second request.
	if _, err := Latest(context.Background()); err == nil {
		t.Fatal("want the cached failure")
	}
	if calls != 1 {
		t.Fatalf("want 1 request, got %d", calls)
	}

	// Age the failure past its window; the answer's own hour must not apply.
	cache.Lock()
	cache.when = time.Now().Add(-failTTL - time.Minute)
	cache.Unlock()
	fail = false

	tag, err := Latest(context.Background())
	if err != nil || tag != "v0.15.0" {
		t.Fatalf("want a retry to succeed, got %q, %v", tag, err)
	}
	if calls != 2 {
		t.Errorf("want 2 requests, got %d", calls)
	}
}

// A successful answer, by contrast, is kept for the full hour.
func TestAnAnswerIsKeptForTheHour(t *testing.T) {
	calls := serving(t, http.StatusOK, `{"tag_name":"v0.15.0"}`)
	if _, err := Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	cache.Lock()
	cache.when = time.Now().Add(-failTTL - time.Minute)
	cache.Unlock()
	if _, err := Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Errorf("a good answer should outlive the failure window; got %d requests", *calls)
	}
}
