package mcp

import (
	"fmt"
	"path/filepath"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/i18nx"
)

// The i18n tools are `ddcore i18n extract` for an agent: the same extractor
// over the same catalogues, returning a structure instead of printed lines,
// plus a way to fill the catalogue without composing CSV by hand.

type i18nExtractIn struct {
	App   string `json:"app,omitempty" jsonschema:"only this app (default: every app plus the core)"`
	Lang  string `json:"lang,omitempty" jsonschema:"catalogue language (default: the site's lang)"`
	Check bool   `json:"check,omitempty" jsonschema:"write nothing; only report"`
	Prune bool   `json:"prune,omitempty" jsonschema:"drop entries the code no longer has"`
}

type setTranslationsIn struct {
	App          string            `json:"app" jsonschema:"the app whose catalogue receives the translations"`
	Lang         string            `json:"lang,omitempty" jsonschema:"catalogue language (default: the site's lang)"`
	Translations map[string]string `json:"translations" jsonschema:"English key → translation; every key must exist in the code (run i18n_extract first for a new label)"`
}

type i18nKey struct {
	Key string `json:"key"`
	Ref string `json:"ref,omitempty"`
}

type i18nTarget struct {
	App     string    `json:"app"`
	File    string    `json:"file"`
	Keys    int       `json:"keys"`
	Missing []i18nKey `json:"missing"`
	Orphans []string  `json:"orphans"`
	Dynamic []string  `json:"dynamic,omitempty"`
}

type i18nReport struct {
	Lang    string       `json:"lang"`
	Missing int          `json:"missing"`
	Targets []i18nTarget `json:"targets"`
}

// i18nSetup resolves what both tools share: the checkout root, the language
// and the extraction targets. `en` is refused as the CLI refuses it — the
// source language has no catalogue.
func (s *server) i18nSetup(app, lang string) (string, []i18nx.Target, error) {
	root := s.e.Cfg.Root
	if root == "" {
		return "", nil, cerr.Validation("the checkout root is unknown: the server was not started from a ddcore.json")
	}
	if lang == "" {
		lang = s.e.Cfg.Lang
	}
	if lang == "" || lang == "en" {
		return "", nil, cerr.Validation("lang {0} is the source language, it has no catalogue; pass the catalogue language (lang: \"pt-BR\")", "en")
	}
	targets := i18nx.Targets(s.e, root, app)
	if len(targets) == 0 {
		return "", nil, cerr.NotFound("app {0} does not exist (apps: {1})", app, s.e.AppOrder())
	}
	return lang, targets, nil
}

// i18nReportTarget extracts one target and describes its catalogue against
// the code. The caller decides whether anything is written.
func (s *server) i18nReportTarget(t i18nx.Target, lang string) (*i18nx.Set, *i18nx.Catalog, i18nTarget, error) {
	set, err := i18nx.Extract(s.e, t)
	if err != nil {
		return nil, nil, i18nTarget{}, err
	}
	path := t.CatalogPath(lang)
	cat, err := i18nx.ReadCatalog(path, lang)
	if err != nil {
		return nil, nil, i18nTarget{}, err
	}
	rel, err := filepath.Rel(t.Root, path)
	if err != nil {
		rel = path
	}
	out := i18nTarget{App: t.App, File: filepath.ToSlash(rel), Keys: set.Len(), Missing: []i18nKey{}, Orphans: []string{}}
	for _, k := range cat.Missing(set) {
		out.Missing = append(out.Missing, i18nKey{Key: k.Text, Ref: k.Ref()})
	}
	out.Orphans = append(out.Orphans, cat.Orphans(set)...)
	for _, d := range set.Dynamic {
		out.Dynamic = append(out.Dynamic, fmt.Sprintf("%s:%d", d.File, d.Line))
	}
	return set, cat, out, nil
}

func (s *server) i18nExtract(in i18nExtractIn) (i18nReport, error) {
	lang, targets, err := s.i18nSetup(in.App, in.Lang)
	if err != nil {
		return i18nReport{}, err
	}
	r := i18nReport{Lang: lang, Targets: []i18nTarget{}}
	for _, t := range targets {
		set, cat, tr, err := s.i18nReportTarget(t, lang)
		if err != nil {
			return i18nReport{}, err
		}
		if !in.Check {
			if err := cat.Write(set, in.Prune); err != nil {
				return i18nReport{}, err
			}
			if in.Prune {
				tr.Orphans = []string{}
			}
		}
		r.Missing += len(tr.Missing)
		r.Targets = append(r.Targets, tr)
	}
	if !in.Check {
		if err := s.e.Load(); err != nil {
			return i18nReport{}, err
		}
	}
	return r, nil
}

func (s *server) setTranslations(in setTranslationsIn) (i18nReport, error) {
	if in.App == "" {
		return i18nReport{}, cerr.Validation("app is required")
	}
	if len(in.Translations) == 0 {
		return i18nReport{}, cerr.Validation("translations is empty")
	}
	lang, targets, err := s.i18nSetup(in.App, in.Lang)
	if err != nil {
		return i18nReport{}, err
	}
	t := targets[0]
	set, cat, _, err := s.i18nReportTarget(t, lang)
	if err != nil {
		return i18nReport{}, err
	}
	if err := cat.Set(set, in.Translations); err != nil {
		return i18nReport{}, cerr.Validation("{0}; run i18n_extract to see the keys the code has", err.Error())
	}
	// The running catalogue must reflect the file now, not when the watcher
	// gets to it: the agent's next call may read a label.
	if err := s.e.Load(); err != nil {
		return i18nReport{}, err
	}
	_, _, tr, err := s.i18nReportTarget(t, lang)
	if err != nil {
		return i18nReport{}, err
	}
	return i18nReport{Lang: lang, Missing: len(tr.Missing), Targets: []i18nTarget{tr}}, nil
}
