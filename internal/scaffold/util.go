package scaffold

import "regexp"

var keyRe = regexp.MustCompile(`(?m)^(\s*)"([a-zA-Z_][a-zA-Z0-9_]*)":`)
