// Package desksdk embeds the TypeScript declarations used by external DDCore apps.
package desksdk

import "embed"

// FS contains the sources materialized by `ddcore types`.
//
//go:embed src/*.ts
var FS embed.FS
