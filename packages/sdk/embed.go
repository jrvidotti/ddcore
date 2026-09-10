// Package sdk embeds the TypeScript sources of @cerne/sdk so the binary can
// resolve `import ... from "@cerne/sdk"` without node_modules.
package sdk

import "embed"

//go:embed src/*.ts
var FS embed.FS
