// Package signature validates the value of a Signature field: a hand-drawn
// signature stored as a PNG data URL, the way Frappe stores it.
//
// It is a leaf: the engine calls Normalize on every write and print calls it
// again before putting the value in an <img>, so a stored value can only ever
// be a small PNG — never markup, a script URL or another image format.
package signature

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// Prefix is what every stored value starts with.
const Prefix = "data:image/png;base64,"

// Limits on a stored signature. A pad trimmed to its strokes is a few KiB;
// 64 KiB leaves room for a large, detailed one while keeping a document's
// row, its API payload and its print small. The dimensions bound what a
// decoder would allocate for an image that compresses well.
const (
	MaxBytes  = 64 << 10
	MaxWidth  = 2000
	MaxHeight = 1000
)

// pngMagic is the eight-byte signature every PNG file starts with.
var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// Kinds of Error, one per message the user can get.
const (
	KindFormat     = "format"     // not a PNG data URL
	KindEncoding   = "encoding"   // the base64 does not decode
	KindNotPNG     = "notPNG"     // the bytes are not a PNG
	KindTooLarge   = "tooLarge"   // more than MaxBytes decoded
	KindDimensions = "dimensions" // wider or taller than allowed
)

// Error says why a value cannot be a signature. It carries a Kind rather than
// a sentence so the caller can phrase it with the field's translated label.
type Error struct {
	Kind string
}

func (e *Error) Error() string { return "signature: " + e.Kind }

// Normalize returns the value a Signature field stores. Outer spaces are
// trimmed; an empty value stays empty and is not an error; anything else must
// be a PNG data URL within the limits, and is returned unchanged, so a value
// read back and saved again is the same.
func Normalize(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	fail := func(kind string) (string, error) { return "", &Error{Kind: kind} }
	if !strings.HasPrefix(s, Prefix) {
		return fail(KindFormat)
	}
	enc := s[len(Prefix):]
	// refuse before decoding what could never fit
	if base64.StdEncoding.DecodedLen(len(enc)) > MaxBytes+2 {
		return fail(KindTooLarge)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return fail(KindEncoding)
	}
	if len(raw) > MaxBytes {
		return fail(KindTooLarge)
	}
	if !bytes.HasPrefix(raw, pngMagic) {
		return fail(KindNotPNG)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return fail(KindNotPNG)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxWidth || cfg.Height > MaxHeight {
		return fail(KindDimensions)
	}
	return s, nil
}

// Valid reports whether s is a non-empty value Normalize accepts.
func Valid(s string) bool {
	v, err := Normalize(s)
	return err == nil && v != ""
}

// Invalid phrases err, as returned by Normalize, as a validation error naming
// the field. Anything else passes through.
func Invalid(label string, err error) error {
	var e *Error
	if !errors.As(err, &e) {
		return err
	}
	switch e.Kind {
	case KindFormat, KindEncoding, KindNotPNG:
		return cerr.Validation("{0} must be a signature drawn on the form (a PNG image)", label)
	case KindTooLarge:
		return cerr.Validation("The signature in {0} is too large (at most {1} KiB)", label, strconv.Itoa(MaxBytes>>10))
	case KindDimensions:
		return cerr.Validation("The signature in {0} is too large (at most {1} by {2} pixels)", label,
			strconv.Itoa(MaxWidth), strconv.Itoa(MaxHeight))
	}
	return err
}
