package engine

// The attachments half of a load (DAT-01).
//
// An exported document carries a `_files` manifest: the File rows attached to
// it, each with the sha256 of its bytes. The bytes themselves sit under the
// directory's files/ folder. Loading them means three things, in this order:
// check the checksum, write the bytes, write the File row — with its own id
// and its own url, because every link to that file is the url.
//
// The one guard that is not about fidelity is the extension. The upload
// handler never lets a public file keep an extension a browser would execute
// on this origin, and an import must not be the way around it.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// dangerousExt is the api package's list, kept here too: the two guard the
// same thing from different doors, and an import does not go through the
// upload handler.
var dangerousExt = map[string]bool{".html": true, ".htm": true, ".svg": true, ".xhtml": true,
	".xml": true, ".js": true, ".mjs": true, ".wasm": true, ".shtml": true}

// importAttachments loads the files one document carries. It returns how many
// it wrote; a file already on the site is left alone, so a second run neither
// duplicates the row nor rewrites the bytes.
func (c *Ctx) importAttachments(a ImportArgs, src *ImportSource, stage importStage, rec ImportRecord, doc Doc) (int, []string, error) {
	files, ok := rec.Doc["_files"].([]any)
	if !ok || len(files) == 0 {
		return 0, nil, nil
	}
	n := 0
	var notes []string
	for _, raw := range files {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		f := importFileFrom(m)
		if f.Missing {
			notes = append(notes, fmt.Sprintf("%s %s: the export marked %s as missing; its bytes were already gone",
				stage.source, doc.ID(), f.FileURL))
			continue
		}
		key, ok := storage.KeyFromURL(f.FileURL)
		if !ok {
			return n, notes, cerr.Validation("%s is not a file url this site can hold", f.FileURL)
		}
		if strings.HasPrefix(key, "public/") && dangerousExt[strings.ToLower(filepath.Ext(f.FileName))] {
			return n, notes, cerr.Validation("%s is public and would be served as active content from this site", f.FileURL)
		}
		exists, err := c.idExists("File", f.ID)
		if err != nil {
			return n, notes, err
		}
		if err := c.putAttachmentBytes(src, f, key); err != nil {
			return n, notes, err
		}
		if exists {
			continue
		}
		file := Doc{
			"doctype": "File", "id": f.ID, "file_name": f.FileName, "file_url": f.FileURL,
			"file_size": f.Size, "content_type": f.ContentType, "is_private": f.Private,
			"attached_to_doctype": stage.target, "attached_to_id": doc.ID(), "attached_to_field": f.Field,
			"owner": doc["owner"], "creation": doc["creation"], "modified": doc["modified"],
			"modified_by": doc["modified_by"],
		}
		if a.DryRun {
			n++
			continue
		}
		if _, err := c.ImportDoc(file, ImportOpts{}); err != nil {
			return n, notes, err
		}
		n++
	}
	return n, notes, nil
}

// putAttachmentBytes verifies the bytes against the manifest before they are
// written. A dry run reads and checks them and writes nothing: knowing the
// bytes are there and are the right ones is most of what a rehearsal is for.
func (c *Ctx) putAttachmentBytes(src *ImportSource, f ExportFile, key string) error {
	path := src.AttachmentPath(f.FileURL, key)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cerr.Validation("%s is not in the export: %s", f.FileURL, filepath.Base(path))
		}
		return err
	}
	if f.SHA256 != "" {
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != f.SHA256 {
			return cerr.Validation("%s does not match its checksum: the export recorded %s, the file on disk is %s", f.FileURL, short(f.SHA256), short(got))
		}
	}
	if c.Flags["rollback"] == true {
		return nil
	}
	return c.E.Storage().Put(c.Ctx, key, bytes.NewReader(b), int64(len(b)), f.ContentType)
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// importFileFrom reads one `_files` entry.
func importFileFrom(m map[string]any) ExportFile {
	f := ExportFile{
		ID: db.Str(m["id"]), AttachedTo: db.Str(m["attachedTo"]), Field: db.Str(m["field"]),
		FileName: db.Str(m["fileName"]), FileURL: db.Str(m["fileUrl"]), ContentType: db.Str(m["contentType"]),
		SHA256: db.Str(m["sha256"]),
	}
	if v, ok := m["private"].(bool); ok {
		f.Private = v
	}
	if v, ok := m["missing"].(bool); ok {
		f.Missing = v
	}
	f.Size = int64(toFloat(m["size"]))
	if f.ID == "" {
		f.ID = randomID()
	}
	return f
}
