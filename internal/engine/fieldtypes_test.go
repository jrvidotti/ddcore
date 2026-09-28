package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/signature"
)

func castOne(t *testing.T, f *meta.Field, v any) any {
	t.Helper()
	got, err := castValueWith(f, v, utcOpts)
	if err != nil {
		t.Fatalf("castValueWith(%v) on %s: %v", v, f.Fieldtype, err)
	}
	return got
}

func castFails(t *testing.T, f *meta.Field, v any) {
	t.Helper()
	if got, err := castValueWith(f, v, utcOpts); err == nil {
		t.Fatalf("expected %s to refuse %v, got %v", f.Fieldtype, v, got)
	}
}

// Rich text is cleaned on the way in, so what the column holds is what every
// reader gets. A value that predates rich text is plain text and is read as
// such — the angle bracket in "a < b" is not the start of a tag.
func TestCastTextEditorSanitizes(t *testing.T) {
	f := &meta.Field{Fieldname: "notes", Fieldtype: "Text Editor", Label: "Notes"}
	got := castOne(t, f, `<p>hi<img src=x onerror=alert(1)></p>`).(string)
	if strings.Contains(got, "onerror") || strings.Contains(got, "<img") {
		t.Fatalf("stored value kept the attack: %s", got)
	}
	if got := castOne(t, f, "a < b"); got != "<p>a &lt; b</p>" {
		t.Fatalf("plain text read as markup: %v", got)
	}
	// clearing the field in an editor leaves an empty paragraph behind, and
	// `reqd` has to see that as empty
	if got := castOne(t, f, "<p></p>"); got != nil {
		t.Fatalf("empty markup stored as %v", got)
	}
}

func TestCastDurationRatingColorAndImage(t *testing.T) {
	duration := &meta.Field{Fieldname: "spent", Fieldtype: "Duration", Label: "Time spent"}
	if got := castOne(t, duration, 90.4); got != int64(90) {
		t.Errorf("Duration stored %v", got)
	}
	if got := castOne(t, duration, "3600"); got != int64(3600) {
		t.Errorf("Duration from a string stored %v", got)
	}
	castFails(t, duration, -1)

	rating := &meta.Field{Fieldname: "score", Fieldtype: "Rating", Label: "Score"}
	if got := castOne(t, rating, 4); got != int64(4) {
		t.Errorf("Rating stored %v", got)
	}
	castFails(t, rating, 6) // the default is five stars
	if got := castOne(t, &meta.Field{Fieldname: "score", Fieldtype: "Rating", Label: "Score", Options: float64(10)}, 8); got != int64(8) {
		t.Errorf("Rating with ten stars stored %v", got)
	}

	colour := &meta.Field{Fieldname: "accent", Fieldtype: "Color", Label: "Accent"}
	for in, want := range map[string]string{"#ABC": "#aabbcc", " #A1B2C3 ": "#a1b2c3", "a1b2c3": "#a1b2c3"} {
		if got := castOne(t, colour, in); got != want {
			t.Errorf("Color(%q) = %v, want %q", in, got, want)
		}
	}
	castFails(t, colour, "red")
	castFails(t, colour, "#12345")

	image := &meta.Field{Fieldname: "photo", Fieldtype: "Attach Image", Label: "Photo"}
	if got := castOne(t, image, "/files/a.PNG"); got != "/files/a.PNG" {
		t.Errorf("Attach Image stored %v", got)
	}
	castFails(t, image, "/files/report.pdf")
	castFails(t, image, "/files/logo.svg") // svg carries script
	castFails(t, image, "https://evil.example/p.png")
}

func TestCodeAndMarkdownKeepTheirSource(t *testing.T) {
	code := &meta.Field{Fieldname: "body", Fieldtype: "Code", Label: "Body", Options: "sql"}
	in := "select 1\n  from dual  \n"
	if got := castOne(t, code, in); got != in {
		t.Fatalf("Code changed its source: %q", got)
	}
	md := &meta.Field{Fieldname: "readme", Fieldtype: "Markdown Editor", Label: "Readme"}
	src := "# Title\n\n*not* html <b>either</b>\n"
	if got := castOne(t, md, src); got != src {
		t.Fatalf("Markdown changed its source: %q", got)
	}
}

const fieldtypesDoctype = `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Note", module: "Demo", trackChanges: true, fields: [
 {fieldname: "title", fieldtype: "Data", label: "Title"},
 {fieldname: "body", fieldtype: "Text Editor", label: "Body"},
 {fieldname: "readme", fieldtype: "Markdown Editor", label: "Readme"},
 {fieldname: "snippet", fieldtype: "Code", label: "Snippet", options: "sql"},
 {fieldname: "spent", fieldtype: "Duration", label: "Time spent"},
 {fieldname: "score", fieldtype: "Rating", label: "Score"},
 {fieldname: "accent", fieldtype: "Color", label: "Accent"},
 {fieldname: "photo", fieldtype: "Attach Image", label: "Photo"},
 {fieldname: "tag", fieldtype: "Autocomplete", label: "Tag", options: ["Red", "Blue"]},
 {fieldname: "sku", fieldtype: "Barcode", label: "SKU"},
 {fieldname: "gtin", fieldtype: "Barcode", label: "GTIN", options: "EAN-13"},
 {fieldname: "link_qr", fieldtype: "Barcode", label: "Link QR", options: "QR"},
 {fieldname: "signed", fieldtype: "Signature", label: "Signed by"},
 {fieldname: "locked", fieldtype: "Text Editor", label: "Locked", readOnly: true}
], permissions: [{role: "All", read: true, write: true, create: true, delete: true}]});`

// The value that comes back has to be the value the next save sends: a
// document opened and saved untouched must record no change at all, or every
// read-only rich-text field becomes unsaveable and the timeline fills with
// empty diffs.
func TestRichTextRoundTripsThroughSave(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/note/note.doctype.ts": fieldtypesDoctype})
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		doc, err := c.NewDoc("Note", Doc{
			"title": "One", "body": `<p>hello <strong>world</strong></p>`,
			"snippet": "select 1", "spent": 5400, "score": 4, "accent": "#ABC", "locked": "fixed",
			"tag": "  Violet ", "sku": " SKU-1 ", "gtin": "400638133393", "link_qr": "https://ddcore.dev/n/1",
			"signed": signaturePNG(t, 40, 20),
		})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		loaded, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		if loaded.Str("body") != `<p>hello <strong>world</strong></p>` {
			t.Fatalf("body came back as %q", loaded.Str("body"))
		}
		if loaded["spent"] != int64(5400) || loaded["score"] != int64(4) || loaded.Str("accent") != "#aabbcc" {
			t.Fatalf("numbers and colour came back as %v, %v, %v", loaded["spent"], loaded["score"], loaded["accent"])
		}
		if loaded.Str("tag") != "Violet" {
			t.Fatalf("an Autocomplete is free text, trimmed: %q", loaded.Str("tag"))
		}
		if loaded.Str("sku") != "SKU-1" || loaded.Str("gtin") != "4006381333931" || loaded.Str("link_qr") != "https://ddcore.dev/n/1" {
			t.Fatalf("barcodes came back as %q, %q, %q", loaded.Str("sku"), loaded.Str("gtin"), loaded.Str("link_qr"))
		}
		// saving it again, untouched, must change nothing and write no Version
		if _, err := c.Save(loaded, SaveOpts{}); err != nil {
			return err
		}
		var versions int
		if err := c.Q().QueryRow(c.Ctx, `SELECT count(*) FROM tab_version WHERE ref_doctype = 'Note'`).Scan(&versions); err != nil {
			return err
		}
		if versions != 0 {
			t.Fatalf("an untouched save wrote %d version(s)", versions)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Everything already in a Text Editor column was written as plain text. The
// first save after the upgrade converts it, which is a change of bytes and not
// of content: it must pass the read-only check and leave the timeline alone.
func TestLegacyPlainTextIsNotAChange(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/note/note.doctype.ts": fieldtypesDoctype})
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		doc, err := c.NewDoc("Note", Doc{"title": "Legacy", "locked": "as it was"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		// write the column the way a pre-0.17 server did
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_note SET body = $1, locked = $2 WHERE id = $3`,
			"line one\nline two", "as it was", saved.ID()); err != nil {
			return err
		}
		loaded, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		if loaded.Str("body") != "line one\nline two" {
			t.Fatalf("the stored value is not what was written: %q", loaded.Str("body"))
		}
		loaded["title"] = "Legacy edited"
		again, err := c.Save(loaded, SaveOpts{})
		if err != nil {
			return err // a read-only field comparing unequal to itself would land here
		}
		if again.Str("body") != "<p>line one<br>line two</p>" {
			t.Fatalf("the conversion did not happen: %q", again.Str("body"))
		}
		var data string
		if err := c.Q().QueryRow(c.Ctx, `SELECT data FROM tab_version WHERE ref_doctype = 'Note' ORDER BY creation DESC LIMIT 1`).Scan(&data); err != nil {
			return err
		}
		if strings.Contains(data, "body") || strings.Contains(data, "locked") {
			t.Fatalf("the conversion reached the timeline: %s", data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Text, Small Text and Data share a column with the text types, and Int shares
// one with Duration and Rating, so changing a field's type over existing data
// is a metadata change with no SQL and nothing to convert.
func TestTextTypesShareTheirColumn(t *testing.T) {
	for _, pair := range [][2]string{
		{"Text", "Text Editor"}, {"Small Text", "Markdown Editor"}, {"Data", "Code"},
		{"Data", "Color"}, {"Data", "Autocomplete"}, {"Data", "Barcode"}, {"Text", "Signature"}, {"Attach", "Attach Image"}, {"Int", "Duration"}, {"Int", "Rating"},
	} {
		if a, b := meta.ColumnType(pair[0]), meta.ColumnType(pair[1]); a != b {
			t.Errorf("%s (%s) and %s (%s) do not share a column", pair[0], a, pair[1], b)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	for _, tc := range []struct {
		secs                  int64
		hideDays, hideSeconds bool
		want                  string
	}{
		{0, false, false, "0s"},
		{0, false, true, "0m"},
		{45, false, false, "45s"},
		{5400, false, false, "1h 30m"},
		{93784, false, false, "1d 2h 3m 4s"},
		{93784, true, false, "26h 3m 4s"},
		{93784, false, true, "1d 2h 3m"},
		{-10, false, false, "0s"},
	} {
		if got := FormatDuration(tc.secs, tc.hideDays, tc.hideSeconds); got != tc.want {
			t.Errorf("FormatDuration(%d, %v, %v) = %q, want %q", tc.secs, tc.hideDays, tc.hideSeconds, got, tc.want)
		}
	}
}

// A Rating column can hold a value outside its range — the field's `options`
// may have been lowered, or the column was an Int before. Reading clamps the
// value so callers (API, desk, reports) receive a value within [0, max].
func TestRatingOutOfRangeIsClampedOnRead(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/note/note.doctype.ts": fieldtypesDoctype})
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		doc, err := c.NewDoc("Note", Doc{"title": "Clamp Test", "score": 3})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}

		// Directly write out-of-range ratings via SQL (e.g. legacy data from before type change)
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_note SET score = 42 WHERE id = $1`, saved.ID()); err != nil {
			return err
		}

		loaded, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		if got := loaded["score"]; got != int64(5) {
			t.Fatalf("GetDoc score over max: got %v (%T), want int64(5)", got, got)
		}

		rows, err := c.GetList("Note", ListArgs{Filters: map[string]any{"id": saved.ID()}, Fields: []string{"id", "score"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["score"] != int64(5) {
			t.Fatalf("GetList score over max: got %v, want int64(5)", rows[0]["score"])
		}

		val, err := c.GetValue("Note", saved.ID(), "score")
		if err != nil {
			return err
		}
		if val != int64(5) {
			t.Fatalf("GetValue score over max: got %v, want int64(5)", val)
		}

		// Test negative value clamped to 0
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_note SET score = -5 WHERE id = $1`, saved.ID()); err != nil {
			return err
		}
		loadedNeg, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		if got := loadedNeg["score"]; got != int64(0) {
			t.Fatalf("GetDoc score negative: got %v, want int64(0)", got)
		}

		// Test null rating remains nil
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_note SET score = NULL WHERE id = $1`, saved.ID()); err != nil {
			return err
		}
		loadedNull, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		if got := loadedNull["score"]; got != nil {
			t.Fatalf("GetDoc null score: got %v, want nil", got)
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Autocomplete suggests and never restricts: whatever is typed is kept,
// trimmed, and a blank is no value at all.
func TestCastAutocompleteKeepsFreeText(t *testing.T) {
	f := &meta.Field{Fieldname: "tag", Fieldtype: "Autocomplete", Options: []any{"Red", "Blue"}}
	if got := castOne(t, f, "  Green  "); got != "Green" {
		t.Fatalf("free text came back as %v", got)
	}
	if got := castOne(t, f, "   "); got != nil {
		t.Fatalf("a blank should be nil, got %v", got)
	}
}

// A Data field turned Autocomplete keeps whatever whitespace Data kept. The
// first save trims it, which is no edit anybody made, so the timeline stays
// quiet about it.
func TestDataToAutocompleteIsNotAChange(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/note/note.doctype.ts": fieldtypesDoctype})
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		saved, err := c.Insert(Doc{"doctype": "Note", "title": "Padded"}, SaveOpts{})
		if err != nil {
			return err
		}
		// write the column the way a Data field did, untrimmed
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_note SET tag = $1 WHERE id = $2`, " Red ", saved.ID()); err != nil {
			return err
		}
		loaded, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		loaded["title"] = "Padded edited"
		again, err := c.Save(loaded, SaveOpts{})
		if err != nil {
			return err
		}
		if again.Str("tag") != "Red" {
			t.Fatalf("the save did not trim: %q", again.Str("tag"))
		}
		var data string
		if err := c.Q().QueryRow(c.Ctx, `SELECT data FROM tab_version WHERE ref_doctype = 'Note' ORDER BY creation DESC LIMIT 1`).Scan(&data); err != nil {
			return err
		}
		if strings.Contains(data, "tag") {
			t.Fatalf("the trim reached the timeline: %s", data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Only a Select's options are values that carry labels. Autocomplete
// suggestions are what gets typed and stored, so translating them would show
// one word and save another.
func TestOnlySelectOptionsGetLabels(t *testing.T) {
	st := &State{I18n: &I18n{Lang: "en", dict: map[string]map[string]string{"pt-BR": {"Red": "Vermelho"}}}}
	d := &meta.DocType{Name: "Paint", Fields: []*meta.Field{
		{Fieldname: "shade", Fieldtype: "Select", Options: []any{"Red"}},
		{Fieldname: "tag", Fieldtype: "Autocomplete", Options: []any{"Red"}},
	}}
	out := st.TranslateDocType(d, "pt-BR")
	if got := out.Field("shade").OptionLabels; len(got) != 1 || got[0] != "Vermelho" {
		t.Fatalf("the Select lost its labels: %v", got)
	}
	if got := out.Field("tag").OptionLabels; got != nil {
		t.Fatalf("Autocomplete suggestions were translated: %v", got)
	}
}

// A Barcode stores what its symbology can draw. An EAN-13 typed without its
// check digit gets it, and one with the wrong digit is refused naming the
// field, so a label printed later scans as the value that was saved.
func TestCastBarcode(t *testing.T) {
	sku := &meta.Field{Fieldname: "sku", Fieldtype: "Barcode", Label: "SKU"}
	if got := castOne(t, sku, "  AB-12 "); got != "AB-12" {
		t.Fatalf("Code128 stored %v", got)
	}
	if got := castOne(t, sku, "   "); got != nil {
		t.Fatalf("a blank should be nil, got %v", got)
	}
	castFails(t, sku, "tab\there")
	castFails(t, sku, strings.Repeat("x", 81))

	gtin := &meta.Field{Fieldname: "gtin", Fieldtype: "Barcode", Label: "GTIN", Options: "EAN-13"}
	if got := castOne(t, gtin, "400638133393"); got != "4006381333931" {
		t.Fatalf("EAN-13 without its check digit stored %v", got)
	}
	if got := castOne(t, gtin, "4006381333931"); got != "4006381333931" {
		t.Fatalf("a complete EAN-13 stored %v", got)
	}
	_, err := castValueWith(gtin, "4006381333932", utcOpts)
	if err == nil || !strings.Contains(err.Error(), "GTIN") || !strings.Contains(err.Error(), "4006381333932") {
		t.Fatalf("a wrong check digit should be refused naming the field: %v", err)
	}
	castFails(t, gtin, "ABC")

	qr := &meta.Field{Fieldname: "q", Fieldtype: "Barcode", Label: "Q", Options: "QR"}
	castFails(t, qr, strings.Repeat("x", 1001))
}

// A Data field turned EAN-13 Barcode holds numbers typed without their check
// digit. The first save completes them, which is no edit anybody made.
func TestDataToBarcodeIsNotAChange(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/note/note.doctype.ts": fieldtypesDoctype})
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		saved, err := c.Insert(Doc{"doctype": "Note", "title": "Legacy EAN"}, SaveOpts{})
		if err != nil {
			return err
		}
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_note SET gtin = $1 WHERE id = $2`, "400638133393", saved.ID()); err != nil {
			return err
		}
		loaded, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		loaded["title"] = "Legacy EAN edited"
		again, err := c.Save(loaded, SaveOpts{})
		if err != nil {
			return err
		}
		if again.Str("gtin") != "4006381333931" {
			t.Fatalf("the save did not complete the EAN: %q", again.Str("gtin"))
		}
		var data string
		if err := c.Q().QueryRow(c.Ctx, `SELECT data FROM tab_version WHERE ref_doctype = 'Note' ORDER BY creation DESC LIMIT 1`).Scan(&data); err != nil {
			return err
		}
		if strings.Contains(data, "gtin") {
			t.Fatalf("the check digit reached the timeline: %s", data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// signaturePNG draws a w×h PNG with one dark pixel and returns its data URL;
// the size tells two signatures apart.
func signaturePNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return signature.Prefix + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// A Signature stores a PNG data URL and nothing else: the value lands in an
// <img src> on the form and in print.
func TestCastSignature(t *testing.T) {
	f := &meta.Field{Fieldname: "signed", Fieldtype: "Signature", Label: "Signed by"}
	sig := signaturePNG(t, 30, 10)
	if got := castOne(t, f, "  "+sig+" "); got != sig {
		t.Fatalf("a signature should be stored as sent, trimmed")
	}
	if got := castOne(t, f, "  "); got != nil {
		t.Fatalf("a blank should be nil, got %v", got)
	}
	for _, bad := range []string{
		"javascript:alert(1)", "data:image/svg+xml;base64,PHN2Zz4=", signature.Prefix + "%%%",
		signature.Prefix + base64.StdEncoding.EncodeToString([]byte("\xff\xd8\xff\xe0 not a png")),
		signaturePNG(t, signature.MaxWidth+1, 10),
	} {
		_, err := castValueWith(f, bad, utcOpts)
		if err == nil || !strings.Contains(err.Error(), "Signed by") {
			t.Fatalf("%.40q should be refused naming the field: %v", bad, err)
		}
	}
}

// The timeline records that a signature changed, not two copies of a 64 KiB
// image: each side is a short hash, and nil stays nil.
func TestSignatureVersionStoresMarker(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/note/note.doctype.ts": fieldtypesDoctype})
	ctx := context.Background()
	var id string
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		saved, err := c.Insert(Doc{"doctype": "Note", "title": "Signed"}, SaveOpts{})
		id = saved.ID()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// each save in its own transaction, so each Version has its own creation
	var versions []map[string][]any
	for _, v := range []any{signaturePNG(t, 40, 20), signaturePNG(t, 50, 20), nil} {
		if err := e.Run(ctx, "Admin", func(c *Ctx) error {
			doc, err := c.GetDoc("Note", id)
			if err != nil {
				return err
			}
			doc["signed"] = v
			if _, err := c.Save(doc, SaveOpts{}); err != nil {
				return err
			}
			var data string
			if err := c.Q().QueryRow(c.Ctx, `SELECT data FROM tab_version WHERE ref_doctype = 'Note' ORDER BY creation DESC LIMIT 1`).Scan(&data); err != nil {
				return err
			}
			if strings.Contains(data, "data:image") {
				t.Fatalf("the image reached the timeline: %.200s", data)
			}
			var parsed struct{ Changed map[string][]any }
			if err := json.Unmarshal([]byte(data), &parsed); err != nil {
				return err
			}
			versions = append(versions, parsed.Changed)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	marker := regexp.MustCompile(`^sha256:[0-9a-f]{12}$`)
	signed, resigned, cleared := versions[0]["signed"], versions[1]["signed"], versions[2]["signed"]
	if signed[0] != nil || !marker.MatchString(fmt.Sprint(signed[1])) {
		t.Fatalf("signing: %v", signed)
	}
	if resigned[0] != signed[1] || !marker.MatchString(fmt.Sprint(resigned[1])) || resigned[1] == resigned[0] {
		t.Fatalf("signing again: %v after %v", resigned, signed)
	}
	if cleared[0] != resigned[1] || cleared[1] != nil {
		t.Fatalf("clearing: %v", cleared)
	}
}

// A spreadsheet cell cannot carry a drawn signature: the column is offered as
// not importable, with the reason.
func TestSignatureIsNotImported(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/note/note.doctype.ts": fieldtypesDoctype})
	c := e.NewCtx(context.Background(), "Admin")
	c.Lang = "en"
	d, err := c.St.DocType("Note")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.importableReason(d, d.Field("signed"), c.FieldAccess(d)); got != "Signatures are signed on the form" {
		t.Fatalf("a Signature should not be importable: %q", got)
	}
	for _, f := range c.importableFields(d) {
		if f.Fieldname == "signed" {
			t.Fatal("the Signature is among the importable fields")
		}
	}
}
