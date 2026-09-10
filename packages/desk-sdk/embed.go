// Package desksdk embeds the TypeScript declarations used by external Cerne apps.
package desksdk

import "embed"

// FS contains the sources materialized by `cerne types`.
//
//go:embed src/*.ts
var FS embed.FS
