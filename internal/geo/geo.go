// Package geo validates the value of a Geolocation field: GeoJSON, stored as
// a FeatureCollection in a jsonb column.
//
// It is a leaf: the engine calls Normalize on every write, so what the column
// holds is always one canonical shape — a FeatureCollection of Features whose
// geometries are Points, MultiPoints, LineStrings or Polygons, coordinates
// rounded to 7 decimals, no altitude and no properties. Canonical means a
// value read back and saved again is equal to itself, which is what keeps an
// unchanged save from recording a Version.
//
// The desk mirrors Normalize in desk/src/lib/geo.ts; both run the cases in
// testdata/cases.json.
package geo

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// Limits on a stored value. 500 shapes and 64 KiB are far past a delivery
// area or a route, and keep a document's row and its API payload small.
const (
	MaxFeatures = 500
	MaxBytes    = 64 << 10
	// Decimals is the precision kept: 1e-7 degrees is about 1 cm.
	Decimals = 7
)

// Kinds of Error, one per message the user can get.
const (
	KindFormat      = "format"      // not GeoJSON, or not JSON at all
	KindGeometry    = "geometry"    // a geometry type this field does not take, or none
	KindCoordinates = "coordinates" // a position that is not two finite numbers
	KindRange       = "range"       // a longitude or a latitude out of range
	KindShape       = "shape"       // a line or a ring with too few positions
	KindTooMany     = "tooMany"     // more than MaxFeatures features
	KindTooLarge    = "tooLarge"    // more than MaxBytes of canonical JSON
)

// Error says why a value cannot be a Geolocation. It carries a Kind rather
// than a sentence so the caller can phrase it with the field's translated
// label.
type Error struct {
	Kind string
}

func (e *Error) Error() string { return "geolocation: " + e.Kind }

func fail(kind string) error { return &Error{Kind: kind} }

// Normalize returns the value a Geolocation field stores: the canonical
// FeatureCollection as a map, decoded from its own JSON so that it compares
// equal to what the driver reads back from jsonb. v is a JSON string or an
// already decoded value; a FeatureCollection, a single Feature or a bare
// geometry are accepted. An empty value, or a collection with no features,
// is nil and not an error.
func Normalize(v any) (map[string]any, error) {
	root, err := decode(v)
	if err != nil || root == nil {
		return nil, err
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil, fail(KindFormat)
	}
	var features []any
	switch t, _ := obj["type"].(string); t {
	case "FeatureCollection":
		list, ok := obj["features"].([]any)
		if !ok && obj["features"] != nil {
			return nil, fail(KindFormat)
		}
		if len(list) > MaxFeatures {
			return nil, fail(KindTooMany)
		}
		for _, f := range list {
			nf, err := feature(f)
			if err != nil {
				return nil, err
			}
			features = append(features, nf)
		}
	case "Feature":
		nf, err := feature(obj)
		if err != nil {
			return nil, err
		}
		features = []any{nf}
	case "Point", "MultiPoint", "LineString", "Polygon":
		g, err := geometry(obj)
		if err != nil {
			return nil, err
		}
		features = []any{wrap(g)}
	case "MultiLineString", "MultiPolygon", "GeometryCollection":
		return nil, fail(KindGeometry)
	default:
		return nil, fail(KindFormat)
	}
	if len(features) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(map[string]any{"type": "FeatureCollection", "features": features})
	if err != nil {
		return nil, fail(KindCoordinates)
	}
	if len(b) > MaxBytes {
		return nil, fail(KindTooLarge)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decode turns v into plain JSON values: maps, slices, strings and float64s.
// A string is parsed; a value of any other Go type — a map from a hook, with
// int64s and typed slices in it — goes through JSON, which is also what
// refuses a NaN.
func decode(v any) (any, error) {
	var s string
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		s = x
	case []byte:
		s = string(x)
	default:
		b, err := json.Marshal(v)
		var unsupported *json.UnsupportedValueError
		if errors.As(err, &unsupported) {
			return nil, fail(KindCoordinates) // a NaN or an infinity
		} else if err != nil {
			return nil, fail(KindFormat)
		}
		s = string(b)
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return nil, nil
	}
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fail(KindFormat)
	}
	return out, nil
}

func wrap(g map[string]any) map[string]any {
	return map[string]any{"type": "Feature", "geometry": g, "properties": map[string]any{}}
}

// feature keeps a Feature's geometry and drops everything else: properties
// (a Frappe circle's radius among them) are not stored.
func feature(v any) (map[string]any, error) {
	obj, ok := v.(map[string]any)
	if !ok || obj["type"] != "Feature" {
		return nil, fail(KindFormat)
	}
	g, ok := obj["geometry"].(map[string]any)
	if !ok {
		return nil, fail(KindGeometry)
	}
	ng, err := geometry(g)
	if err != nil {
		return nil, err
	}
	return wrap(ng), nil
}

func geometry(g map[string]any) (map[string]any, error) {
	t, _ := g["type"].(string)
	var coords any
	var err error
	switch t {
	case "Point":
		coords, err = position(g["coordinates"])
	case "MultiPoint":
		coords, err = positions(g["coordinates"], 1)
	case "LineString":
		coords, err = positions(g["coordinates"], 2)
	case "Polygon":
		coords, err = polygon(g["coordinates"])
	default:
		return nil, fail(KindGeometry)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"type": t, "coordinates": coords}, nil
}

// position reads [lon, lat, ...], drops any altitude and rounds.
func position(v any) ([]any, error) {
	p, ok := v.([]any)
	if !ok || len(p) < 2 {
		return nil, fail(KindCoordinates)
	}
	lon, ok1 := number(p[0])
	lat, ok2 := number(p[1])
	if !ok1 || !ok2 {
		return nil, fail(KindCoordinates)
	}
	if lon < -180 || lon > 180 || lat < -90 || lat > 90 {
		return nil, fail(KindRange)
	}
	return []any{Round(lon), Round(lat)}, nil
}

func positions(v any, min int) ([]any, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fail(KindCoordinates)
	}
	out := make([]any, 0, len(list))
	for _, p := range list {
		np, err := position(p)
		if err != nil {
			return nil, err
		}
		out = append(out, np)
	}
	if len(out) < min {
		return nil, fail(KindShape)
	}
	return out, nil
}

// polygon closes each ring that does not end where it starts, and then asks
// for a triangle at least: four positions, the last repeating the first.
func polygon(v any) ([]any, error) {
	rings, ok := v.([]any)
	if !ok {
		return nil, fail(KindCoordinates)
	}
	if len(rings) == 0 {
		return nil, fail(KindShape)
	}
	out := make([]any, 0, len(rings))
	for _, r := range rings {
		ring, err := positions(r, 0)
		if err != nil {
			return nil, err
		}
		if len(ring) > 0 && !samePosition(ring[0], ring[len(ring)-1]) {
			ring = append(ring, ring[0])
		}
		if len(ring) < 4 {
			return nil, fail(KindShape)
		}
		out = append(out, ring)
	}
	return out, nil
}

func samePosition(a, b any) bool {
	pa, pb := a.([]any), b.([]any)
	return pa[0] == pb[0] && pa[1] == pb[1]
}

func number(v any) (float64, bool) {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case float32:
		n = float64(x)
	case int:
		n = float64(x)
	case int64:
		n = float64(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// Round keeps Decimals decimals, half away from zero, and never returns a
// negative zero — `-0` would print differently from `0` and read as a change.
func Round(n float64) float64 {
	r := math.Round(n*1e7) / 1e7
	if r == 0 {
		return 0
	}
	return r
}

// Summary counts what a value holds. Point is set only when the value is a
// single point and nothing else, as [lon, lat].
type Summary struct {
	Points, Lines, Polygons int
	Point                   []float64
}

// Empty reports whether the value holds nothing.
func (s Summary) Empty() bool { return s.Points+s.Lines+s.Polygons == 0 }

// Summarize counts the shapes in v, which is anything Normalize accepts; a
// value it refuses counts as empty.
func Summarize(v any) Summary {
	var s Summary
	fc, err := Normalize(v)
	if err != nil || fc == nil {
		return s
	}
	var only []any
	for _, f := range fc["features"].([]any) {
		g := f.(map[string]any)["geometry"].(map[string]any)
		coords := g["coordinates"].([]any)
		switch g["type"] {
		case "Point":
			s.Points++
			only = coords
		case "MultiPoint":
			s.Points += len(coords)
			if len(coords) == 1 {
				only = coords[0].([]any)
			}
		case "LineString":
			s.Lines++
		case "Polygon":
			s.Polygons++
		}
	}
	if s.Points == 1 && s.Lines == 0 && s.Polygons == 0 {
		s.Point = []float64{only[0].(float64), only[1].(float64)}
	}
	return s
}

// FromLatLon reads a point typed as latitude then longitude — the order people
// write and maps show — and returns it as a canonical collection. `lat; lon`
// is always accepted; `lat, lon` only when the decimal separator is ".", since
// with "," the comma is inside the numbers. Spaces around either are ignored.
func FromLatLon(s, decimal string) (map[string]any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var parts []string
	switch {
	case strings.Contains(s, ";"):
		parts = strings.Split(s, ";")
	case decimal == "" || decimal == ".":
		parts = strings.Split(s, ",")
	default:
		return nil, fail(KindFormat)
	}
	if len(parts) != 2 {
		return nil, fail(KindFormat)
	}
	var n [2]float64
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if decimal == "," {
			p = strings.ReplaceAll(p, ",", ".")
		}
		f, err := strconv.ParseFloat(p, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fail(KindFormat)
		}
		n[i] = f
	}
	return Normalize(map[string]any{"type": "Point", "coordinates": []any{n[1], n[0]}})
}

// Invalid phrases err, as returned by Normalize or FromLatLon, as a
// validation error naming the field. Anything else passes through.
func Invalid(label string, err error) error {
	var e *Error
	if !errors.As(err, &e) {
		return err
	}
	switch e.Kind {
	case KindFormat:
		return cerr.Validation("{0} must be a location: GeoJSON, or a latitude and a longitude", label)
	case KindGeometry:
		return cerr.Validation("{0} takes points, lines and areas only", label)
	case KindCoordinates:
		return cerr.Validation("{0} has a position that is not a longitude and a latitude", label)
	case KindRange:
		return cerr.Validation("{0} has a position out of range (latitude from -90 to 90, longitude from -180 to 180)", label)
	case KindShape:
		return cerr.Validation("{0} has a line with fewer than 2 points or an area with fewer than 3", label)
	case KindTooMany:
		return cerr.Validation("{0} has too many shapes (at most {1})", label, strconv.Itoa(MaxFeatures))
	case KindTooLarge:
		return cerr.Validation("{0} is too large (at most {1} KiB)", label, strconv.Itoa(MaxBytes>>10))
	}
	return err
}
