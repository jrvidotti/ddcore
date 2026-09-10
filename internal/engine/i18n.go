package engine

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/jrvidotti/cerne/internal/js"
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

func (i *I18n) T(lang, s string, args ...any) string {
	if lang == "" {
		lang = i.Lang
	}
	if t, ok := i.dict[lang][s]; ok && t != "" {
		s = t
	}
	for n, a := range args {
		s = strings.ReplaceAll(s, fmt.Sprintf("{%d}", n), fmt.Sprint(a))
	}
	return s
}

// Catalogue returns all translations for a language (served to the desk).
func (i *I18n) Catalogue(lang string) map[string]string {
	if lang == "" {
		lang = i.Lang
	}
	return i.dict[lang]
}
