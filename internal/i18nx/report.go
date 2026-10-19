package i18nx

import (
	"fmt"
	"path/filepath"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// ReportKey is a missing key: the English text and where the code has it.
type ReportKey struct {
	Key string `json:"key"`
	Ref string `json:"ref,omitempty"`
}

// ReportOverride is a catalogue entry for a key an earlier app translates: App
// is that app, whose translation this catalogue replaces.
type ReportOverride struct {
	Key string `json:"key"`
	App string `json:"app"`
}

// TargetReport describes one catalogue against the code. File is slash
// separated and relative to the checkout root; Dynamic lists "file:line" of
// the labels only known at run time.
type TargetReport struct {
	App       string           `json:"app"`
	File      string           `json:"file"`
	Keys      int              `json:"keys"`
	Missing   []ReportKey      `json:"missing"`
	Orphans   []string         `json:"orphans"`
	Overrides []ReportOverride `json:"overrides"`
	Dynamic   []string         `json:"dynamic,omitempty"`
}

// Report is the run over every target of one language; Missing totals the
// targets'. `ddcore i18n extract` prints it and the i18n_extract tool returns
// it as JSON.
type Report struct {
	Lang    string         `json:"lang"`
	Missing int            `json:"missing"`
	Targets []TargetReport `json:"targets"`
}

// ReportOpts says what a run does besides reporting: Write rewrites each
// catalogue from the code and Prune drops the entries the code no longer has.
type ReportOpts struct {
	Write bool
	Prune bool
}

// Describe extracts one target and describes its catalogue against the code.
// It writes nothing; the Set and the Catalog come back for a caller that does.
func Describe(e *engine.Engine, t Target, lang string) (*Set, *Catalog, TargetReport, error) {
	set, err := Extract(e, t)
	if err != nil {
		return nil, nil, TargetReport{}, err
	}
	path := t.CatalogPath(lang)
	cat, err := ReadCatalog(path, lang)
	if err != nil {
		return nil, nil, TargetReport{}, err
	}
	rel, err := filepath.Rel(t.Root, path)
	if err != nil {
		rel = path
	}
	out := TargetReport{App: t.App, File: filepath.ToSlash(rel), Keys: set.Len(), Missing: []ReportKey{}, Orphans: []string{}, Overrides: []ReportOverride{}}
	for _, k := range cat.Missing(set) {
		out.Missing = append(out.Missing, ReportKey{Key: k.Text, Ref: k.Ref()})
	}
	out.Orphans = append(out.Orphans, cat.Orphans(set)...)
	for _, k := range cat.Overrides(set) {
		out.Overrides = append(out.Overrides, ReportOverride{Key: k, App: set.Inherited[k]})
	}
	for _, d := range set.Dynamic {
		out.Dynamic = append(out.Dynamic, fmt.Sprintf("%s:%d", d.File, d.Line))
	}
	return set, cat, out, nil
}

// BuildReport runs Describe over each target in turn, writing each catalogue
// right after it is described when o.Write is set. On an error the report
// holds the targets done up to it, the failing one included once its
// description succeeded, so a caller printing as it goes loses nothing.
func BuildReport(e *engine.Engine, targets []Target, lang string, o ReportOpts) (Report, error) {
	r := Report{Lang: lang, Targets: []TargetReport{}}
	for _, t := range targets {
		set, cat, tr, err := Describe(e, t, lang)
		if err != nil {
			return r, err
		}
		r.Missing += len(tr.Missing)
		r.Targets = append(r.Targets, tr)
		if o.Write {
			if err := cat.Write(set, o.Prune); err != nil {
				return r, err
			}
		}
	}
	return r, nil
}
