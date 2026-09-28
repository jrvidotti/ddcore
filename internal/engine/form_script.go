package engine

import (
	"path"
	"strings"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// FormScript picks, from an app's .form.ts files, the one that belongs to d,
// or "" when there is none. A form script is named after its DocType: either
// meta.Snake of the name — what the scaffold writes, and what an extending app
// uses — or the stem of the DocType's own .doctype.ts, so an app that names
// "TagOne Settings" tagone_settings.doctype.ts may name its script
// tagone_settings.form.ts next to it.
func FormScript(files []string, d *meta.DocType) string {
	stems := []string{meta.Snake(d.Name)}
	// SourceFile is a module path: <app>.doctypes.<dir>.<stem>.doctype
	if mp := strings.TrimSuffix(d.SourceFile, ".doctype"); mp != d.SourceFile {
		stems = append(stems, mp[strings.LastIndex(mp, ".")+1:])
	}
	for _, f := range files {
		base := path.Base(strings.ReplaceAll(f, "\\", "/"))
		for _, s := range stems {
			if base == s+".form.ts" {
				return f
			}
		}
	}
	return ""
}
