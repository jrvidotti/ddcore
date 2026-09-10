package meta

// TreeLabelKeys are the map keys whose value is human-facing text in the JSON
// trees that `defineWorkspace`, `defineReport` and `defineApp` produce. Those
// trees are `map[string]any` on the Go side, so translating them means walking
// them and touching exactly these keys.
//
// It is deliberately a closed list, and deliberately shared: the extractor
// (which decides what goes into the catalogue) and the runtime (which decides
// what gets translated) read the same names, so a key can never be collected
// but not translated, or the reverse.
//
// Everything absent is an identifier and must survive untouched — `name`,
// `route`, `doctype`, `report`, `icon`, `fieldname`, `options`, `color`, …
var TreeLabelKeys = map[string]bool{
	"label":       true,
	"title":       true,
	"description": true,
	"category":    true,
}
