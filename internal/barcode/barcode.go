// Package barcode validates the value of a Barcode field and draws it as SVG.
//
// It is a leaf: the engine calls Normalize on every write, the API and print
// call SVG, and neither needs anything from the other. The encoders come from
// github.com/boombuler/barcode; what this package adds is the rule for what a
// field may store — so a value that saves is always a value that draws — and a
// vector drawing that stays crisp at any print size.
package barcode

import (
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"strconv"
	"strings"

	bc "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/qr"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// The symbologies a field may declare in `options`.
const (
	Code128 = "Code128"
	EAN13   = "EAN-13"
	QR      = "QR"
)

// Default is the symbology of a field that declares none.
const Default = Code128

// Symbologies lists the accepted `options`, in the order the docs give them.
var Symbologies = []string{Code128, EAN13, QR}

// Limits on what a value may hold. Code 128 past 80 characters is wider than
// any label a scanner reads in one pass; a QR code past 1000 bytes has modules
// too small to print on anything short of a full page.
const (
	MaxCode128 = 80
	MaxQRBytes = 1000
)

// Valid reports whether sym names a supported symbology. The empty string is
// the default.
func Valid(sym string) bool {
	switch sym {
	case "", Code128, EAN13, QR:
		return true
	}
	return false
}

// Kinds of Error, one per message the user can get.
const (
	KindSymbology  = "symbology"
	KindTooLong    = "tooLong"
	KindCharacters = "characters"
	KindDigits     = "digits"
	KindCheckDigit = "checkDigit"
	KindEmpty      = "empty"
)

// Error says why a value cannot be a barcode. It carries a Kind rather than a
// sentence so the caller can phrase it with the field's translated label.
type Error struct {
	Kind      string
	Symbology string
	Value     string
}

func (e *Error) Error() string {
	return fmt.Sprintf("barcode %s: %s (%q)", e.Symbology, e.Kind, e.Value)
}

// Normalize returns the value a Barcode field stores: trimmed, and for EAN-13
// completed with its check digit. It is idempotent, so a value read back and
// saved again is unchanged. An empty value stays empty and is not an error.
func Normalize(sym, v string) (string, error) {
	if sym == "" {
		sym = Default
	}
	v = strings.TrimSpace(v)
	fail := func(kind string) (string, error) { return "", &Error{Kind: kind, Symbology: sym, Value: v} }
	if !Valid(sym) {
		return fail(KindSymbology)
	}
	if v == "" {
		return "", nil
	}
	switch sym {
	case Code128:
		if len(v) > MaxCode128 {
			return fail(KindTooLong)
		}
		for i := 0; i < len(v); i++ {
			// printable ASCII only: code set B has no letter outside it, and a
			// control character is invisible in every place the value shows
			if v[i] < 32 || v[i] > 126 {
				return fail(KindCharacters)
			}
		}
	case EAN13:
		for i := 0; i < len(v); i++ {
			if v[i] < '0' || v[i] > '9' {
				return fail(KindDigits)
			}
		}
		switch len(v) {
		case 12:
			v += strconv.Itoa(CheckDigit(v))
		case 13:
			if CheckDigit(v[:12]) != int(v[12]-'0') {
				return fail(KindCheckDigit)
			}
		default:
			return fail(KindDigits)
		}
	case QR:
		if len(v) > MaxQRBytes {
			return fail(KindTooLong)
		}
	}
	return v, nil
}

// CheckDigit computes the EAN-13 check digit of twelve digits: weights 1 and 3
// alternating from the left, and the digit that rounds the sum up to ten.
func CheckDigit(digits12 string) int {
	sum := 0
	for i := 0; i < len(digits12); i++ {
		d := int(digits12[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return (10 - sum%10) % 10
}

// Invalid phrases err, as returned by Normalize, as a validation error naming
// the field. Anything else passes through.
func Invalid(label string, err error) error {
	var e *Error
	if !errors.As(err, &e) {
		return err
	}
	switch e.Kind {
	case KindSymbology:
		return cerr.Validation("Unknown barcode symbology in {0}: \"{1}\"", label, e.Symbology)
	case KindTooLong:
		limit := MaxCode128
		if e.Symbology == QR {
			limit = MaxQRBytes
		}
		return cerr.Validation("{0} is too long for a {1} barcode (at most {2} characters)", label, e.Symbology, strconv.Itoa(limit))
	case KindCharacters:
		return cerr.Validation("{0} takes only letters, digits, spaces and symbols for a {1} barcode", label, e.Symbology)
	case KindDigits:
		return cerr.Validation("{0} takes 12 or 13 digits for an EAN-13 barcode: \"{1}\"", label, e.Value)
	case KindCheckDigit:
		return cerr.Validation("Wrong check digit in {0}: \"{1}\"", label, e.Value)
	case KindEmpty:
		return cerr.Validation("{0} has no value to draw as a barcode", label)
	}
	return err
}

// Drawing geometry, in modules (the narrowest bar is one unit).
const (
	quiet1D   = 10 // Code 128 asks for ten modules each side; EAN-13 for 11 and 7
	quietQR   = 4
	barHeight = 50
	textSize  = 10
	textGap   = 12 // from the bars' bottom edge to the text's baseline
)

// SVG draws a value as a standalone SVG document. The value is normalized
// first, so anything Normalize refuses is refused here with the same error.
// 1D codes carry the human-readable value under the bars.
func SVG(sym, v string) ([]byte, error) {
	if sym == "" {
		sym = Default
	}
	v, err := Normalize(sym, v)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, &Error{Kind: KindEmpty, Symbology: sym}
	}
	var code bc.Barcode
	switch sym {
	case Code128:
		code, err = code128.Encode(v)
	case EAN13:
		code, err = ean.Encode(v)
	case QR:
		code, err = qr.Encode(v, qr.M, qr.Auto)
	}
	if err != nil {
		return nil, fmt.Errorf("barcode %s: %w", sym, err)
	}
	if sym == QR {
		return drawQR(code), nil
	}
	return draw1D(code, v), nil
}

func dark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r+g+b < 3*0x8000
}

// runs appends one rectangle per horizontal run of dark modules on row y of
// the image, offset by (ox, oy) and h modules tall.
func runs(sb *strings.Builder, img image.Image, y, ox, oy, h int) {
	bounds := img.Bounds()
	for x := bounds.Min.X; x < bounds.Max.X; {
		if !dark(img.At(x, y)) {
			x++
			continue
		}
		start := x
		for x < bounds.Max.X && dark(img.At(x, y)) {
			x++
		}
		w := x - start
		fmt.Fprintf(sb, "M%d %dh%dv%dh-%dz", ox+start-bounds.Min.X, oy, w, h, w)
	}
}

func draw1D(code bc.Barcode, text string) []byte {
	bounds := code.Bounds()
	width := bounds.Dx() + 2*quiet1D
	height := barHeight + textGap + 2
	var path strings.Builder
	runs(&path, code, bounds.Min.Y, quiet1D, 0, barHeight)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" class="barcode" viewBox="0 0 %d %d" width="%d" height="%d">`,
		width, height, width*2, height*2)
	fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#fff"/>`, width, height)
	fmt.Fprintf(&sb, `<path shape-rendering="crispEdges" fill="#000" d="%s"/>`, path.String())
	fmt.Fprintf(&sb, `<text x="%d" y="%d" text-anchor="middle" font-family="monospace" font-size="%d" fill="#000">%s</text>`,
		width/2, barHeight+textGap, textSize, html.EscapeString(text))
	sb.WriteString(`</svg>`)
	return []byte(sb.String())
}

func drawQR(code bc.Barcode) []byte {
	bounds := code.Bounds()
	size := bounds.Dx() + 2*quietQR
	var path strings.Builder
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		runs(&path, code, y, quietQR, quietQR+y-bounds.Min.Y, 1)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" class="barcode" viewBox="0 0 %d %d" width="%d" height="%d">`,
		size, size, size*4, size*4)
	fmt.Fprintf(&sb, `<rect width="%d" height="%d" fill="#fff"/>`, size, size)
	fmt.Fprintf(&sb, `<path shape-rendering="crispEdges" fill="#000" d="%s"/>`, path.String())
	sb.WriteString(`</svg>`)
	return []byte(sb.String())
}
