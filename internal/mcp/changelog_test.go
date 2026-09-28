package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	ddcore "github.com/jrvidotti/ddcore"
	"github.com/jrvidotti/ddcore/docs"
	"github.com/jrvidotti/ddcore/internal/release"
)

// connect runs the server over an in-memory pair and returns a connected
// client session. The engine is nil: nothing reached here touches it, and a
// test that needed a database to read a document would be testing the database.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := New(nil).Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// The changelog has to be reachable the same way the reference is. Before this
// it was a file at the repository root that no binary could see, so an agent
// upgrading an app had no way to ask what had changed.
func TestChangelogResourceIsServed(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()

	list, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found bool
	for _, r := range list.Resources {
		if r.URI == "ddcore://changelog" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ddcore://changelog is not registered")
	}

	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "ddcore://changelog"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("want 1 content, got %d", len(res.Contents))
	}
	if !strings.Contains(res.Contents[0].Text, "# Changelog") {
		t.Fatalf("the changelog did not come back:\n%s", res.Contents[0].Text)
	}

	// Every archived series is its own resource.
	uris := map[string]bool{}
	for _, r := range list.Resources {
		uris[r.URI] = true
	}
	for _, s := range release.Archive(docs.ChangelogFS) {
		uri := "ddcore://changelog/" + s.Minor
		if !uris[uri] {
			t.Errorf("%s is not registered", uri)
			continue
		}
		res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		if !strings.HasPrefix(res.Contents[0].Text, "# ddcore "+s.Minor) {
			t.Errorf("%s did not come back: %.40q", uri, res.Contents[0].Text)
		}
	}
}

// An app several minors behind still hears about the archived series: whats_new
// reads across the root file and every archive, not just the root.
func TestWhatsNewReachesIntoTheArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	defer release.SetEndpointForTest(srv.URL)()

	out, err := connect(t).CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "whats_new",
		Arguments: map[string]any{"since": "0.18.4"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := out.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"## 0.19.0", "## 0.20.0", "## 0.21.0"} {
		if !strings.Contains(text, want) {
			t.Errorf("whats_new since 0.18.4 is missing %s", want)
		}
	}
	if strings.Contains(text, "## 0.18.4") {
		t.Errorf("whats_new since 0.18.4 includes 0.18.4 itself")
	}
}

// The split has to stay honest as releases are cut: the root holds Unreleased
// and one minor series, each archive holds exactly its own series, and no
// release is recorded twice. A promoted X.Y.0 whose previous series was not
// moved out fails here.
func TestChangelogSplitIsConsistent(t *testing.T) {
	minorOf := func(v string) string {
		p := strings.SplitN(v, ".", 3)
		return p[0] + "." + p[1]
	}
	seen := map[string]string{}
	record := func(v, where string) {
		if prev, dup := seen[v]; dup {
			t.Errorf("%s is recorded in both %s and %s", v, prev, where)
		}
		seen[v] = where
	}

	rootMinors := map[string]bool{}
	for _, s := range release.Sections(ddcore.Changelog) {
		if s.Version == "" {
			continue
		}
		rootMinors[minorOf(s.Version)] = true
		record(s.Version, "CHANGELOG.md")
	}
	if len(rootMinors) > 1 {
		t.Errorf("CHANGELOG.md holds more than one minor series %v: move the older one to docs/changelog/<minor>.md", rootMinors)
	}
	if !strings.Contains(ddcore.Changelog, "<!-- #endregion releases -->") {
		t.Errorf("CHANGELOG.md lost its closing region marker, which docs/changelog/index.md includes up to")
	}

	archive := release.Archive(docs.ChangelogFS)
	if len(archive) == 0 {
		t.Fatal("no archived series embedded")
	}
	for _, a := range archive {
		if rootMinors[a.Minor] {
			t.Errorf("series %s is both current and archived", a.Minor)
		}
		secs := release.Sections(a.Text)
		if len(secs) == 0 {
			t.Errorf("changelog/%s.md has no releases", a.Minor)
		}
		for _, s := range secs {
			if s.Version == "" || minorOf(s.Version) != a.Minor {
				t.Errorf("changelog/%s.md holds a section that is not a %s release: %.40q", a.Minor, a.Minor, s.Body)
				continue
			}
			record(s.Version, "changelog/"+a.Minor+".md")
		}
		if !strings.Contains(ddcore.Changelog, "(docs/changelog/"+a.Minor+".md)") {
			t.Errorf("CHANGELOG.md does not link docs/changelog/%s.md", a.Minor)
		}
	}
}

func TestWhatsNewReturnsTheChangelogAboveAVersion(t *testing.T) {
	// Pinned at a fake GitHub: a test that asked the real one would be slow,
	// offline-hostile, and would spend the unauthenticated request budget of
	// whoever ran it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v99.0.0"}`))
	}))
	defer srv.Close()
	defer release.SetEndpointForTest(srv.URL)()

	cs := connect(t)
	ctx := context.Background()

	out, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "whats_new",
		Arguments: map[string]any{"since": "v0.0.1"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if out.IsError {
		t.Fatalf("whats_new failed: %+v", out.Content)
	}
	text := out.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Unreleased") {
		t.Fatalf("the unreleased work is missing:\n%s", text)
	}
	for _, want := range []string{`"latest": "v99.0.0"`, `"updateAvailable": true`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in:\n%s", want, text)
		}
	}
}

// A GitHub that cannot be reached must not cost the caller the half of the
// answer that is already embedded in the binary.
func TestWhatsNewStillAnswersWithoutGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	defer release.SetEndpointForTest(srv.URL)()

	out, err := connect(t).CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "whats_new",
		Arguments: map[string]any{"since": "v0.0.1"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if out.IsError {
		t.Fatalf("whats_new failed: %+v", out.Content)
	}
	text := out.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Unreleased") {
		t.Fatalf("the changelog is missing:\n%s", text)
	}
	if strings.Contains(text, "updateAvailable") {
		t.Errorf("a failed lookup should report nothing about it:\n%s", text)
	}
}

// The embedded copy is the one that ships, so it is the one worth asserting
// parses at all: a changelog whose headings drifted would silently serve
// nothing to a caller asking what is new.
func TestEmbeddedChangelogParses(t *testing.T) {
	if !strings.HasPrefix(ddcore.Changelog, "# Changelog") {
		t.Fatalf("CHANGELOG.md does not start as expected: %.40q", ddcore.Changelog)
	}
	secs := release.Sections(ddcore.Changelog)
	if len(secs) == 0 {
		t.Fatal("no sections parsed out of the embedded changelog")
	}
	if secs[0].Version != "" || !strings.HasPrefix(secs[0].Body, "## Unreleased") {
		t.Errorf("the first section should be Unreleased, got %q", secs[0].Version)
	}
}
