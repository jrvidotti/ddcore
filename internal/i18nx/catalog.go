package i18nx

import (
	"encoding/csv"
	"fmt"
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
	err = readRows(b, func(key, trans, _ string) { c.Trans[key] = trans })
	if err != nil {
		return nil, err
	}
	return c, nil
}

// readRows calls row for every entry of a catalogue's CSV, with its third
// column (the extractor's comment) or "" when the row has none.
func readRows(b []byte, row func(key, trans, comment string)) error {
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if len(rec) >= 2 && rec[0] != "" {
			comment := ""
			if len(rec) >= 3 {
				comment = rec[2]
			}
			row(rec[0], rec[1], comment)
		}
	}
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

// Orphans lists the catalogue entries no longer present in the code. A key an
// earlier app translates is an override, not an orphan (see Overrides).
func (c *Catalog) Orphans(s *Set) []string {
	var out []string
	for k := range c.Trans {
		if _, inherited := s.Inherited[k]; !s.Has(k) && !inherited {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Overrides lists the catalogue entries the code does not have but an app
// loaded earlier translates: this catalogue merges after that app's, so its
// translation is the one the site shows.
func (c *Catalog) Overrides(s *Set) []string {
	var out []string
	for k := range c.Trans {
		if _, inherited := s.Inherited[k]; inherited && !s.Has(k) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Write rewrites the catalogue from the extracted set: sorted, three columns
// (key, translation, source reference), keeping every translation already
// there. With prune, keys the code no longer has are dropped; without it they
// are kept at the end so a rename does not throw away someone's work. An
// override of an earlier app's key is kept either way: it is not stale, the
// code that has it is just someone else's.
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
	for _, k := range c.Overrides(s) {
		if c.Trans[k] == "" {
			continue
		}
		if err := w.Write([]string{k, c.Trans[k], "# overrides " + s.Inherited[k]}); err != nil {
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

// Set applies translations and rewrites the catalogue. A key neither the code
// nor an earlier app has is refused — all of them at once, with nothing
// applied — because the likeliest cause is a typo on the caller's side, and
// accepting it would quietly create an orphan the reader never sees
// translated. An earlier app's key is accepted: it is an override.
func (c *Catalog) Set(s *Set, trans map[string]string) error {
	var unknown []string
	for k := range trans {
		if _, inherited := s.Inherited[k]; !s.Has(k) && !inherited {
			unknown = append(unknown, fmt.Sprintf("%q", k))
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%d key(s) not in the code: %s", len(unknown), strings.Join(unknown, ", "))
	}
	for k, v := range trans {
		c.Trans[k] = v
	}
	return c.Write(s, false)
}
