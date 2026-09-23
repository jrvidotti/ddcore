package api

// POST /api/data-import/<DocType> — rows from a CSV or XLSX upload, written
// as the caller; GET …/template — the header row of a file that imports new
// records. See docs/agent/data-import.md.
//
// The upload is not stored: the desk posts the same file twice, once as a
// dry run and once for real, and the server keeps nothing in between. That is
// what lets a dry run and its import be two independent requests with no
// cleanup of an abandoned one.

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
)

// DefaultImportMaxRows is the cap when ddcore.json does not set one. Every row
// is written while the request waits, so the cap is what keeps an upload
// inside a reverse proxy's timeout.
const DefaultImportMaxRows = 5000

// maxImportBytes caps the uploaded file.
const maxImportBytes = 10 << 20

func (s *Server) importMaxRows() int {
	if n := s.E.Cfg.ImportMaxRows; n > 0 {
		return n
	}
	return DefaultImportMaxRows
}

func (s *Server) requestCtx(r *http.Request) *engine.Ctx {
	c := s.E.NewCtx(r.Context(), user(r))
	c.Request = map[string]any{"method": r.Method, "path": r.URL.Path, "ip": r.RemoteAddr}
	if ck, err := r.Cookie("sid"); err == nil {
		c.Sid = ck.Value
	}
	c.Lang = s.langFor(r)
	return c
}

func (s *Server) dataImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+1<<20)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		s.writeErr(w, r, cerr.Validation("The file is larger than {0} MB", maxImportBytes>>20))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		s.writeErr(w, r, cerr.Validation("Missing file field"))
		return
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, maxImportBytes+1))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	if len(body) > maxImportBytes {
		s.writeErr(w, r, cerr.Validation("The file is larger than {0} MB", maxImportBytes>>20))
		return
	}
	args := engine.DataImportArgs{
		Doctype:   urlParam(r, "doctype"),
		Mode:      r.FormValue("mode"),
		FileName:  hdr.Filename,
		File:      body,
		DryRun:    r.FormValue("dry_run") == "1" || r.FormValue("dry_run") == "true",
		Decimal:   r.FormValue("decimal"),
		DateOrder: r.FormValue("date_order"),
		MaxRows:   s.importMaxRows(),
	}
	if v := r.FormValue("sep"); v != "" {
		sep, err := csvSep(v)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		args.Sep = sep
	}
	if v := r.FormValue("columns"); v != "" {
		if err := json.Unmarshal([]byte(v), &args.Columns); err != nil {
			s.writeErr(w, r, cerr.Validation("Invalid column mapping: {0}", err.Error()))
			return
		}
	}
	res, err := s.requestCtx(r).DataImport(args)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, 200, response{Data: res})
}

func (s *Server) dataImportTemplate(w http.ResponseWriter, r *http.Request) {
	doctype := urlParam(r, "doctype")
	sep := ','
	if v := r.URL.Query().Get("sep"); v != "" {
		var err error
		if sep, err = csvSep(v); err != nil {
			s.writeErr(w, r, err)
			return
		}
	}
	var header []string
	err := s.requestCtx(r).Run(func(c *engine.Ctx) error {
		var err error
		header, err = c.DataImportTemplate(doctype)
		return err
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.ReplaceAll(doctype, " ", "-")+"-import-template.csv"))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	bw := bufio.NewWriter(w)
	// the BOM, or Excel reads "Endereço" as "EndereÃ§o" — as in an export
	bw.WriteString("\xef\xbb\xbf")
	cw := csv.NewWriter(bw)
	cw.UseCRLF = true
	cw.Comma = sep
	cw.Write(header)
	cw.Flush()
	bw.Flush()
}

func csvSep(v string) (rune, error) {
	rs := []rune(v)
	if len(rs) != 1 || rs[0] == '"' || rs[0] == '\n' || rs[0] == '\r' {
		return 0, cerr.Validation("Invalid CSV separator: {0}", v)
	}
	return rs[0], nil
}
