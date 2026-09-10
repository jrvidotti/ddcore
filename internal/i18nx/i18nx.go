// Package i18nx extracts translatable strings from a ddcore checkout: the
// `_()`/`__()` calls in TypeScript and Svelte, the `label:`/`description:` of
// DocTypes, workspaces and reports, and the message of every `cerr.*` in Go.
//
// The catalogue is derived from the code, never maintained by hand. The
// extractor is the only thing that decides what a key is, so `--check` can
// state, without judgement, what is missing and what is stale.
package i18nx

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Key is one translatable string plus where it was first seen.
type Key struct {
	Text string
	File string // repo-relative, slash-separated
	Line int
}

// Set is the ordered collection the collectors fill in. The first sighting of
// a text wins the reference, so the CSV comment is stable across runs.
type Set struct {
	byText map[string]*Key
	// Dynamic records `_(expr)` call sites, where the key is only known at
	// run time. Frappe does the same and phase 7 depends on it (a Select
	// value is its own key), so it is a warning, never an error.
	Dynamic []Key
}

func NewSet() *Set { return &Set{byText: map[string]*Key{}} }

// Add records text seen at file:line. Empty and whitespace-only texts are
// dropped: they are never worth translating and only add noise.
func (s *Set) Add(text, file string, line int) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if _, ok := s.byText[text]; ok {
		return
	}
	s.byText[text] = &Key{Text: text, File: filepath.ToSlash(file), Line: line}
}

func (s *Set) AddDynamic(file string, line int) {
	s.Dynamic = append(s.Dynamic, Key{File: filepath.ToSlash(file), Line: line})
}

func (s *Set) Has(text string) bool { _, ok := s.byText[text]; return ok }

func (s *Set) Len() int { return len(s.byText) }

// Keys returns every key sorted by text, which is the order the CSV keeps.
func (s *Set) Keys() []Key {
	out := make([]Key, 0, len(s.byText))
	for _, k := range s.byText {
		out = append(out, *k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Text < out[j].Text })
	return out
}

// Ref is the third CSV column: where a reader can go look at the string.
func (k Key) Ref() string {
	if k.File == "" {
		return ""
	}
	if k.Line <= 0 {
		return "# " + k.File
	}
	return fmt.Sprintf("# %s:%d", k.File, k.Line)
}
