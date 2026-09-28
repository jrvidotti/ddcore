package api

import (
	"net/url"
	"strings"
	"testing"
)

func barcodePath(sym, value string) string {
	return "/api/barcode?" + url.Values{"symbology": {sym}, "value": {value}}.Encode()
}

func TestBarcode_DrawsAnSVG(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	for _, tc := range [][2]string{{"", "ABC-1"}, {"EAN-13", "400638133393"}, {"QR", "https://ddcore.dev/?a=1&b=2"}} {
		r := x.call("GET", barcodePath(tc[0], tc[1]), nil, ana)
		if r.Status != 200 || !strings.HasPrefix(r.Raw, "<svg") {
			t.Fatalf("%v: %d %s", tc, r.Status, r.Raw)
		}
		h := r.Header
		if h.Get("Content-Type") != "image/svg+xml" || h.Get("X-Content-Type-Options") != "nosniff" ||
			h.Get("Cache-Control") != "private, max-age=86400" {
			t.Fatalf("%v: headers %v", tc, h)
		}
	}
	// the EAN text carries the check digit the save would have added
	if r := x.call("GET", barcodePath("EAN-13", "400638133393"), nil, ana); !strings.Contains(r.Raw, "4006381333931") {
		t.Fatalf("EAN-13 drawn without its check digit: %s", r.Raw)
	}
}

func TestBarcode_RefusesABadValue(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	for _, p := range []string{
		barcodePath("EAN-13", "4006381333932"), barcodePath("EAN-13", "abc"),
		barcodePath("UPC-A", "123"), barcodePath("", ""), barcodePath("QR", strings.Repeat("x", 1001)),
	} {
		r := x.call("GET", p, nil, ana)
		x.expect(r, 400, "ValidationError")
		if strings.HasPrefix(r.Raw, "<svg") {
			t.Fatalf("%s drew an image", p)
		}
	}
}

func TestBarcode_NeedsASignedInUser(t *testing.T) {
	x := setup(t)
	x.expect(x.call("GET", barcodePath("", "ABC"), nil, ""), 401, "AuthenticationError")
}

// A portal page shows a Barcode field through the same endpoint, so a
// Website User may call it although the desk's routes are closed to them.
func TestPortal_WebsiteUserDrawsBarcodes(t *testing.T) {
	x := setupPortalAPI(t)
	ana := "sid:" + x.sid(portalAna)
	if r := x.call("GET", barcodePath("QR", "member-1"), nil, ana); r.Status != 200 || !strings.HasPrefix(r.Raw, "<svg") {
		t.Fatalf("a Website User could not draw a barcode: %d %s", r.Status, r.Raw)
	}
}
