package engine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
)

const (
	globalSearchMinLen     = 2
	globalSearchMaxLen     = 140
	globalSearchDefault    = 20
	globalSearchMax        = 50
	globalSearchPerDocType = 5
	// What one DocType is asked for before ranking. The cap above is what it
	// contributes to the answer; reading only that many rows would decide the
	// ranking by `modified` instead, and drop an exact match the user is
	// almost certainly looking for.
	globalSearchScan = 50
)

// SearchHit is one document found by GlobalSearch.
type SearchHit struct {
	Doctype string `json:"doctype"`
	Label   string `json:"label"`
	Name    string `json:"name"`
	Title   string `json:"title"`
}

// GlobalSearch looks for txt across every globally searchable DocType, one
// query each through GetList, so roles, user permission scopes, shares and
// field levels decide what comes back exactly as they do for a list. A
// DocType the user cannot list is skipped, not reported.
//
// Hits rank exact matches of name or title first, then prefixes, then any
// other match; ties keep DocType and name order.
func (c *Ctx) GlobalSearch(txt string, limit int) ([]SearchHit, error) {
	txt = strings.TrimSpace(txt)
	if n := utf8.RuneCountInString(txt); n < globalSearchMinLen || n > globalSearchMaxLen {
		return nil, cerr.Validation("Search for between {0} and {1} characters", globalSearchMinLen, globalSearchMaxLen)
	}
	if limit <= 0 {
		limit = globalSearchDefault
	}
	limit = min(limit, globalSearchMax)
	type ranked struct {
		SearchHit
		rank int
	}
	var hits []ranked
	needle := db.FoldAccents(txt)
	for _, name := range c.St.Meta.Names() {
		d, err := c.St.DocType(name)
		if err != nil {
			return nil, err
		}
		if !d.GloballySearchable() {
			continue
		}
		args := searchArgs(d, txt)
		args.Limit = globalSearchScan
		rows, err := c.GetList(name, args)
		if err != nil {
			var ce *cerr.Error
			if errors.As(err, &ce) && ce.Type == "PermissionError" {
				continue
			}
			return nil, err
		}
		label := d.Label
		if label == "" {
			label = d.Name
		}
		for _, r := range rows {
			h := SearchHit{Doctype: d.Name, Label: c.T(label), Name: fmt.Sprint(r["name"]), Title: fmt.Sprint(r["name"])}
			if d.TitleField != "" {
				if t, ok := r[d.TitleField]; ok && t != nil && fmt.Sprint(t) != "" {
					h.Title = fmt.Sprint(t)
				}
			}
			hits = append(hits, ranked{h, searchRank(needle, h.Name, h.Title)})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].rank < hits[j].rank })
	// The per-DocType cap is applied here, on the ranked list, so one crowded
	// DocType cannot fill the answer and the best hit of each is kept.
	out := make([]SearchHit, 0, min(limit, len(hits)))
	taken := map[string]int{}
	for _, h := range hits {
		if len(out) >= limit {
			break
		}
		if taken[h.Doctype] >= globalSearchPerDocType {
			continue
		}
		taken[h.Doctype]++
		out = append(out, h.SearchHit)
	}
	return out, nil
}

// searchRank is 0 for an exact name or title, 1 for a prefix of either and 2
// for anything else that matched.
func searchRank(needle string, values ...string) int {
	best := 2
	for _, v := range values {
		v = db.FoldAccents(v)
		switch {
		case v == needle:
			return 0
		case strings.HasPrefix(v, needle):
			best = 1
		}
	}
	return best
}
