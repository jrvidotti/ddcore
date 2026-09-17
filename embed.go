// Package ddcore embeds the repository documents the binary serves at runtime.
package ddcore

import _ "embed"

// Changelog is CHANGELOG.md, served as the MCP resource ddcore://changelog and
// read by the whats_new tool. It is embedded here and not in the docs package
// because a go:embed pattern cannot reach outside its own directory, and the
// file has to stay at the repository root for GitHub to render it.
//
//go:embed CHANGELOG.md
var Changelog string
