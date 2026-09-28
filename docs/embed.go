// Package docs embeds the framework reference written for agents.
package docs

import "embed"

//go:embed agent/*.md
var FS embed.FS

// ChangelogFS holds the changelog of every minor series older than the current
// one, one `changelog/<minor>.md` per series; the current series stays in the
// root CHANGELOG.md. The pattern names the series files only, so the site's
// changelog/index.md is not embedded.
//
//go:embed changelog/0.*.md
var ChangelogFS embed.FS
