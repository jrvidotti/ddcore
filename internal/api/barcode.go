package api

import (
	"errors"
	"net/http"

	"github.com/jrvidotti/ddcore/internal/barcode"
	"github.com/jrvidotti/ddcore/internal/cerr"
)

// barcodeSVG draws `value` in `symbology` for an <img>: the desk's preview of
// a Barcode field and a portal page showing one. It reads nothing from the
// database, so it only needs a signed-in user — Website Users included — to
// keep an anonymous visitor from using the server as a barcode generator.
//
// The value arrives in the URL, and the same value always draws the same
// image, so the browser may keep it; `private` keeps a shared proxy from
// holding what may be a customer's number. A value the symbology refuses is a
// 400 with the same message a save would give.
func (s *Server) barcodeSVG(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	svg, err := barcode.SVG(q.Get("symbology"), q.Get("value"))
	if err != nil {
		c := s.E.NewCtx(r.Context(), user(r))
		c.Lang = s.langFor(r)
		err = barcode.Invalid(c.T("Barcode"), err)
		var ce *cerr.Error
		if errors.As(err, &ce) {
			bad := *ce
			bad.Status = http.StatusBadRequest
			err = &bad
		}
		s.writeErr(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=86400")
	w.Write(svg)
}
