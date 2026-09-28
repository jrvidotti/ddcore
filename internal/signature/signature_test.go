package signature

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// pngURL draws a w×h image with one dark pixel and returns it as a data URL.
func pngURL(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return Prefix + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestNormalize(t *testing.T) {
	valid := pngURL(t, 300, 100)
	// a PNG header claiming huge dimensions, with nothing after it: the
	// bytes are tiny, the image would not be
	var huge bytes.Buffer
	png.Encode(&huge, image.NewGray(image.Rect(0, 0, 2001, 10)))
	jpeg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01")
	// random bytes do not compress, so the PNG stays past the limit
	noisy := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	seed := uint32(1)
	for i := range noisy.Pix {
		seed = seed*1664525 + 1013904223
		noisy.Pix[i] = byte(seed >> 24)
	}
	var big bytes.Buffer
	png.Encode(&big, noisy)
	if big.Len() <= MaxBytes {
		t.Fatalf("the oversize fixture is only %d bytes", big.Len())
	}

	cases := []struct {
		name, in, want, kind string
	}{
		{"empty", "", "", ""},
		{"blank", "  \n", "", ""},
		{"valid", valid, valid, ""},
		{"valid, spaced", "  " + valid + "\n", valid, ""},
		{"largest", pngURL(t, MaxWidth, MaxHeight), "", ""},
		{"no prefix", strings.TrimPrefix(valid, Prefix), "", KindFormat},
		{"jpeg prefix", strings.Replace(valid, "image/png", "image/jpeg", 1), "", KindFormat},
		{"uppercase prefix", strings.Replace(valid, "data:", "DATA:", 1), "", KindFormat},
		{"javascript", "javascript:alert(1)", "", KindFormat},
		{"svg", "data:image/svg+xml;base64,PHN2Zz4=", "", KindFormat},
		{"bad base64", Prefix + "not base64!", "", KindEncoding},
		{"url-safe base64", Prefix + "_-_-", "", KindEncoding},
		{"jpeg bytes", Prefix + base64.StdEncoding.EncodeToString(jpeg), "", KindNotPNG},
		{"truncated png", Prefix + base64.StdEncoding.EncodeToString(pngMagic), "", KindNotPNG},
		{"too wide", Prefix + base64.StdEncoding.EncodeToString(huge.Bytes()), "", KindDimensions},
		{"too tall", pngURL(t, 10, MaxHeight+1), "", KindDimensions},
		{"too many bytes", Prefix + base64.StdEncoding.EncodeToString(big.Bytes()), "", KindTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.in)
			var e *Error
			switch {
			case tc.kind == "" && err != nil:
				t.Fatalf("Normalize: %v", err)
			case tc.kind != "" && (!errors.As(err, &e) || e.Kind != tc.kind):
				t.Fatalf("Normalize = %.40q, %v; want a %s error", got, err, tc.kind)
			case tc.kind == "" && tc.want != "" && got != tc.want:
				t.Fatalf("Normalize = %.40q, want %.40q", got, tc.want)
			}
			if err == nil {
				// saving what was read back must change nothing
				if again, err := Normalize(got); err != nil || again != got {
					t.Fatalf("Normalize is not idempotent: %v", err)
				}
			}
		})
	}
	if !Valid(valid) || Valid("") || Valid("javascript:x") {
		t.Fatal("Valid disagrees with Normalize")
	}
}

func TestInvalidNamesTheField(t *testing.T) {
	for _, in := range []string{"x", Prefix + "!!", pngURL(t, 10, MaxHeight+1)} {
		_, err := Normalize(in)
		var ce *cerr.Error
		if !errors.As(Invalid("Customer signature", err), &ce) || ce.Type != "ValidationError" {
			t.Fatalf("not a validation error: %v", Invalid("Customer signature", err))
		}
		if !strings.Contains(ce.Message, "Customer signature") {
			t.Fatalf("the message does not name the field: %s", ce.Message)
		}
	}
	plain := errors.New("other")
	if Invalid("X", plain) != plain {
		t.Fatal("an unrelated error was rewritten")
	}
}
