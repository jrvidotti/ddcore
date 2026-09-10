package engine

import (
	"github.com/jrvidotti/cerne/core"
	"github.com/jrvidotti/cerne/internal/js"
)

// CoreApp is the built-in app embedded in the binary.
func CoreApp() js.App {
	return js.App{Name: "core", Dir: "/cerne/core", Embedded: core.FS}
}
