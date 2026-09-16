package api

import (
	"fmt"
	"net/http"

	"github.com/jrvidotti/ddcore/internal/cerr"
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
			return nil, err
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

// printParams reads the query parameters both print endpoints share.
type printParams struct {
	format, letterhead, lang string
	page                     print.PDFOptions
}

func (s *Server) printParams(r *http.Request) (printParams, error) {
	q := r.URL.Query()
	p := printParams{format: q.Get("format"), letterhead: q.Get("letterhead"), lang: q.Get("lang")}
	if p.format == "" {
		p.format = "standard"
	}
	if p.lang == "" {
		p.lang = s.langFor(r)
	}
	pageFormat, ok := print.NormalizePageFormat(q.Get("page_format"))
	if !ok {
		return p, cerr.Validation("Page format {0} is not supported: use A4 or Letter", q.Get("page_format"))
	}
	p.page = print.PDFOptions{
		Format:    pageFormat,
		Landscape: q.Get("landscape") == "1" || q.Get("landscape") == "true",
	}
	return p, nil
}

// renderPrint runs PrintDoc for the request's document and parameters.
func (s *Server) renderPrint(r *http.Request, p printParams) (string, error) {
	var htmlOut string
	c := s.E.NewCtx(r.Context(), user(r))
	c.Lang = p.lang
	err := c.Run(func(c *engine.Ctx) error {
		var err error
		htmlOut, err = c.PrintDoc(urlParam(r, "doctype"), urlParam(r, "name"), p.format, p.letterhead, p.lang, p.page)
		return err
	})
	return htmlOut, err
}

// GET /api/print/{doctype}/{name}
// Renders the document to a full HTML document ready for printing or preview.
func (s *Server) printDoc(w http.ResponseWriter, r *http.Request) {
	p, err := s.printParams(r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	htmlOut, err := s.renderPrint(r, p)
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
	p, err := s.printParams(r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	htmlOut, err := s.renderPrint(r, p)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}

	renderer := print.GetDefaultRenderer()
	pdfBytes, err := renderer.RenderPDF(r.Context(), htmlOut, p.page)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}

	disposition := "inline"
	if q := r.URL.Query(); q.Get("download") == "1" || q.Get("download") == "true" {
		disposition = "attachment"
	}
	filename := fmt.Sprintf("%s-%s.pdf", doctype, name)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	w.WriteHeader(http.StatusOK)
	w.Write(pdfBytes)
}
