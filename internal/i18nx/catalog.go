package i18nx

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Catalog is one translations/<lang>.csv: key → translation, plus the order
// nothing depends on (the file is always rewritten sorted).
type Catalog struct {
	Path  string
	Lang  string
	Trans map[string]string
}

// ReadCatalog loads a catalogue; a missing file is an empty one, not an error,
// because the first extraction of a language creates it.
func ReadCatalog(path, lang string) (*Catalog, error) {
	c := &Catalog{Path: path, Lang: lang, Trans: map[string]string{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) >= 2 && rec[0] != "" {
			c.Trans[rec[0]] = rec[1]
		}
	}
	return c, nil
}

// Missing lists the keys the code has and the catalogue does not translate.
func (c *Catalog) Missing(s *Set) []Key {
	var out []Key
	for _, k := range s.Keys() {
		if t, ok := c.Trans[k.Text]; !ok || t == "" {
			out = append(out, k)
		}
	}
	return out
}

// Orphans lists the catalogue entries no longer present in the code.
func (c *Catalog) Orphans(s *Set) []string {
	var out []string
	for k := range c.Trans {
		if !s.Has(k) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Write rewrites the catalogue from the extracted set: sorted, three columns
// (key, translation, source reference), keeping every translation already
// there. With prune, keys the code no longer has are dropped; without it they
// are kept at the end so a rename does not throw away someone's work.
func (c *Catalog) Write(s *Set, prune bool) error {
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	for _, k := range s.Keys() {
		if err := w.Write([]string{k.Text, c.Trans[k.Text], k.Ref()}); err != nil {
			return err
		}
	}
	if !prune {
		for _, k := range c.Orphans(s) {
			if c.Trans[k] == "" {
				continue
			}
			if err := w.Write([]string{k, c.Trans[k], "# orphan: no longer in the code"}); err != nil {
				return err
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return os.WriteFile(c.Path, []byte(b.String()), 0o644)
}
