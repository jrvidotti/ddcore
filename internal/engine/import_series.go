package engine

// Reading an id back into the counter that would have produced it (DAT-01).
//
// An import keeps the ids it is given, so `ddcore_series` never moves — and the
// first document created afterwards takes PED-00001 again, colliding with what
// was just loaded. This is the inverse of nextInSeries and formatID: it rebuilds
// the counter key from the id itself, so the load can advance the counter past
// what it wrote.
//
// It reads the date segments out of the id rather than rendering today's: an
// import loads last year's documents, and last year's counter is the one to
// move.

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// seriesCounterFor reads a document's id as the output of its DocType's
// idGeneration rule and returns the counter key and the number it used. ok is
// false when the DocType has no counter, when the document is an amendment (it
// keeps its original's id), or when the id does not fit the pattern — a legacy
// key that came in from somewhere else leaves the counter alone rather than
// moving it by a guess.
func seriesCounterFor(d *meta.DocType, doc Doc) (string, int64, bool) {
	id := strings.TrimSpace(doc.Str("id"))
	if id == "" || doc.Str("amended_from") != "" {
		return "", 0, false
	}
	g := d.IDGeneration
	switch {
	case g.Series != "":
		series := g.Series
		if s := doc.Str("id_series"); s != "" {
			series = s
		}
		return matchSegments(seriesSegments(series, doc), id)
	case g.Format != "":
		return matchSegments(formatSegments(g.Format, doc), id)
	}
	return "", 0, false
}

// segment is one piece of a rendered id: a literal (already resolved from the
// document), a date whose digits are read out of the id, or the counter.
type segment struct {
	literal string
	digits  int // >0 for a date segment of that width
	counter bool
}

// seriesSegments splits a Frappe-style series the way nextInSeries renders it.
func seriesSegments(series string, doc Doc) []segment {
	var segs []segment
	for _, p := range strings.Split(series, ".") {
		switch {
		case p == "":
			continue
		case hashRun.MatchString(p) && strings.Trim(p, "#") == "":
			segs = append(segs, segment{counter: true})
		case p == "YYYY":
			segs = append(segs, segment{digits: 4})
		case p == "YY", p == "MM", p == "DD":
			segs = append(segs, segment{digits: 2})
		case strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}"):
			segs = append(segs, segment{literal: doc.Str(strings.Trim(p, "{}"))})
		default:
			segs = append(segs, segment{literal: p})
		}
	}
	return segs
}

// formatSegments does the same for a "{imovel}-{YYYY}-{###}" format, where the
// counter may sit anywhere.
func formatSegments(format string, doc Doc) []segment {
	var segs []segment
	rest := format
	for {
		loc := fmtField.FindStringIndex(rest)
		if loc == nil {
			if rest != "" {
				segs = append(segs, segment{literal: rest})
			}
			return segs
		}
		if loc[0] > 0 {
			segs = append(segs, segment{literal: rest[:loc[0]]})
		}
		k := strings.Trim(rest[loc[0]:loc[1]], "{}")
		switch {
		case k == "YYYY":
			segs = append(segs, segment{digits: 4})
		case k == "YY", k == "MM", k == "DD":
			segs = append(segs, segment{digits: 2})
		case strings.Trim(k, "#") == "":
			segs = append(segs, segment{counter: true})
		default:
			segs = append(segs, segment{literal: doc.Str(k)})
		}
		rest = rest[loc[1]:]
	}
}

// matchSegments reads the id through the segments, returning the counter key —
// everything the rendering produced except the counter — and the counter.
func matchSegments(segs []segment, id string) (string, int64, bool) {
	var pattern strings.Builder
	counters := 0
	pattern.WriteString("^")
	for _, s := range segs {
		switch {
		case s.counter:
			counters++
			pattern.WriteString(`(\d+)`)
		case s.digits > 0:
			pattern.WriteString(`(\d{` + strconv.Itoa(s.digits) + `})`)
		default:
			pattern.WriteString("(" + regexp.QuoteMeta(s.literal) + ")")
		}
	}
	pattern.WriteString("$")
	if counters != 1 {
		return "", 0, false
	}
	re, err := regexp.Compile(pattern.String())
	if err != nil {
		return "", 0, false
	}
	m := re.FindStringSubmatch(id)
	if m == nil {
		return "", 0, false
	}
	var key strings.Builder
	var n int64
	for i, s := range segs {
		if s.counter {
			v, err := strconv.ParseInt(m[i+1], 10, 64)
			if err != nil {
				return "", 0, false
			}
			n = v
			continue
		}
		key.WriteString(m[i+1])
	}
	return key.String(), n, true
}
