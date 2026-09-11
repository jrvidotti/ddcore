package api

// GET /api/export/<DocType> — the whole filtered set as a download.
//
// This handler deliberately does not go through Server.run: run buffers the
// result and writes a JSON envelope, which is exactly what an export must not
// do. Everything that can fail — the doctype, the filters, the permission, the
// size — is settled before the first byte, because after a 200 has gone out
// there is no status code left to change. What is left is the count header and
// the NDJSON manifest line, so a consumer can prove the file is complete.

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// DefaultExportMaxRows is the cap when ddcore.json does not set one. Beyond it
// the answer is `ddcore export`, which writes to disk and holds no socket.
const DefaultExportMaxRows = 100000

func (s *Server) exportMaxRows() int {
	if n := s.E.Cfg.ExportMaxRows; n > 0 {
		return n
	}
	return DefaultExportMaxRows
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	doctype := chi.URLParam(r, "doctype")
	q := r.URL.Query()

	format := q.Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "ndjson" {
		s.writeErr(w, r, cerr.Validation("Unknown export format: {0}", format))
		return
	}
	children := q.Get("children") != "" && q.Get("children") != "0"
	if children && format == "csv" {
		// One CSV cannot hold a parent and its child tables without either
		// flattening them away or repeating the parent per row. Refusing is
		// honest; silently dropping the children would not be.
		s.writeErr(w, r, cerr.Validation("CSV cannot carry child tables: use format=ndjson, or `ddcore export`, which writes one file per table."))
		return
	}
	sep := ','
	if v := q.Get("sep"); v != "" {
		rs := []rune(v)
		if len(rs) != 1 || rs[0] == '"' || rs[0] == '\n' || rs[0] == '\r' {
			s.writeErr(w, r, cerr.Validation("Invalid CSV separator: {0}", v))
			return
		}
		sep = rs[0]
	}

	filters, err := queryJSON(r, "filters")
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	orFilters, err := queryJSON(r, "or_filters")
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	var fields []string
	if fv, err := queryJSON(r, "fields"); err != nil {
		s.writeErr(w, r, err)
		return
	} else if list, ok := fv.([]any); ok {
		for _, f := range list {
			fields = append(fields, fmt.Sprint(f))
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))

	args := engine.ExportArgs{
		Doctype:     doctype,
		Filters:     filters,
		OrFilters:   orFilters,
		Fields:      fields,
		Children:    children,
		Attachments: q.Get("attachments") != "" && q.Get("attachments") != "0",
		Limit:       limit,
	}

	c := s.E.NewCtx(r.Context(), user(r))
	c.Request = map[string]any{"method": r.Method, "path": r.URL.Path, "ip": r.RemoteAddr}
	c.Lang = s.langFor(r)

	// started flips once the response has begun; after that an error can only
	// be logged and the stream cut, never turned into a status code.
	started := false
	err = c.Run(func(c *engine.Ctx) error {
		// First statement of the transaction, or Postgres refuses it: every
		// page of the walk has to see one instant of the database.
		if err := c.SnapshotIsolation(); err != nil {
			return err
		}
		if err := s.referenceGuard(c, doctype, filters); err != nil {
			return err
		}
		// Every refusal has to happen here, while the status code is still
		// ours to choose: once the headers go out, a denied export would look
		// like an empty but successful download.
		if err := c.CanExport(args); err != nil {
			return err
		}
		count, err := s.exportCount(c, args)
		if err != nil {
			return err
		}
		// count is already capped at ?limit, so a caller asking for a sample
		// passes while one asking for more than the endpoint streams does not.
		// Checking the raw total instead would refuse a legitimate `limit=10`
		// over a big table; skipping the check whenever a limit was given
		// would let `limit=99999999` walk straight past the cap.
		if max := s.exportMaxRows(); count > int64(max) {
			return cerr.Validation(
				"This export has {0} rows and the limit here is {1}. Narrow the filters, or run `ddcore export` for the whole set.",
				count, max)
		}

		bw := bufio.NewWriterSize(w, 32<<10)
		sink, filename := exportSink(c, doctype, format, sep, bw)
		s.exportHeaders(w, filename, format, count)
		started = true

		flusher, _ := w.(http.Flusher)
		if _, err := c.Export(args, &flushingSink{ExportSink: sink, bw: bw, f: flusher}); err != nil {
			return err
		}
		return bw.Flush()
	})
	if err != nil {
		if !started {
			s.writeErr(w, r, err)
			return
		}
		// The body is already on the wire: the count header is what tells the
		// consumer the file is short, and the log is what tells us why.
		s.E.LogError(r.Context(), "api.export", err)
	}
}

// exportCount is the row count the header advertises. It runs the same filters
// through the same permission path as the walk, so the two cannot disagree.
func (s *Server) exportCount(c *engine.Ctx, a engine.ExportArgs) (int64, error) {
	n, err := c.Count(a.Doctype, a.Filters, a.OrFilters)
	if err != nil {
		return 0, err
	}
	if a.Limit > 0 && n > int64(a.Limit) {
		n = int64(a.Limit)
	}
	return n, nil
}

func exportSink(c *engine.Ctx, doctype, format string, sep rune, w io.Writer) (engine.ExportSink, string) {
	stamp := time.Now().Format("20060102-150405")
	base := strings.ReplaceAll(doctype, " ", "-")
	if format == "ndjson" {
		return engine.NewNDJSONSink(w), base + "-" + stamp + ".ndjson"
	}
	return &engine.CSVSink{
		Open:   func(string) (io.Writer, error) { return w, nil },
		Sep:    sep,
		Lookup: func(n string) (*meta.DocType, error) { return c.St.DocType(n) },
	}, base + "-" + stamp + ".csv"
}

func (s *Server) exportHeaders(w http.ResponseWriter, filename, format string, count int64) {
	ct := "text/csv; charset=utf-8"
	if format == "ndjson" {
		ct = "application/x-ndjson; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// How many rows the file should hold. A truncated download is otherwise
	// indistinguishable from a complete one once the 200 has been sent.
	w.Header().Set("X-DDCore-Export-Count", strconv.FormatInt(count, 10))
	w.WriteHeader(http.StatusOK)
}

// flushingSink pushes each document out as it is produced, so a slow export
// starts arriving immediately and memory stays flat whatever its size.
type flushingSink struct {
	engine.ExportSink
	bw *bufio.Writer
	f  http.Flusher
	n  int
}

func (s *flushingSink) Doc(doc engine.Doc, files []engine.ExportFile) error {
	if err := s.ExportSink.Doc(doc, files); err != nil {
		return err
	}
	s.n++
	if s.n%200 == 0 {
		if err := s.bw.Flush(); err != nil {
			return err
		}
		if s.f != nil {
			s.f.Flush()
		}
	}
	return nil
}
