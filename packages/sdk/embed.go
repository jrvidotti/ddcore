// Package sdk embeds the TypeScript sources of @ddcore/sdk so the binary can
// resolve `import ... from "@ddcore/sdk"` without node_modules.
package sdk

import "embed"

//go:embed src/*.ts
var FS embed.FS
