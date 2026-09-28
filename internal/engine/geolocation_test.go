package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/geo"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/print"
	"github.com/jrvidotti/ddcore/internal/tabular"
)

// A Geolocation is stored as its canonical collection, a map — the object a
// hook reads — and every shape GeoJSON is written in arrives as one.
func TestCastGeolocation(t *testing.T) {
	f := &meta.Field{Fieldname: "area", Fieldtype: "Geolocation", Label: "Delivery area"}
	point := castOne(t, f, map[string]any{"type": "Point", "coordinates": []any{-46.6333, -23.5505}})
	fc, ok := point.(map[string]any)
	if !ok || fc["type"] != "FeatureCollection" || len(fc["features"].([]any)) != 1 {
		t.Fatalf("a bare point should be wrapped in a collection, got %#v", point)
	}
	feature := castOne(t, f, `{"type":"Feature","properties":{"radius":10},"geometry":{"type":"Point","coordinates":[-46.6333,-23.5505,700]}}`)
	if !reflect.DeepEqual(feature, point) {
		t.Fatalf("a feature is its geometry, without properties or altitude: %v", feature)
	}
	if again := castOne(t, f, point); !reflect.DeepEqual(again, point) {
		t.Fatalf("a stored value cast again changed: %v", again)
	}
	for _, empty := range []any{nil, "", map[string]any{"type": "FeatureCollection", "features": []any{}}} {
		if got := castOne(t, f, empty); got != nil {
			t.Fatalf("%v should be nil, got %v", empty, got)
		}
	}
	for _, bad := range []any{
		"here", `{"type":"Point","coordinates":[200,0]}`,
		map[string]any{"type": "LineString", "coordinates": []any{[]any{0.0, 0.0}}},
		map[string]any{"type": "Feature", "geometry": nil},
		map[string]any{"type": "MultiPolygon", "coordinates": []any{}},
	} {
		_, err := castValueWith(f, bad, utcOpts)
		if cerr.From(err).Type != "ValidationError" || !strings.Contains(err.Error(), "Delivery area") {
			t.Fatalf("%v should be refused naming the field: %v", bad, err)
		}
	}
}

var geoApp = map[string]string{
	"doctypes/zone_stop/zone_stop.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Zone Stop", module: "Demo", isChild: true, fields: [
 {fieldname: "label", fieldtype: "Data", label: "Label", inListView: true},
 {fieldname: "path", fieldtype: "Geolocation", label: "Path", inListView: true}
]});`,
	"doctypes/zone/zone.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Zone", module: "Demo", trackChanges: true, submittable: true, fields: [
 {fieldname: "title", fieldtype: "Data", label: "Title", allowOnSubmit: true},
 {fieldname: "area", fieldtype: "Geolocation", label: "Area"},
 {fieldname: "depot", fieldtype: "Geolocation", label: "Depot", readOnly: true},
 {fieldname: "shapes", fieldtype: "Int", label: "Shapes"},
 {fieldname: "stops", fieldtype: "Table", label: "Stops", options: "Zone Stop"}
], permissions: [{role: "All", read: true, write: true, create: true, delete: true, submit: true, cancel: true}]});`,
	// a hook reads the value as the object it is
	"doctypes/zone/zone.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Zone", {
  validate(doc) {
    const n = doc.area ? doc.area.features.length : 0;
    if (n > 3) ddcore.throw("A zone has at most three shapes");
    doc.shapes = n;
  },
});`,
}

var geoArea = map[string]any{"type": "FeatureCollection", "features": []any{
	map[string]any{"type": "Feature", "properties": map[string]any{"name": "HQ"},
		"geometry": map[string]any{"type": "Point", "coordinates": []any{-46.633300049, -23.5505, 760.0}}},
	map[string]any{"type": "Feature", "geometry": map[string]any{"type": "Polygon",
		"coordinates": []any{[]any{[]any{-46.7, -23.6}, []any{-46.5, -23.6}, []any{-46.5, -23.4}}}}},
}}

func countVersions(t *testing.T, c *Ctx) int {
	t.Helper()
	var n int
	if err := c.Q().QueryRow(c.Ctx, `SELECT count(*) FROM tab_version WHERE ref_doctype = 'Zone'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The regression this type exists to avoid: a document opened and saved
// untouched records no Version, and a read-only or submitted Geolocation
// compares equal to itself — the value read back from jsonb is the value the
// cast produces.
func TestGeolocationRoundTrip(t *testing.T) {
	e := setupWith(t, geoApp)
	ctx := context.Background()
	var id string
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		saved, err := c.Insert(Doc{"doctype": "Zone", "title": "South", "area": geoArea,
			"depot": `{"type":"Point","coordinates":[-46.6,-23.5]}`,
			"stops": []any{map[string]any{"label": "Route", "path": map[string]any{"type": "LineString", "coordinates": []any{[]any{-46.6, -23.5}, []any{-46.61, -23.51}}}}},
		}, SaveOpts{})
		if err != nil {
			return err
		}
		id = saved.ID()
		if saved["shapes"] != int64(2) {
			t.Fatalf("the hook did not read doc.area.features.length: shapes = %v", saved["shapes"])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want, _ := geo.Normalize(geoArea)
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		loaded, err := c.GetDoc("Zone", id)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(loaded["area"], want) {
			t.Fatalf("the area came back as %#v", loaded["area"])
		}
		if _, err := c.Save(loaded, SaveOpts{}); err != nil {
			return err
		}
		if n := countVersions(t, c); n != 0 {
			t.Fatalf("an untouched save wrote %d version(s)", n)
		}
		_, err = c.Submit(loaded)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// submitted: only the title may change, and the areas — read-only, not
	// allowOnSubmit — must compare equal to what is stored
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		loaded, err := c.GetDoc("Zone", id)
		if err != nil {
			return err
		}
		before := countVersions(t, c)
		loaded["title"] = "South (renamed)"
		if _, err := c.Save(loaded, SaveOpts{}); err != nil {
			return err
		}
		var data string
		if err := c.Q().QueryRow(c.Ctx, `SELECT data FROM tab_version WHERE ref_doctype = 'Zone' ORDER BY creation DESC LIMIT 1`).Scan(&data); err != nil {
			return err
		}
		if countVersions(t, c) != before+1 || strings.Contains(data, "area") || strings.Contains(data, "depot") || strings.Contains(data, "stops") {
			t.Fatalf("the Version should hold the title alone: %s", data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGeolocationHookRefuses(t *testing.T) {
	e := setupWith(t, geoApp)
	many := map[string]any{"type": "MultiPoint", "coordinates": []any{[]any{1.0, 1.0}, []any{2.0, 2.0}}}
	fc := map[string]any{"type": "FeatureCollection", "features": []any{
		map[string]any{"type": "Feature", "geometry": many}, map[string]any{"type": "Feature", "geometry": many},
		map[string]any{"type": "Feature", "geometry": many}, map[string]any{"type": "Feature", "geometry": many},
	}}
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		_, err := c.Insert(Doc{"doctype": "Zone", "title": "Too much", "area": fc}, SaveOpts{})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "at most three shapes") {
		t.Fatalf("the hook should have refused four shapes: %v", err)
	}
}

// A spreadsheet cell holds GeoJSON, as an export writes it, or a point typed
// by hand, latitude first.
func TestParseCellGeolocation(t *testing.T) {
	f := &meta.Field{Fieldname: "area", Fieldtype: "Geolocation", Label: "Area"}
	tr := func(s string, a ...any) string { return s }
	txt := func(s string) tabular.Cell { return tabular.Cell{Text: s} }
	br := cellConv{decimal: ",", order: "dmy"}
	us := cellConv{decimal: ".", order: "mdy"}
	point, _ := geo.Normalize(map[string]any{"type": "Point", "coordinates": []any{-46.6333, -23.5505}})

	json := `{"type":"Point","coordinates":[-46.6333,-23.5505]}`
	if got, err := parseCell(f, "Area", txt(" "+json+" "), br, tr); err != nil || got != json {
		t.Fatalf("GeoJSON goes to the save as text: %v %v", got, err)
	}
	for _, c := range []struct {
		text string
		conv cellConv
	}{{"-23.5505; -46.6333", us}, {"-23,5505; -46,6333", br}, {"-23.5505, -46.6333", us}} {
		got, err := parseCell(f, "Area", txt(c.text), c.conv, tr)
		if err != nil || !reflect.DeepEqual(got, point) {
			t.Fatalf("%q (decimal %q): got %v, %v", c.text, c.conv.decimal, got, err)
		}
	}
	for _, c := range []struct {
		text string
		conv cellConv
	}{{"-23,5505, -46,6333", br}, {"Sao Paulo", us}, {"95; 10", us}} {
		if _, err := parseCell(f, "Area", txt(c.text), c.conv, tr); cerr.From(err).Type != "ValidationError" || !strings.Contains(err.Error(), "Area") {
			t.Fatalf("%q should be refused naming the column: %v", c.text, err)
		}
	}

	// and the column is offered to the import
	e := setupWith(t, geoApp)
	c := e.NewCtx(context.Background(), "Admin")
	d, err := c.St.DocType("Zone")
	if err != nil {
		t.Fatal(err)
	}
	if reason := c.importableReason(d, d.Field("area"), c.FieldAccess(d)); reason != "" {
		t.Fatalf("a Geolocation should be importable: %q", reason)
	}
}

// Print shows no map: a point prints as "lat, lon", anything else as what it
// holds, in the reader's language.
func TestPrintDoc_Geolocation(t *testing.T) {
	e := setupWith(t, geoApp)
	ctx := context.Background()
	var id string
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		saved, err := c.Insert(Doc{"doctype": "Zone", "title": "North",
			"depot": map[string]any{"type": "Point", "coordinates": []any{-46.6333, -23.5505}},
			"area":  geoArea,
			"stops": []any{map[string]any{"label": "Two stops", "path": map[string]any{"type": "MultiPoint", "coordinates": []any{[]any{1.0, 2.0}, []any{3.0, 4.0}}}}},
		}, SaveOpts{})
		id = saved.ID()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	c := e.NewCtx(ctx, "Admin")
	out, err := c.PrintDoc("Zone", id, "", "none", "en", print.PDFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-23.55050, -46.63330", "1 point, 1 polygon", "2 points"} {
		if !strings.Contains(out, want) {
			t.Fatalf("print is missing %q: %.3000s", want, out)
		}
	}
	if strings.Contains(out, "tile.openstreetmap.org") || strings.Contains(out, "FeatureCollection") {
		t.Fatalf("print should show a summary, not a map or the GeoJSON: %.3000s", out)
	}
	pt, err := c.PrintDoc("Zone", id, "", "none", "pt-BR", print.PDFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pt, "1 point, 1 polygon") {
		t.Fatalf("the summary is not translated: %.3000s", pt)
	}
}
