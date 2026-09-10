// Package core embeds the built-in app: the DocTypes the framework itself needs.
package core

import "embed"

//go:embed cerne.app.ts doctypes translations
var FS embed.FS
