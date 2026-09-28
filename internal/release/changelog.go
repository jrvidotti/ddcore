// Package release answers two questions the binary cannot answer from its own
// build alone: what changed in the releases since this one, and whether a newer
// one exists. Both `ddcore doctor` and the MCP server ask them, so the parsing
// and the network call live here once rather than in each caller.
package release

import (
	"io/fs"
	"path"
	"sort"
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
		// The root file wraps its releases in a VitePress region so the site
		// can include them without the preamble; the markers are not news.
		if !fenced && isRegionMarker(line) {
			continue
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

func isRegionMarker(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "<!-- #region ") || strings.HasPrefix(t, "<!-- #endregion ")
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

// Series is the changelog of one archived minor series, `<minor>.md` in the
// archive: Minor is "0.20", Text the whole file.
type Series struct {
	Minor string
	Text  string
}

// Archive reads every `changelog/<minor>.md` in fsys, newest series first. The
// current series is not there — it lives in the root CHANGELOG.md.
func Archive(fsys fs.FS) []Series {
	names, _ := fs.Glob(fsys, "changelog/*.md")
	var out []Series
	for _, n := range names {
		minor := strings.TrimSuffix(path.Base(n), ".md")
		if !engine.IsRelease(minor + ".0") {
			continue
		}
		b, err := fs.ReadFile(fsys, n)
		if err != nil {
			continue
		}
		out = append(out, Series{Minor: minor, Text: string(b)})
	}
	sort.Slice(out, func(i, j int) bool {
		newer, _ := engine.Newer(out[j].Minor+".0", out[i].Minor+".0")
		return newer
	})
	return out
}

// Full is the whole history as one document: the root changelog followed by
// the sections of every archived series, newest first. Only the sections of an
// archive are appended — its own heading and preamble would otherwise land in
// the body of the release above it.
func Full(root string, fsys fs.FS) string {
	parts := []string{strings.TrimRight(root, "\n")}
	for _, s := range Archive(fsys) {
		for _, sec := range Sections(s.Text) {
			parts = append(parts, sec.Body)
		}
	}
	return strings.Join(parts, "\n\n")
}
