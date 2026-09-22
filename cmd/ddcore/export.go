package main

// `ddcore export` — the extraction side of a migration.
//
// The HTTP endpoint answers "give me this list as a file". This answers the
// other question: take the whole site, or one DocType of it, to disk, with the
// child tables, the attachment bytes and a manifest of checksums — so a second
// run can be compared with the first, and so what was loaded downstream can be
// reconciled against what was actually taken.
//
// It runs without the HTTP cap: nothing here holds a socket, and the reason
// the cap exists does not apply.

import (
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"crypto/sha256"
	"encoding/hex"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/storage"
)

func cmdExport(args []string) error {
	fs := newFlagSet("export")
	out := fs.String("out", "", "output directory (default export/<timestamp>)")
	format := fs.String("format", "ndjson", "ndjson or csv")
	filters := fs.String("filters", "", "filters as JSON, e.g. '[[\"status\",\"=\",\"Open\"]]'")
	fieldList := fs.String("fields", "", "comma-separated columns (default: all exportable ones)")
	children := fs.Bool("children", false, "include the child tables")
	attachments := fs.Bool("attachments", false, "include the attachments and copy their bytes")
	all := fs.Bool("all", false, "every DocType of the site")
	user := fs.String("user", "Admin", "export as this user, applying his permissions")
	sep := fs.String("sep", ",", "CSV column separator")
	batch := fs.Int("batch", 0, "documents per page of the walk")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if (len(rest) == 0) == !*all {
		return fmt.Errorf("usage: ddcore export <DocType> [--children] [--attachments] [--out DIR], or ddcore export --all")
	}
	if *format != "ndjson" && *format != "csv" {
		return fmt.Errorf("unknown format: %s (use ndjson or csv)", *format)
	}
	sepRunes := []rune(*sep)
	if len(sepRunes) != 1 {
		return fmt.Errorf("--sep takes a single character")
	}
	var parsedFilters any
	if *filters != "" {
		if err := json.Unmarshal([]byte(*filters), &parsedFilters); err != nil {
			return fmt.Errorf("--filters is not valid JSON: %w", err)
		}
	}
	var fields []string
	if *fieldList != "" {
		for _, f := range strings.Split(*fieldList, ",") {
			if f = strings.TrimSpace(f); f != "" {
				fields = append(fields, f)
			}
		}
	}

	e, _, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()

	dir := *out
	if dir == "" {
		dir = filepath.Join("export", time.Now().Format("20060102-150405"))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	doctypes := rest
	if *all {
		// Child tables travel inside their parent; a Single has no table.
		for _, n := range e.Meta.Names() {
			if d := e.Meta.DocTypes[n]; !d.IsChild && !d.IsSingle {
				doctypes = append(doctypes, n)
			}
		}
	}

	run := &exportRun{
		DDCore: engine.Version, Layout: exportLayout, Site: e.SiteTitle(), User: *user, Started: time.Now(), Dir: dir,
		Format: *format, Filters: parsedFilters, Apps: map[string]string{},
	}
	for _, n := range e.AppOrder() {
		if am := e.Snap.Apps[n]; am != nil && am.Version != "" {
			run.Apps[n] = am.Version
		}
	}
	ctx := context.Background()
	for _, dt := range doctypes {
		res, err := exportOne(ctx, e, *user, dir, dt, *format, sepRunes[0], engine.ExportArgs{
			Doctype: dt, Filters: parsedFilters, Fields: fields,
			Children: *children, Attachments: *attachments, Batch: *batch,
		})
		if err != nil {
			// With --all over a whole site, one DocType the user may not
			// export is a line in the report, not the end of the run. Asked
			// for by name, it is an error.
			if *all && cerr.From(err).Type == "PermissionError" {
				fmt.Fprintf(os.Stderr, "skipping %s: %v\n", dt, err)
				run.Skipped = append(run.Skipped, dt)
				continue
			}
			return fmt.Errorf("%s: %w", dt, err)
		}
		run.Exports = append(run.Exports, res)
		fmt.Printf("%-28s %7d rows", dt, res.Summary.Rows)
		if n := totalChildRows(res.Summary); n > 0 {
			fmt.Printf("  %5d child rows", n)
		}
		if res.Summary.Files > 0 {
			fmt.Printf("  %4d files", res.Summary.Files)
		}
		fmt.Println()
	}
	run.Finished = time.Now()

	manifest := filepath.Join(dir, "manifest.json")
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifest, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("\n%s\n", manifest)
	return nil
}

// exportLayout is the current arrangement of an export directory; see
// exportRun.Layout.
const exportLayout = 2

// exportRun is the manifest: what was asked, what came out, and the checksum of
// every file written, so a second run can be compared with this one.
type exportRun struct {
	DDCore string `json:"ddcore"`
	// Layout is how this directory is arranged. 1 wrote the attachment bytes
	// to files/<base name>; 2 writes them to files/<storage key>. An importer
	// reads it to know which one it is looking at.
	Layout   int               `json:"exportFormat"`
	Apps     map[string]string `json:"apps,omitempty"` // app name → declared version
	Site     string            `json:"site"`
	User     string            `json:"user"`
	Dir      string            `json:"dir"`
	Format   string            `json:"format"`
	Filters  any               `json:"filters,omitempty"`
	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"`
	Skipped  []string          `json:"skipped,omitempty"`
	Exports  []*exportResult   `json:"exports"`
}

type exportResult struct {
	Summary     *engine.ExportSummary `json:"summary"`
	Outputs     []exportOutput        `json:"outputs"`
	Attachments []engine.ExportFile   `json:"attachments,omitempty"`
}

type exportOutput struct {
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func totalChildRows(s *engine.ExportSummary) int64 {
	var n int64
	for _, v := range s.ChildRows {
		n += v
	}
	return n
}

// hashedFile counts and checksums what it writes, so the manifest describes
// the file that was actually produced rather than what was meant to be.
type hashedFile struct {
	name string
	f    *os.File
	h    hash.Hash
	n    int64
}

func (w *hashedFile) Write(p []byte) (int, error) {
	n, err := w.f.Write(p)
	w.n += int64(n)
	w.h.Write(p[:n])
	return n, err
}

func exportOne(ctx context.Context, e *engine.Engine, user, dir, dt, format string, sep rune, args engine.ExportArgs) (*exportResult, error) {
	res := &exportResult{}
	var files []*hashedFile
	defer func() {
		for _, f := range files {
			f.f.Close()
		}
	}()

	open := func(table string) (io.Writer, error) {
		ext := ".ndjson"
		if format == "csv" {
			ext = ".csv"
		}
		name := strings.ReplaceAll(table, " ", "-") + ext
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		hf := &hashedFile{name: name, f: f, h: sha256.New()}
		files = append(files, hf)
		return hf, nil
	}

	err := e.Run(ctx, user, func(c *engine.Ctx) error {
		// First statement of the transaction: every page of the walk has to
		// see the same instant of the database.
		if err := c.SnapshotIsolation(); err != nil {
			return err
		}
		if err := c.CanExport(args); err != nil {
			return err
		}
		var sink engine.ExportSink
		if format == "ndjson" {
			w, err := open(dt)
			if err != nil {
				return err
			}
			sink = engine.NewNDJSONSink(w)
		} else {
			sink = &engine.CSVSink{
				Open:   open,
				Sep:    sep,
				Lookup: func(n string) (*meta.DocType, error) { return c.St.DocType(n) },
			}
		}
		cs := &copyingSink{ExportSink: sink, c: c, dir: filepath.Join(dir, "files"), res: res}
		sum, err := c.Export(args, cs)
		if err != nil {
			return err
		}
		res.Summary = sum
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if err := f.f.Close(); err != nil {
			return nil, err
		}
		res.Outputs = append(res.Outputs, exportOutput{File: f.name, Bytes: f.n, SHA256: hex.EncodeToString(f.h.Sum(nil))})
	}
	files = nil
	return res, nil
}

// copyingSink is what makes the CLI export self-contained: it copies each
// attachment's bytes next to the data. A file attached to several documents is
// copied once; one whose bytes are gone is recorded as missing rather than
// silently skipped, because that is a finding for the reconciliation.
type copyingSink struct {
	engine.ExportSink
	c    *engine.Ctx
	dir  string
	res  *exportResult
	seen map[string]bool
}

func (s *copyingSink) Doc(doc engine.Doc, files []engine.ExportFile) error {
	if err := s.ExportSink.Doc(doc, files); err != nil {
		return err
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	for _, f := range files {
		s.res.Attachments = append(s.res.Attachments, f)
		if f.Missing || s.seen[f.FileURL] {
			continue
		}
		s.seen[f.FileURL] = true
		if err := copyAttachment(s.c, f.FileURL, filepath.Join(s.dir, attachmentPath(f.FileURL))); err != nil {
			return err
		}
	}
	return nil
}

// attachmentPath is where an attachment's bytes go under <out>/files. It is the
// file's storage key ("public/x", "private/x"), not its base name: two files
// may share a base name across the two prefixes, and the importer puts the
// bytes back by the same key.
func attachmentPath(fileURL string) string {
	if key, ok := storage.KeyFromURL(fileURL); ok {
		return filepath.FromSlash(key)
	}
	return filepath.Base(fileURL)
}

func copyAttachment(c *engine.Ctx, fileURL, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := c.OpenAttachment(fileURL)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
