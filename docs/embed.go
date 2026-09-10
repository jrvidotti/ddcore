// Package docs embeds the framework reference written for agents.
package docs

import "embed"

//go:embed agent/*.md
var FS embed.FS
