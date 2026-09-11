// Package core embeds the built-in app: the DocTypes the framework itself needs.
package core

import "embed"

//go:embed ddcore.app.ts doctypes services translations
var FS embed.FS
