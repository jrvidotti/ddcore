// Package release answers two questions the binary cannot answer from its own
// build alone: what changed in the releases since this one, and whether a newer
// one exists. Both `ddcore doctor` and the MCP server ask them, so the parsing
// and the network call live here once rather than in each caller.
package release

import (
	"strings"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// Section is one `## …` block of CHANGELOG.md. Version is empty for the
// Unreleased section, which is the only heading that names no release.
type Section struct {
	Version string
	// Body is the whole section including its heading line, so that joining
	// sections back together reproduces the document.
	Body string
}

// Sections splits a changelog into its `## …` blocks, dropping whatever
// preamble comes before the first one. The heading is read leniently —
// `## Unreleased`, `## [0.15.0] - 2026-09-20` and `## 0.15.0 — 2026-09-20` all
// parse — because the file is written by hand and a heading that drifts by a
// bracket should not silently hide a release.
func Sections(md string) []Section {
	var out []Section
	var cur *Section
	var body []string

	flush := func() {
		if cur != nil {
			cur.Body = strings.TrimRight(strings.Join(body, "\n"), "\n")
			out = append(out, *cur)
		}
	}

	fenced := false
	for _, line := range strings.Split(md, "\n") {
		// A `## ` inside a fenced block is sample text, not a heading — an
		// upgrade path under Breaking is exactly where one would appear, and
		// splitting there would hand the rest of that release to the section
		// with no version, which is shown to everybody.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "## ") {
			flush()
			cur = &Section{Version: headingVersion(line)}
			body = []string{line}
			continue
		}
		if cur != nil {
			body = append(body, line)
		}
	}
	flush()
	return out
}

// headingVersion pulls the release out of a `## …` line, or returns "" when the
// heading names none (Unreleased).
func headingVersion(line string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if rest == "" {
		return ""
	}
	tok := strings.Fields(rest)[0]
	tok = strings.Trim(tok, "[]")
	if !engine.IsRelease(tok) {
		return ""
	}
	return tok
}

// Since returns the Unreleased section plus every section newer than current,
// as markdown. A current that is not a release — a `dev` build — cannot be
// compared against anything, so the whole file comes back: too much news is a
// better failure than none.
func Since(md, current string) string {
	if !engine.IsRelease(current) {
		return strings.TrimSpace(md)
	}
	var keep []string
	for _, s := range Sections(md) {
		if s.Version == "" {
			keep = append(keep, s.Body)
			continue
		}
		if newer, ok := engine.Newer(current, s.Version); ok && newer {
			keep = append(keep, s.Body)
		}
	}
	return strings.Join(keep, "\n\n")
}
