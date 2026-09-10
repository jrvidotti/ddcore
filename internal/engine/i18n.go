package engine

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

// I18n holds the merged translation catalogue: core first, then each app
// (later apps win), read from translations/<lang>.csv (source,translation).
type I18n struct {
	Lang string
	dict map[string]map[string]string // lang -> source -> translation
}

func LoadI18n(apps []js.App, defaultLang string) (*I18n, error) {
	i := &I18n{Lang: defaultLang, dict: map[string]map[string]string{}}
	for _, a := range apps {
		for _, f := range js.ListFiles(a, ".csv") {
			if !strings.HasPrefix(f, "translations/") {
				continue
			}
			lang := strings.TrimSuffix(strings.TrimPrefix(f, "translations/"), ".csv")
			data, err := js.ReadFile(a, f)
			if err != nil {
				return nil, err
			}
			r := csv.NewReader(strings.NewReader(string(data)))
			r.FieldsPerRecord = -1
			r.LazyQuotes = true
			if i.dict[lang] == nil {
				i.dict[lang] = map[string]string{}
			}
			for {
				rec, err := r.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, fmt.Errorf("%s/%s: %w", a.Name, f, err)
				}
				if len(rec) >= 2 && rec[0] != "" {
					i.dict[lang][rec[0]] = rec[1]
				}
			}
		}
	}
	return i, nil
}

// T translates s and interpolates args. Translation and interpolation are
// separate steps on purpose: the arguments are values, never keys, so a
// translated template gets today's values, not yesterday's text.
func (i *I18n) T(lang, s string, args ...any) string {
	if lang == "" {
		lang = i.Lang
	}
	if t, ok := i.dict[lang][s]; ok && t != "" {
		s = t
	}
	return cerr.Render(s, args)
}

// Catalogue returns all translations for a language (served to the desk and
// mirrored into the JS runtime).
//
// It returns a copy. The dictionary is shared by every request in flight and
// is meant to be immutable for the life of a State; handing out the live map
// let one caller's stray write change what everyone else reads. The map is a
// few hundred entries and this is called once per session, not per string.
func (i *I18n) Catalogue(lang string) map[string]string {
	if lang == "" {
		lang = i.Lang
	}
	src := i.dict[lang]
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// Langs returns the languages with a catalogue, sorted. "en" is always present:
// it is the source language, so every key already resolves to itself.
func (i *I18n) Langs() []string {
	out := []string{"en"}
	for l := range i.dict {
		if l != "en" {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// HasLang reports whether lang is a language this site can serve.
func (i *I18n) HasLang(lang string) bool {
	if lang == "en" {
		return true
	}
	_, ok := i.dict[lang]
	return ok
}
