package api

import (
	"fmt"
	"net/http"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/print"
)

// GET /api/print/formats/{doctype}
// Returns a list of available print formats for the given DocType.
func (s *Server) printFormats(w http.ResponseWriter, r *http.Request) {
	doctype := urlParam(r, "doctype")
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		return c.ListPrintFormats(doctype)
	})
}

// GET /api/letterheads
// Returns active letterheads with default status.
func (s *Server) listLetterHeads(w http.ResponseWriter, r *http.Request) {
	s.run(w, r, func(c *engine.Ctx) (any, error) {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT name, is_default, disabled FROM "tab_letter_head" WHERE disabled = false ORDER BY is_default DESC, name ASC`)
		if err != nil {
			return []map[string]any{}, nil
		}
		var result []map[string]any
		for _, row := range rows {
			result = append(result, map[string]any{
				"name":       row["name"],
				"is_default": row["is_default"] == true,
				"disabled":   row["disabled"] == true,
			})
		}
		if result == nil {
			result = []map[string]any{}
		}
		return result, nil
	})
}

// GET /api/print/{doctype}/{name}
// Renders the document to a full HTML document ready for printing or preview.
func (s *Server) printDoc(w http.ResponseWriter, r *http.Request) {
	doctype := urlParam(r, "doctype")
	name := urlParam(r, "name")
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "standard"
	}
	letterhead := q.Get("letterhead")
	lang := q.Get("lang")
	if lang == "" {
		lang = s.langFor(r)
	}

	var htmlOut string
	c := s.E.NewCtx(r.Context(), user(r))
	c.Lang = lang
	err := c.Run(func(c *engine.Ctx) error {
		var err error
		htmlOut, err = c.PrintDoc(doctype, name, format, letterhead, lang)
		return err
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(htmlOut))
}

// GET /api/print/{doctype}/{name}/pdf
// Renders the document to PDF and streams raw PDF bytes.
func (s *Server) printDocPDF(w http.ResponseWriter, r *http.Request) {
	doctype := urlParam(r, "doctype")
	name := urlParam(r, "name")
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "standard"
	}
	letterhead := q.Get("letterhead")
	lang := q.Get("lang")
	if lang == "" {
		lang = s.langFor(r)
	}

	var htmlOut string
	c := s.E.NewCtx(r.Context(), user(r))
	c.Lang = lang
	err := c.Run(func(c *engine.Ctx) error {
		var err error
		htmlOut, err = c.PrintDoc(doctype, name, format, letterhead, lang)
		return err
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}

	pdfOpts := print.PDFOptions{
		Format:    q.Get("page_format"),
		Landscape: q.Get("landscape") == "1" || q.Get("landscape") == "true",
	}
	if pdfOpts.Format == "" {
		pdfOpts.Format = "A4"
	}

	renderer := print.GetDefaultRenderer()
	pdfBytes, err := renderer.RenderPDF(r.Context(), htmlOut, pdfOpts)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}

	disposition := "inline"
	if q.Get("download") == "1" || q.Get("download") == "true" {
		disposition = "attachment"
	}
	filename := fmt.Sprintf("%s-%s.pdf", doctype, name)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	w.WriteHeader(http.StatusOK)
	w.Write(pdfBytes)
}
