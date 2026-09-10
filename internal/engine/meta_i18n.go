package engine

import (
	"sync"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// Metadata is served translated, from the server.
//
// A `label:` in a DocType is a key like any other; it just happens to be
// declared in TypeScript instead of called through `_()`. Translating it here,
// at the same border that translates errors, means a third-party app gets
// translated metadata without a line of code: `LoadI18n` already reads
// `translations/*.csv` from every app.
//
// The registry is never mutated. It is shared by every request in flight, and
// a request's language is not a property of the meta — so these functions
// always return a copy, and the copies are cached per (doctype, language).
//
// Invalidation is free: a reload replaces the whole *State (engine.go), so the
// cache dies with the state that built it.

type metaCache struct {
	mu sync.RWMutex
	m  map[string]*meta.DocType // "<doctype>\x00<lang>"
}

func (c *metaCache) get(key string) (*meta.DocType, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.m[key]
	return d, ok
}

func (c *metaCache) put(key string, d *meta.DocType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]*meta.DocType{}
	}
	c.m[key] = d
}

// TranslateDocType returns a copy of d with its human-facing text in lang.
// Never mutates d.
func (st *State) TranslateDocType(d *meta.DocType, lang string) *meta.DocType {
	if d == nil {
		return nil
	}
	key := d.Name + "\x00" + lang
	if out, ok := st.metaCache.get(key); ok {
		return out
	}
	t := func(s string) string {
		if s == "" {
			return s
		}
		return st.I18n.T(lang, s)
	}
	out := *d
	out.Label = t(d.Label)
	out.Description = t(d.Description)
	out.Fields = make([]*meta.Field, len(d.Fields))
	for i, f := range d.Fields {
		// Section, Column and Tab Breaks are copied too: a break's label is a
		// heading on the form.
		cp := *f
		cp.Label = t(f.Label)
		cp.Description = t(f.Description)
		if opts, ok := f.Options.([]string); ok {
			cp.OptionLabels = translateEach(t, opts)
		} else if opts, ok := f.Options.([]any); ok {
			var ss []string
			for _, o := range opts {
				s, _ := o.(string)
				ss = append(ss, s)
			}
			cp.OptionLabels = translateEach(t, ss)
		}
		out.Fields[i] = &cp
	}
	// the copy's field index belongs to the copy; let it rebuild lazily
	out.ResetFieldIndex()
	st.metaCache.put(key, &out)
	return &out
}

func translateEach(t func(string) string, opts []string) []string {
	if len(opts) == 0 {
		return nil
	}
	out := make([]string, len(opts))
	same := true
	for i, o := range opts {
		out[i] = t(o)
		if out[i] != o {
			same = false
		}
	}
	if same {
		// nothing was translated: leave the field out of the payload rather
		// than shipping a duplicate of Options
		return nil
	}
	return out
}

// TranslateTree copies v, translating the strings under meta.TreeLabelKeys and
// nothing else. Workspaces and reports reach Go as `map[string]any`, so this
// is how their labels get translated — and the closed key list is the same one
// the extractor collects by, so a key cannot be collected and not translated,
// or the reverse.
func (st *State) TranslateTree(v any, lang string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if s, ok := val.(string); ok {
				if meta.TreeLabelKeys[k] && s != "" {
					out[k] = st.I18n.T(lang, s)
				} else {
					out[k] = s
				}
				continue
			}
			out[k] = st.TranslateTree(val, lang)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = st.TranslateTree(val, lang)
		}
		return out
	}
	return v
}

// TranslateStringMap is TranslateTree for the `map[string]map[string]any` the
// snapshot keeps workspaces and reports in.
func (st *State) TranslateStringMap(m map[string]any, lang string) map[string]any {
	out, _ := st.TranslateTree(m, lang).(map[string]any)
	if out == nil {
		return m
	}
	return out
}
