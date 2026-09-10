// Package desk embeds the compiled SvelteKit app (desk/build).
package desk

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var buildFS embed.FS

// FS returns the build directory, or nil when the desk was not compiled.
func FS() fs.FS {
	sub, err := fs.Sub(buildFS, "build")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
