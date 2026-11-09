package thumb

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func filled(w, h int, c color.Color) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// decoded checks the format Make chose and the size it came out at.
func decoded(t *testing.T, b []byte, wantFormat string, w, h int) image.Image {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if format != wantFormat || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("got %s %dx%d, want %s %dx%d", format, img.Bounds().Dx(), img.Bounds().Dy(), wantFormat, w, h)
	}
	return img
}

func TestOpaquePNGBecomesAJPEG(t *testing.T) {
	out, ext, err := Make(encodePNG(t, filled(1000, 500, color.NRGBA{200, 10, 10, 255})), ".png", 128)
	if err != nil || ext != ".jpg" {
		t.Fatalf("%q %v", ext, err)
	}
	decoded(t, out, "jpeg", 128, 64)
}

func TestTransparencyStaysPNG(t *testing.T) {
	out, ext, err := Make(encodePNG(t, filled(300, 600, color.NRGBA{0, 0, 255, 100})), ".png", 64)
	if err != nil || ext != ".png" {
		t.Fatalf("%q %v", ext, err)
	}
	decoded(t, out, "png", 32, 64)
}

func TestNeverEnlarged(t *testing.T) {
	out, _, err := Make(encodePNG(t, filled(50, 40, color.White)), ".png", 512)
	if err != nil {
		t.Fatal(err)
	}
	decoded(t, out, "jpeg", 50, 40)
}

func TestGIFAndWebP(t *testing.T) {
	pal := image.NewPaletted(image.Rect(0, 0, 400, 200), color.Palette{color.Black, color.White})
	var g bytes.Buffer
	if err := gif.Encode(&g, pal, nil); err != nil {
		t.Fatal(err)
	}
	out, _, err := Make(g.Bytes(), ".gif", 64)
	if err != nil {
		t.Fatal(err)
	}
	decoded(t, out, "jpeg", 64, 32)

	webp, err := os.ReadFile("testdata/blue-purple-pink.lossy.webp")
	if err != nil {
		t.Fatal(err)
	}
	out, ext, err := Make(webp, ".webp", 64)
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil || ext != ".jpg" || format != "jpeg" || max(img.Bounds().Dx(), img.Bounds().Dy()) != 64 {
		t.Fatalf("webp: %s %v %v", ext, img.Bounds(), err)
	}
}

// withOrientation puts an EXIF segment with tag 0x0112 right after the SOI.
func withOrientation(jpg []byte, o byte) []byte {
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, // header, IFD0 at 8
		0, 1, // one entry
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, o, 0, 0, // Orientation SHORT = o
		0, 0, 0, 0} // no next IFD
	seg := append([]byte("Exif\x00\x00"), tiff...)
	n := len(seg) + 2
	app1 := append([]byte{0xFF, 0xE1, byte(n >> 8), byte(n)}, seg...)
	return append(append(append([]byte{}, jpg[:2]...), app1...), jpg[2:]...)
}

func TestEXIFOrientationIsBakedIn(t *testing.T) {
	// left half red, right half blue
	src := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 100 {
				c = color.RGBA{0, 0, 255, 255}
			}
			src.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	jpg := withOrientation(b.Bytes(), 6)
	if o := orientation(jpg); o != 6 {
		t.Fatalf("orientation = %d", o)
	}
	out, _, err := Make(jpg, ".jpg", 64)
	if err != nil {
		t.Fatal(err)
	}
	// a quarter turn clockwise: tall, the left (red) half on top
	img := decoded(t, out, "jpeg", 32, 64)
	top, bottom := img.At(16, 8), img.At(16, 56)
	if r, _, bl, _ := top.RGBA(); r < 0xC000 || bl > 0x4000 {
		t.Errorf("top is not red: %v", top)
	}
	if r, _, bl, _ := bottom.RGBA(); bl < 0xC000 || r > 0x4000 {
		t.Errorf("bottom is not blue: %v", bottom)
	}
	if o := orientation(b.Bytes()); o != 1 {
		t.Errorf("a JPEG without EXIF: orientation %d", o)
	}
}

func TestTooLargeAndBroken(t *testing.T) {
	defer func(n int) { MaxPixels = n }(MaxPixels)
	MaxPixels = 100
	if _, _, err := Make(encodePNG(t, filled(20, 20, color.White)), ".png", 64); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	if _, _, err := Make([]byte("not an image"), ".png", 64); err == nil {
		t.Error("garbage decoded")
	}
}
