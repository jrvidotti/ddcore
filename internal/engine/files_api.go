package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	neturl "net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// This file is how bytes become a File: the upload handler and server code
// (ddcore.files) go through the same rules — who may attach to what, which
// fields force a file private, which names are safe to serve.

// DefaultMaxUpload is the largest file accepted when nothing sets a limit.
const DefaultMaxUpload = 50 << 20

// RandomFileName never reuses the given name: it is unguessable and only a
// safe extension survives, so a public file cannot be located by name nor
// served as active content.
func RandomFileName(orig string) string {
	ext := strings.ToLower(filepath.Ext(orig))
	for _, r := range ext {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.') {
			ext = ".bin"
			break
		}
	}
	if dangerousExt[ext] || len(ext) > 10 || ext == "." {
		ext = ".bin"
	}
	return RandomToken() + ext
}

// UploadTarget checks that the user may put a file on the document named, and
// returns the id to attach it to.
//
// Attaching is writing: a file on a document is read by everyone who reads the
// document, so naming one needs write on it. A document not saved yet — no
// id, or an id that does not exist yet — needs the right to create one, and
// the file is stored detached; the save that names it attaches it
// (claimAttachments). A Website User must always say where the file goes, and
// it can only go into an attachment field a portal page lets them edit
// (OPS-10).
func (c *Ctx) UploadTarget(doctype, id, field string) (string, error) {
	if doctype == "" {
		if c.PortalMode() {
			return "", cerr.Permission("Not permitted to upload here")
		}
		return "", nil
	}
	d, err := c.St.DocType(doctype)
	if err != nil {
		return "", err
	}
	if c.PortalMode() && !c.PortalUploadAllowed(d.Name, field) {
		return "", cerr.Permission("Not permitted to upload here")
	}
	if id != "" {
		var doc Doc
		err := c.WithIgnorePermissions(func() error {
			var e error
			doc, e = c.GetDoc(d.Name, id)
			return e
		})
		if err == nil {
			ok, err := c.HasPermission(d.Name, "write", doc)
			if err != nil {
				return "", err
			}
			if !ok {
				return "", cerr.Permission("No permission ({0}) on {1} {2}", "write", c.T(d.Label), id)
			}
			return id, nil
		}
		if ce := cerr.From(err); ce == nil || ce.Status != http.StatusNotFound {
			return "", err
		}
	}
	for _, ptype := range []string{"create", "write"} {
		ok, err := c.HasPermission(d.Name, ptype, nil)
		if err != nil {
			return "", err
		}
		if ok {
			return "", nil
		}
	}
	return "", cerr.Permission("Not permitted to upload files to {0}", c.T(d.Label))
}

// NewFile describes bytes about to be stored as a File.
type NewFile struct {
	Doctype, ID, Field string // what the file is attached to; ID from UploadTarget
	Name               string // the name the user knows it by
	ContentType        string
	Size               int64
	Private            bool
}

// StoreFile applies the attachment field's rules, writes r to the store under
// a fresh name and inserts the File row. The row is written on the current
// transaction; if that transaction is rolled back, the bytes are deleted, so
// no row-less bytes are left behind.
func (c *Ctx) StoreFile(f NewFile, r io.Reader) (Doc, error) {
	// a file put into a restricted field is that field's value: only its
	// writers may put one there, and it is never public (SEC-02)
	if restricted, canWrite := c.AttachmentFieldRestricted(f.Doctype, f.Field); restricted {
		if !canWrite {
			return nil, cerr.Permission("Not permitted to change {0}", f.Field)
		}
		f.Private = true
	}
	if wantsImage, label := c.AttachmentFieldWantsImage(f.Doctype, f.Field); wantsImage && !IsImageFileName(f.Name) {
		return nil, cerr.Validation("{0} takes an image file (png, jpg, gif, webp)", label)
	}
	name := RandomFileName(f.Name)
	url := "/files/" + name
	if f.Private {
		url = "/private/files/" + name
	}
	key, _ := storage.KeyFromURL(url)
	store := c.E.Storage()
	if err := store.Put(c.Ctx, key, r, f.Size, f.ContentType); err != nil {
		return nil, err
	}
	drop := func() {
		if err := store.Delete(context.WithoutCancel(c.Ctx), key); err != nil {
			c.E.Log.Warn("stored bytes left behind after a rollback", "file_url", url, "err", err)
		}
	}
	doc, err := c.NewDoc("File", Doc{"file_name": f.Name, "file_url": url, "file_size": f.Size, "content_type": f.ContentType,
		"is_private": f.Private, "attached_to_doctype": f.Doctype, "attached_to_id": f.ID, "attached_to_field": f.Field})
	if err == nil {
		doc, err = c.Insert(doc, SaveOpts{IgnorePermissions: true})
	}
	if err != nil {
		// no row will ever name these bytes: take them back out
		drop()
		return nil, err
	}
	c.AfterRollback(drop)
	return doc, nil
}

// SaveFileArgs is ddcore.files.save's argument. Exactly one of Content,
// ContentBase64 and FromURL is the source of the bytes.
type SaveFileArgs struct {
	Doctype           string            `json:"doctype"`
	ID                string            `json:"id"`
	Fieldname         string            `json:"fieldname"`
	Filename          string            `json:"filename"`
	IsPrivate         *bool             `json:"isPrivate"`
	ContentType       string            `json:"contentType"`
	Content           *string           `json:"content"`
	ContentBase64     *string           `json:"contentBase64"`
	FromURL           string            `json:"fromUrl"`
	Headers           map[string]string `json:"headers"`
	MaxBytes          float64           `json:"maxBytes"`
	Timeout           float64           `json:"timeout"`
	IgnorePermissions bool              `json:"ignorePermissions"`
}

// SaveFile stores bytes server code holds, or downloads, as a File, with the
// rules of an upload: attaching needs write on the document (unless
// IgnorePermissions), a restricted field forces the file private, and the
// stored name is random. A file is private unless IsPrivate is false.
func (c *Ctx) SaveFile(a SaveFileArgs) (Doc, error) {
	if c.User == "Guest" && !a.IgnorePermissions {
		return nil, cerr.Auth("Sign in to upload files")
	}
	sources := 0
	for _, set := range []bool{a.Content != nil, a.ContentBase64 != nil, a.FromURL != ""} {
		if set {
			sources++
		}
	}
	if sources != 1 {
		return nil, cerr.Validation("files.save needs exactly one of content, contentBase64 and fromUrl")
	}
	max := int64(a.MaxBytes)
	if max <= 0 {
		max = DefaultMaxUpload
	}
	name, contentType := a.Filename, a.ContentType
	var b []byte
	switch {
	case a.Content != nil:
		b = []byte(*a.Content)
	case a.ContentBase64 != nil:
		var err error
		if b, err = base64.StdEncoding.DecodeString(*a.ContentBase64); err != nil {
			return nil, cerr.Validation("files.save: contentBase64 is not base64: {0}", err)
		}
	default:
		res, body, err := httpFetch("GET", a.FromURL, nil, "", a.Headers, a.Timeout, max, nil)
		if err != nil {
			return nil, err
		}
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return nil, cerr.Validation("files.save: {0} answered {1}", a.FromURL, res.StatusCode)
		}
		b = body
		if contentType == "" {
			contentType = res.Header.Get("Content-Type")
		}
		if name == "" {
			if u, err := neturl.Parse(a.FromURL); err == nil {
				name = path.Base(u.Path)
			}
		}
	}
	if name == "" || name == "/" || name == "." {
		return nil, cerr.Validation("files.save needs a filename")
	}
	if int64(len(b)) > max {
		return nil, cerr.Validation("files.save: the file is larger than {0} bytes (raise maxBytes)", max)
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	}
	var doc Doc
	run := func() error {
		id, err := c.UploadTarget(a.Doctype, a.ID, a.Fieldname)
		if err != nil {
			return err
		}
		private := a.IsPrivate == nil || *a.IsPrivate || c.PortalMode()
		doc, err = c.StoreFile(NewFile{Doctype: a.Doctype, ID: id, Field: a.Fieldname, Name: name,
			ContentType: contentType, Size: int64(len(b)), Private: private}, bytes.NewReader(b))
		return err
	}
	var err error
	if a.IgnorePermissions {
		err = c.WithIgnorePermissions(run)
	} else {
		err = run()
	}
	return doc, err
}

// MaxPresignTTL is the longest a presigned URL may live: the S3 limit.
const MaxPresignTTL = 7 * 24 * time.Hour

// PresignFile returns a URL that serves the File at fileURL to anyone holding
// it for ttl (the store's default when zero): how a file reaches a third party
// with no session. The user must be able to read the File, as for a download.
func (c *Ctx) PresignFile(fileURL string, ttl time.Duration, ignorePermissions bool) (string, error) {
	key, ok := storage.KeyFromURL(fileURL)
	if !ok {
		return "", cerr.Validation("files.presign: {0} is not a file url", fileURL)
	}
	if ttl <= 0 {
		ttl = c.E.Cfg.Storage.S3.PresignTTL
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if ttl > MaxPresignTTL {
		return "", cerr.Validation("files.presign: ttl is at most {0} seconds", int(MaxPresignTTL.Seconds()))
	}
	var f map[string]any
	err := c.WithIgnorePermissions(func() error {
		var err error
		f, err = c.GetValues("File", map[string]any{"file_url": fileURL}, append([]string{"file_name"}, FilePermFields...))
		return err
	})
	if err != nil {
		return "", err
	}
	if f == nil {
		return "", cerr.NotFound("File {0} not found", fileURL)
	}
	if !ignorePermissions && !c.CanReadFile(f) {
		return "", cerr.Permission("No permission for this file")
	}
	u, err := c.E.Storage().Presign(c.Ctx, key, ttl, storage.Serving{Name: path.Base(key), Inline: true})
	switch {
	case errors.Is(err, storage.ErrPresignUnsupported):
		return "", cerr.Validation("files.presign needs the s3 storage backend (DDCORE_STORAGE=s3); the {0} backend has no presigned URLs", c.E.Storage().Backend())
	case errors.Is(err, storage.ErrNotFound):
		return "", cerr.NotFound("The bytes of {0} are not in the store", fileURL)
	}
	return u, err
}
