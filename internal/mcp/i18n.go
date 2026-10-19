package mcp

import (
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
	Translations map[string]string `json:"translations" jsonschema:"English key → translation; every key must exist in the code (run i18n_extract first for a new label) or be translated by an app loaded earlier, the core included, which this catalogue then overrides"`
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
		return "", nil, cerr.NotFound("app {0} does not exist (apps: {1})", app, s.e.Current().AppOrder())
	}
	return lang, targets, nil
}

func (s *server) i18nExtract(in i18nExtractIn) (i18nx.Report, error) {
	lang, targets, err := s.i18nSetup(in.App, in.Lang)
	if err != nil {
		return i18nx.Report{}, err
	}
	r, err := i18nx.BuildReport(s.e, targets, lang, i18nx.ReportOpts{Write: !in.Check, Prune: in.Prune})
	if err != nil {
		return i18nx.Report{}, err
	}
	if !in.Check {
		if in.Prune {
			// pruned: the orphans are gone from the file
			for i := range r.Targets {
				r.Targets[i].Orphans = []string{}
			}
		}
		if err := s.e.Load(); err != nil {
			return i18nx.Report{}, err
		}
	}
	return r, nil
}

func (s *server) setTranslations(in setTranslationsIn) (i18nx.Report, error) {
	if in.App == "" {
		return i18nx.Report{}, cerr.Validation("app is required")
	}
	if len(in.Translations) == 0 {
		return i18nx.Report{}, cerr.Validation("translations is empty")
	}
	lang, targets, err := s.i18nSetup(in.App, in.Lang)
	if err != nil {
		return i18nx.Report{}, err
	}
	t := targets[0]
	set, cat, _, err := i18nx.Describe(s.e, t, lang)
	if err != nil {
		return i18nx.Report{}, err
	}
	if err := cat.Set(set, in.Translations); err != nil {
		return i18nx.Report{}, cerr.Validation("{0}; run i18n_extract to see the keys the code has", err.Error())
	}
	// The running catalogue must reflect the file now, not when the watcher
	// gets to it: the agent's next call may read a label.
	if err := s.e.Load(); err != nil {
		return i18nx.Report{}, err
	}
	_, _, tr, err := i18nx.Describe(s.e, t, lang)
	if err != nil {
		return i18nx.Report{}, err
	}
	return i18nx.Report{Lang: lang, Missing: len(tr.Missing), Targets: []i18nx.TargetReport{tr}}, nil
}
