package barcode

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

var update = flag.Bool("update", false, "rewrite the golden SVGs in testdata")

// Known EAN-13 numbers, printed on real products and in the GS1 examples.
func TestCheckDigit(t *testing.T) {
	for _, full := range []string{"4006381333931", "5901234123457", "9780201379624", "0012345678905", "7891000100103"} {
		if got := CheckDigit(full[:12]); got != int(full[12]-'0') {
			t.Errorf("CheckDigit(%s) = %d, want %c", full[:12], got, full[12])
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		sym, in, want, kind string
	}{
		{"", "  ABC-123 ", "ABC-123", ""},
		{Code128, "", "", ""},
		{Code128, "   ", "", ""},
		{Code128, "tab\there", "", KindCharacters},
		{Code128, "café", "", KindCharacters},
		{Code128, strings.Repeat("x", 80), strings.Repeat("x", 80), ""},
		{Code128, strings.Repeat("x", 81), "", KindTooLong},
		{EAN13, "400638133393", "4006381333931", ""},
		{EAN13, "4006381333931", "4006381333931", ""},
		{EAN13, "0012345678905", "0012345678905", ""},
		{EAN13, "4006381333932", "", KindCheckDigit},
		{EAN13, "40063813339", "", KindDigits},
		{EAN13, "40063813339312", "", KindDigits},
		{EAN13, "40063813339a", "", KindDigits},
		{QR, "https://example.com/?a=1&b=2", "https://example.com/?a=1&b=2", ""},
		{QR, "ação ✓", "ação ✓", ""},
		{QR, strings.Repeat("é", 500), strings.Repeat("é", 500), ""},
		{QR, strings.Repeat("é", 501), "", KindTooLong},
		{"UPC-A", "123", "", KindSymbology},
	}
	for _, tc := range cases {
		got, err := Normalize(tc.sym, tc.in)
		var e *Error
		switch {
		case tc.kind == "" && err != nil:
			t.Errorf("Normalize(%s, %q): %v", tc.sym, tc.in, err)
		case tc.kind != "" && (!errors.As(err, &e) || e.Kind != tc.kind):
			t.Errorf("Normalize(%s, %q) = %q, %v; want a %s error", tc.sym, tc.in, got, err, tc.kind)
		case got != tc.want:
			t.Errorf("Normalize(%s, %q) = %q, want %q", tc.sym, tc.in, got, tc.want)
		}
		if err == nil {
			// saving what was read back must change nothing
			if again, err := Normalize(tc.sym, got); err != nil || again != got {
				t.Errorf("Normalize is not idempotent on %q: %q, %v", got, again, err)
			}
		}
	}
}

func TestInvalidNamesTheField(t *testing.T) {
	_, err := Normalize(EAN13, "4006381333932")
	var ce *cerr.Error
	if !errors.As(Invalid("GTIN", err), &ce) || ce.Type != "ValidationError" {
		t.Fatalf("not a validation error: %v", Invalid("GTIN", err))
	}
	if !strings.Contains(ce.Message, "GTIN") || !strings.Contains(ce.Message, "4006381333932") {
		t.Fatalf("the message names neither field nor value: %s", ce.Message)
	}
	// a QR code's limit is in bytes, Code 128's in characters
	_, err = Normalize(QR, strings.Repeat("é", 501))
	if msg := Invalid("Link", err).Error(); !strings.Contains(msg, "1000 bytes") {
		t.Fatalf("the QR limit should be given in bytes: %s", msg)
	}
	_, err = Normalize(Code128, strings.Repeat("x", 81))
	if msg := Invalid("SKU", err).Error(); !strings.Contains(msg, "80 characters") {
		t.Fatalf("the Code 128 limit should be given in characters: %s", msg)
	}
	plain := errors.New("other")
	if Invalid("GTIN", plain) != plain {
		t.Fatal("an unrelated error was rewritten")
	}
}

func TestSVGGolden(t *testing.T) {
	for _, tc := range []struct{ name, sym, value string }{
		{"code128", Code128, "DDCORE-0042"},
		{"ean13", EAN13, "400638133393"},
		{"qr", QR, "https://ddcore.dev/p/42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SVG(tc.sym, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if n := bytes.Count(got, []byte("<path")); n != 1 {
				t.Fatalf("want one path, got %d", n)
			}
			if !bytes.Contains(got, []byte(`shape-rendering="crispEdges"`)) || !bytes.Contains(got, []byte("viewBox=")) {
				t.Fatalf("missing crispEdges or viewBox: %s", got)
			}
			golden := filepath.Join("testdata", tc.name+".svg")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s differs from the golden file (run with -update if the change is intended)\n%s", golden, got)
			}
		})
	}
}

func TestSVGText(t *testing.T) {
	got, err := SVG(Code128, `<a href="x">&`)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte("<a ")) || !bytes.Contains(got, []byte("&lt;a href=&#34;x&#34;&gt;&amp;")) {
		t.Fatalf("the text under the bars is not escaped: %s", got)
	}
	ean, _ := SVG(EAN13, "400638133393")
	if !bytes.Contains(ean, []byte(">4006381333931</text>")) {
		t.Fatalf("EAN-13 text should carry the check digit: %s", ean)
	}
	qr, _ := SVG(QR, "hello")
	if bytes.Contains(qr, []byte("<text")) {
		t.Fatal("a QR code carries no text")
	}
	for _, bad := range [][2]string{{EAN13, "123"}, {"Nope", "x"}, {Code128, " "}} {
		if _, err := SVG(bad[0], bad[1]); err == nil {
			t.Errorf("SVG(%q, %q) should fail", bad[0], bad[1])
		}
	}
}
