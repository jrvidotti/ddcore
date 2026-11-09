// Package thumb makes the small variants of public images that lists and cards
// draw (#154): fitted inside a square, never enlarged, turned the way the
// camera said, and encoded as JPEG unless the image has transparency.
package thumb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif" // a GIF's first frame
	"image/jpeg"
	"image/png"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// MaxPixels is the largest image decoded: past it the pixels alone would take
// hundreds of megabytes. A variable so a test can lower it.
var MaxPixels = 40_000_000

// ErrTooLarge is an image with more than MaxPixels pixels.
var ErrTooLarge = errors.New("thumb: image too large to resize")

// jpegQuality is enough for a 64–512 px picture of a product.
const jpegQuality = 80

// Make returns src fitted inside w×w with its aspect ratio kept, never larger
// than it is, and the extension of the format it chose: ".jpg" for an opaque
// image, ".png" for one with transparency. ext is the source's extension; it
// only decides whether the EXIF orientation is read.
func Make(src []byte, ext string, w int) ([]byte, string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, "", err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, "", errors.New("thumb: empty image")
	}
	if int64(cfg.Width)*int64(cfg.Height) > int64(MaxPixels) {
		return nil, "", ErrTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", err
	}
	orient := 1
	if e := strings.ToLower(ext); e == ".jpg" || e == ".jpeg" {
		orient = orientation(src)
	}
	sb := img.Bounds()
	// the box is fitted on the image as it will be seen; a quarter turn swaps
	// the sides it is scaled to before it is turned
	ow, oh := sb.Dx(), sb.Dy()
	turned := orient >= 5 && orient <= 8
	if turned {
		ow, oh = oh, ow
	}
	tw, th := fit(ow, oh, w)
	if turned {
		tw, th = th, tw
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, sb, draw.Src, nil)
	out := reorient(dst, orient)

	var buf bytes.Buffer
	if opaque(img) {
		err = jpeg.Encode(&buf, out, &jpeg.Options{Quality: jpegQuality})
		return buf.Bytes(), ".jpg", err
	}
	err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out)
	return buf.Bytes(), ".png", err
}

// fit is the size of a w×h image inside an n×n square, never enlarged.
func fit(w, h, n int) (int, int) {
	if w <= n && h <= n {
		return w, h
	}
	if w >= h {
		return n, max(1, (h*n+w/2)/w)
	}
	return max(1, (w*n+h/2)/h), n
}

// opaque is decided on the source: a resized edge may come out a hair short
// of full alpha, which would turn a photo into a PNG for nothing.
func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}

// reorient applies an EXIF orientation (1–8). The browser turns the original
// by it; the thumbnail carries no EXIF, so it is turned here.
func reorient(src *image.RGBA, orient int) image.Image {
	if orient < 2 || orient > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if orient >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch orient {
			case 2: // mirrored
				sx, sy = w-1-x, y
			case 3: // upside down
				sx, sy = w-1-x, h-1-y
			case 4: // mirrored upside down
				sx, sy = x, h-1-y
			case 5: // transposed
				sx, sy = y, x
			case 6: // a quarter turn clockwise
				sx, sy = y, h-1-x
			case 7: // transversed
				sx, sy = w-1-y, h-1-x
			case 8: // a quarter turn counter-clockwise
				sx, sy = w-1-y, x
			}
			i, j := dst.PixOffset(x, y), src.PixOffset(sx, sy)
			copy(dst.Pix[i:i+4], src.Pix[j:j+4])
		}
	}
	return dst
}

// orientation reads tag 0x0112 from a JPEG's EXIF segment; 1 (as stored) when
// there is none or it cannot be read.
func orientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xD8 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 || marker == 0xFF {
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // the image data starts: no EXIF before it
			return 1
		}
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+n]
		if marker == 0xE1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + n
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(t[2:]) != 42 {
		return 1
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 1
	}
	count := int(bo.Uint16(t[ifd:]))
	for k := 0; k < count; k++ {
		e := ifd + 2 + 12*k
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 { // SHORT
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}
