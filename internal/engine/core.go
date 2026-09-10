package engine

import (
	"github.com/jrvidotti/ddcore/core"
	"github.com/jrvidotti/ddcore/internal/js"
)

// CoreApp is the built-in app embedded in the binary.
func CoreApp() js.App {
	return js.App{Name: "core", Dir: "/ddcore/core", Embedded: core.FS}
}
