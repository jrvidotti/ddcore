package api

import (
	"net/http"
	"time"

	"golang.org/x/text/language"
)

// langCacheTTL is how long a user's resolved language stays in the process
// cache. Short enough that changing User.language is felt quickly even if the
// invalidation in the controller ever fails; long enough to keep the boot from
// hitting the database on every request.
const langCacheTTL = 15 * time.Minute

// langFor resolves the language of a request, in order:
//
//  1. an X-Lang header naming a language this site serves
//  2. the requesting user's User.language, cached under "lang:<user>"
//  3. for Guest only, Accept-Language intersected with the site's languages
//  4. the site default (ddcore.json:lang)
//  5. "en"
//
// It is a pure function of the request plus the cache: it never touches the
// database, so any handler holding an *http.Request can call it.
func (s *Server) langFor(r *http.Request) string {
	i18n := s.E.Current().I18n
	if l := r.Header.Get("X-Lang"); l != "" && i18n.HasLang(l) {
		return l
	}
	u := user(r)
	if u != "Guest" {
		if v, ok := s.E.Cache.Get("lang:" + u); ok {
			if l, _ := v.(string); l != "" && i18n.HasLang(l) {
				return l
			}
		}
	} else if l := matchAcceptLanguage(r.Header.Get("Accept-Language"), i18n.Langs()); l != "" {
		return l
	}
	if l := s.E.Cfg.Lang; l != "" {
		return l
	}
	return "en"
}

// matchAcceptLanguage picks the best of `have` for an Accept-Language header,
// or "" when nothing matches well enough.
func matchAcceptLanguage(header string, have []string) string {
	if header == "" || len(have) == 0 {
		return ""
	}
	tags := make([]language.Tag, 0, len(have))
	for _, l := range have {
		t, err := language.Parse(l)
		if err != nil {
			continue
		}
		tags = append(tags, t)
	}
	if len(tags) == 0 {
		return ""
	}
	desired, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(desired) == 0 {
		return ""
	}
	_, idx, conf := language.NewMatcher(tags).Match(desired...)
	if conf == language.No || idx < 0 || idx >= len(have) {
		return ""
	}
	// NewMatcher was built from `tags`, which may be shorter than `have`; map
	// back through the parsed tag so the returned string is a site language.
	want := tags[idx].String()
	for _, l := range have {
		if l == want {
			return l
		}
		if t, err := language.Parse(l); err == nil && t == tags[idx] {
			return l
		}
	}
	return ""
}
