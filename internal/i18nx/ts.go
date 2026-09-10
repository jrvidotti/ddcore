package i18nx

import (
	"os"
	"path/filepath"
	"strings"
)

// This file collects `_("…")` and `__("…")` from TypeScript and Svelte.
//
// It lexes rather than pattern-matches. A regex over source finds the calls
// inside comments, inside strings and inside `//`-commented-out code, and
// misses the ones split across lines — and a catalogue built from a regex is
// wrong in a way nobody notices, because a missing key silently renders as
// English. The lexer below is small but it knows what a string, a comment, a
// template literal and a regex literal are, which is all it takes to be right.

type tokKind int

const (
	tokIdent tokKind = iota
	tokString
	tokPunct
)

type tsToken struct {
	kind tokKind
	val  string // for tokString, the decoded value
	line int
}

// CollectTS walks dir and collects from every .ts/.js/.svelte file that pred
// accepts (nil accepts all). Paths in the Set are relative to rel.
func CollectTS(s *Set, dir, rel string, skip func(path string) bool) error {
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if n == "node_modules" || n == "build" || n == "dist" || strings.HasPrefix(n, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(p)
		if ext != ".ts" && ext != ".js" && ext != ".svelte" {
			return nil
		}
		name := p
		if r, err := filepath.Rel(rel, p); err == nil {
			name = r
		}
		if skip != nil && skip(filepath.ToSlash(name)) {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		collectSource(s, string(src), name, ext == ".svelte")
		return nil
	})
}

func collectSource(s *Set, src, name string, svelte bool) {
	if svelte {
		for _, r := range svelteScripts(src) {
			collectTokens(s, lexJS(src[r[0]:r[1]], lineAt(src, r[0])), name)
		}
		return
	}
	collectTokens(s, lexJS(src, 1), name)
}

// collectTokens finds `_(` / `__(` followed by a string literal.
func collectTokens(s *Set, toks []tsToken, file string) {
	for i, t := range toks {
		if t.kind != tokIdent || (t.val != "_" && t.val != "__") {
			continue
		}
		// `function __(s)` is the definition, not a call site.
		if i > 0 && toks[i-1].kind == tokIdent && toks[i-1].val == "function" {
			continue
		}
		if i+1 >= len(toks) || toks[i+1].kind != tokPunct || toks[i+1].val != "(" {
			continue
		}
		if i+2 < len(toks) && toks[i+2].kind == tokString {
			s.Add(toks[i+2].val, file, toks[i+2].line)
			continue
		}
		// A declaration, not a call: `_(text: string, args?: any[])` in an
		// interface or a type. The parameter name is followed by `:` or `?`,
		// where an argument would be followed by `)`, `.` or `,`.
		if i+3 < len(toks) && toks[i+2].kind == tokIdent && toks[i+3].kind == tokPunct &&
			(toks[i+3].val == ":" || toks[i+3].val == "?") {
			continue
		}
		// A computed key: the value is its own key at run time (a Select
		// option, a status). Reported, never fatal.
		s.AddDynamic(file, t.line)
	}
}

func lineAt(src string, off int) int { return 1 + strings.Count(src[:off], "\n") }

// svelteScripts returns the byte ranges of a Svelte file that are JavaScript:
// every <script> block, and every `{…}` expression in the markup. Markup text
// is skipped, so an apostrophe in prose cannot open a phantom string literal.
func svelteScripts(src string) [][2]int {
	var out [][2]int
	i := 0
	for i < len(src) {
		switch {
		case startsTag(src, i, "<script"):
			open := strings.IndexByte(src[i:], '>')
			if open < 0 {
				return out
			}
			start := i + open + 1
			end := indexFold(src[start:], "</script")
			if end < 0 {
				out = append(out, [2]int{start, len(src)})
				return out
			}
			out = append(out, [2]int{start, start + end})
			i = start + end
		case startsTag(src, i, "<style"):
			end := indexFold(src[i:], "</style")
			if end < 0 {
				return out
			}
			i += end
		case src[i] == '{':
			end := matchBrace(src, i)
			// skip the sigil of {#if}, {:else}, {/each}, {@const}
			start := i + 1
			if start < end && strings.IndexByte("#:/@", src[start]) >= 0 {
				start++
			}
			if start < end {
				out = append(out, [2]int{start, end})
			}
			i = end + 1
		default:
			i++
		}
	}
	return out
}

func startsTag(src string, i int, tag string) bool {
	if i+len(tag) > len(src) {
		return false
	}
	if !strings.EqualFold(src[i:i+len(tag)], tag) {
		return false
	}
	r := src[i+len(tag):]
	return r == "" || r[0] == '>' || r[0] == ' ' || r[0] == '\t' || r[0] == '\n' || r[0] == '\r'
}

func indexFold(s, sub string) int { return strings.Index(strings.ToLower(s), strings.ToLower(sub)) }

// matchBrace returns the offset of the `}` closing the `{` at i, skipping
// nested braces and anything inside a string, template or comment.
func matchBrace(src string, i int) int {
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		case '"', '\'', '`':
			_, end := readString(src, j)
			j = end - 1
		case '/':
			if j+1 < len(src) && (src[j+1] == '/' || src[j+1] == '*') {
				j = skipComment(src, j) - 1
			}
		}
	}
	return len(src)
}

// lexJS turns JavaScript-ish source into the few token kinds the collector
// needs. firstLine is the line number of src[0] in the original file.
func lexJS(src string, firstLine int) []tsToken {
	var toks []tsToken
	line := firstLine
	prevSignificant := func() *tsToken {
		if len(toks) == 0 {
			return nil
		}
		return &toks[len(toks)-1]
	}
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < len(src) && (src[i+1] == '/' || src[i+1] == '*'):
			end := skipComment(src, i)
			line += strings.Count(src[i:end], "\n")
			i = end
		case c == '/' && regexAllowed(prevSignificant()):
			end := skipRegex(src, i)
			line += strings.Count(src[i:end], "\n")
			i = end
		case c == '"' || c == '\'' || c == '`':
			val, end := readString(src, i)
			startLine := line
			line += strings.Count(src[i:end], "\n")
			// A template literal with a `${…}` is never a plain key; keep it
			// as a token so `__(`+"`"+`…`+"`"+`)` reads as a dynamic call.
			toks = append(toks, tsToken{kind: tokString, val: val, line: startLine})
			if c == '`' && strings.Contains(src[i:end], "${") {
				toks[len(toks)-1].kind = tokPunct
				toks[len(toks)-1].val = "`"
			}
			i = end
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, tsToken{kind: tokIdent, val: src[i:j], line: line})
			i = j
		default:
			toks = append(toks, tsToken{kind: tokPunct, val: string(c), line: line})
			i++
		}
	}
	return toks
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// regexAllowed decides whether a `/` opens a regex literal or is division,
// from the token before it — the standard heuristic, and enough here.
func regexAllowed(prev *tsToken) bool {
	if prev == nil {
		return true
	}
	switch prev.kind {
	case tokString:
		return false
	case tokIdent:
		switch prev.val {
		case "return", "typeof", "instanceof", "in", "of", "new", "delete", "void", "case", "do", "else", "yield", "await":
			return true
		}
		return false
	}
	// after a value-ending punctuator, `/` is division
	return strings.IndexByte(")]}", prev.val[0]) < 0
}

func skipComment(src string, i int) int {
	if src[i+1] == '/' {
		if j := strings.IndexByte(src[i:], '\n'); j >= 0 {
			return i + j
		}
		return len(src)
	}
	if j := strings.Index(src[i+2:], "*/"); j >= 0 {
		return i + 2 + j + 2
	}
	return len(src)
}

func skipRegex(src string, i int) int {
	inClass := false
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				// consume flags
				j++
				for j < len(src) && isIdentPart(src[j]) {
					j++
				}
				return j
			}
		case '\n':
			// an unterminated regex is not a regex: it was division
			return i + 1
		}
	}
	return len(src)
}

// readString reads the literal starting at i and returns its decoded value
// and the offset just past the closing quote.
func readString(src string, i int) (string, int) {
	quote := src[i]
	var b strings.Builder
	for j := i + 1; j < len(src); j++ {
		c := src[j]
		switch c {
		case '\\':
			if j+1 >= len(src) {
				return b.String(), len(src)
			}
			j++
			switch src[j] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\n':
				// line continuation
			default:
				b.WriteByte(src[j])
			}
		case quote:
			return b.String(), j + 1
		case '\n':
			if quote != '`' {
				// unterminated: treat the line end as the end so one bad
				// literal cannot swallow the rest of the file
				return b.String(), j
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), len(src)
}
