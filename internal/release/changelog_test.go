package release

import (
	"strings"
	"testing"
)

const fixture = `# Changelog

Preamble that belongs to no section.

## Unreleased

### Added

- Something not yet released.

## 0.15.0 — 2026-09-20

### Breaking

- A thing that moved.

## [0.14.0] - 2026-09-16

### Fixed

- An older fix.
`

func TestSections(t *testing.T) {
	got := Sections(fixture)
	if len(got) != 3 {
		t.Fatalf("want 3 sections, got %d: %+v", len(got), got)
	}
	want := []string{"", "0.15.0", "0.14.0"}
	for i, w := range want {
		if got[i].Version != w {
			t.Errorf("section %d: version %q, want %q", i, got[i].Version, w)
		}
	}
	if !strings.HasPrefix(got[0].Body, "## Unreleased") {
		t.Errorf("first body should start at its heading, got %q", got[0].Body)
	}
	// The preamble belongs to no section and must not leak into one.
	if strings.Contains(got[0].Body, "Preamble") {
		t.Errorf("preamble leaked into the Unreleased section: %q", got[0].Body)
	}
}

func TestSectionsHeadingForms(t *testing.T) {
	for _, tc := range []struct{ heading, want string }{
		{"## Unreleased", ""},
		{"## [0.14.0] - 2026-09-16", "0.14.0"},
		{"## 0.15.0 — 2026-09-20", "0.15.0"},
		{"## v0.16.0", "v0.16.0"},
		{"## Not a version at all", ""},
		{"## ", ""},
	} {
		got := Sections(tc.heading + "\n\nbody\n")
		if len(got) != 1 {
			t.Fatalf("%q: want 1 section, got %d", tc.heading, len(got))
		}
		if got[0].Version != tc.want {
			t.Errorf("%q: version %q, want %q", tc.heading, got[0].Version, tc.want)
		}
	}
}

func TestSince(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		want    []string
		absent  []string
	}{
		{
			name:    "older binary sees both newer releases",
			current: "v0.14.0",
			want:    []string{"## Unreleased", "## 0.15.0"},
			absent:  []string{"## [0.14.0]"},
		},
		{
			name:    "current binary sees only the unreleased work",
			current: "v0.15.0",
			want:    []string{"## Unreleased"},
			absent:  []string{"## 0.15.0", "## [0.14.0]"},
		},
		{
			name:    "a describe build counts as its release",
			current: "v0.14.0-3-gabc123",
			want:    []string{"## Unreleased", "## 0.15.0"},
			absent:  []string{"## [0.14.0]"},
		},
		{
			name:    "ahead of everything published",
			current: "v9.0.0",
			want:    []string{"## Unreleased"},
			absent:  []string{"## 0.15.0", "## [0.14.0]"},
		},
		{
			name:    "a dev build cannot compare, so it gets the lot",
			current: "dev",
			want:    []string{"## Unreleased", "## 0.15.0", "## [0.14.0]"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Since(fixture, tc.current)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("unexpected %q in:\n%s", a, got)
				}
			}
		})
	}
}

// An upgrade path under Breaking is written with a code fence, and a shell
// comment inside one starts with `## `. Splitting there would end the release's
// section early and hand its remainder to the version-less section, which is
// shown to every caller regardless of what they are running.
func TestSectionsIgnoresHeadingsInsideAFence(t *testing.T) {
	const md = "## Unreleased\n" +
		"\n" +
		"### Breaking\n" +
		"\n" +
		"- Rename the column:\n" +
		"\n" +
		"  ```sh\n" +
		"  ## this is a comment, not a heading\n" +
		"  ddcore migrate\n" +
		"  ```\n" +
		"\n" +
		"## 0.14.0 — 2026-09-16\n" +
		"\n" +
		"- An older fix.\n"

	got := Sections(md)
	if len(got) != 2 {
		t.Fatalf("want 2 sections, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Body, "ddcore migrate") {
		t.Errorf("the fenced block was cut out of its section:\n%s", got[0].Body)
	}
	if got[1].Version != "0.14.0" {
		t.Errorf("second section is %q, want 0.14.0", got[1].Version)
	}
	// And the fenced text must not reach a caller who is already up to date.
	if out := Since(md, "v0.14.0"); strings.Contains(out, "0.14.0 — 2026-09-16") {
		t.Errorf("an old release leaked into the news:\n%s", out)
	}
}
