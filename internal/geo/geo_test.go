package geo

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

type fixture struct {
	Normalize []struct {
		Name  string          `json:"name"`
		In    any             `json:"in"`
		Out   json.RawMessage `json:"out"`
		Error string          `json:"error"`
	} `json:"normalize"`
	LatLon []struct {
		Name    string    `json:"name"`
		In      string    `json:"in"`
		Decimal string    `json:"decimal"`
		Out     []float64 `json:"out"`
		Error   string    `json:"error"`
	} `json:"latlon"`
	Summary []struct {
		Name     string    `json:"name"`
		In       any       `json:"in"`
		Points   int       `json:"points"`
		Lines    int       `json:"lines"`
		Polygons int       `json:"polygons"`
		Point    []float64 `json:"point"`
	} `json:"summary"`
}

// The desk runs the same fixture against lib/geo.ts, so the two cannot drift
// apart on what a stored value is.
func loadFixture(t *testing.T) fixture {
	t.Helper()
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func kindOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestNormalizeSharedFixture(t *testing.T) {
	for _, c := range loadFixture(t).Normalize {
		t.Run(c.Name, func(t *testing.T) {
			got, err := Normalize(c.In)
			if c.Error != "" {
				if kindOf(err) != c.Error {
					t.Fatalf("want a %q error, got %v (value %v)", c.Error, err, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := json.Unmarshal(c.Out, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got  %s\nwant %s", mustJSON(got), mustJSON(want))
			}
		})
	}
}

// What Normalize returns is what it keeps: a stored value saved again is the
// same value, which is what keeps an unchanged save from recording a Version.
func TestNormalizeIsIdempotent(t *testing.T) {
	for _, c := range loadFixture(t).Normalize {
		if c.Error != "" {
			continue
		}
		once, _ := Normalize(c.In)
		if once == nil {
			continue
		}
		twice, err := Normalize(once)
		if err != nil || !reflect.DeepEqual(once, twice) {
			t.Fatalf("%s: not stable: %v / %v", c.Name, twice, err)
		}
		// and through JSON text, as the driver hands it back from jsonb
		thrice, err := Normalize(string(mustJSON(once)))
		if err != nil || !reflect.DeepEqual(once, thrice) {
			t.Fatalf("%s: not stable through JSON: %v / %v", c.Name, thrice, err)
		}
	}
}

// A hook hands back Go values the JSON decoder never makes: int64s, typed
// slices. They are the same location.
func TestNormalizeTakesGoValues(t *testing.T) {
	got, err := Normalize(map[string]any{"type": "Point", "coordinates": []int64{10, 20}})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Normalize(`{"type":"Point","coordinates":[10,20]}`)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1)} {
		_, err := Normalize(map[string]any{"type": "Point", "coordinates": []float64{bad, 0}})
		if kindOf(err) != KindCoordinates {
			t.Fatalf("%v should be refused as a coordinate: %v", bad, err)
		}
	}
}

func TestNormalizeLimits(t *testing.T) {
	point := func(i int) map[string]any {
		return map[string]any{"type": "Feature", "geometry": map[string]any{"type": "Point", "coordinates": []any{float64(i % 180), 0.0}}}
	}
	many := make([]any, MaxFeatures+1)
	for i := range many {
		many[i] = point(i)
	}
	if _, err := Normalize(map[string]any{"type": "FeatureCollection", "features": many}); kindOf(err) != KindTooMany {
		t.Fatalf("%d features should be too many: %v", len(many), err)
	}
	if _, err := Normalize(map[string]any{"type": "FeatureCollection", "features": many[:MaxFeatures]}); err != nil {
		t.Fatalf("%d features should be allowed: %v", MaxFeatures, err)
	}
	// one line with enough long coordinates to pass 64 KiB
	line := make([]any, 3000)
	for i := range line {
		line[i] = []any{-179.1234567 + float64(i)/1e4, -89.1234567}
	}
	if _, err := Normalize(map[string]any{"type": "LineString", "coordinates": line}); kindOf(err) != KindTooLarge {
		t.Fatalf("a line of %d points should be too large: %v", len(line), err)
	}
}

func TestSummarize(t *testing.T) {
	for _, c := range loadFixture(t).Summary {
		s := Summarize(c.In)
		if s.Points != c.Points || s.Lines != c.Lines || s.Polygons != c.Polygons || !reflect.DeepEqual(s.Point, c.Point) {
			t.Errorf("%s: got %+v", c.Name, s)
		}
		if s.Empty() != (c.Points+c.Lines+c.Polygons == 0) {
			t.Errorf("%s: Empty() is %v", c.Name, s.Empty())
		}
	}
}

func TestFromLatLon(t *testing.T) {
	for _, c := range loadFixture(t).LatLon {
		t.Run(c.Name, func(t *testing.T) {
			got, err := FromLatLon(c.In, c.Decimal)
			if c.Error != "" {
				if kindOf(err) != c.Error {
					t.Fatalf("want a %q error, got %v", c.Error, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.Out == nil {
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
				return
			}
			if s := Summarize(got); !reflect.DeepEqual(s.Point, c.Out) {
				t.Fatalf("got %v, want [lon, lat] %v", s.Point, c.Out)
			}
		})
	}
}

// Each refusal names the field and reads as a validation error.
func TestInvalid(t *testing.T) {
	for _, kind := range []string{KindFormat, KindGeometry, KindCoordinates, KindRange, KindShape, KindTooMany, KindTooLarge} {
		err := Invalid("Delivery area", &Error{Kind: kind})
		var ce *cerr.Error
		if !errors.As(err, &ce) || ce.Type != "ValidationError" {
			t.Fatalf("%s: not a validation error: %#v", kind, err)
		}
		if !strings.Contains(ce.Message, "Delivery area") {
			t.Fatalf("%s: the message does not name the field: %v", kind, err)
		}
	}
	other := fmt.Errorf("boom")
	if Invalid("X", other) != other {
		t.Fatal("an error that is not a geo.Error should pass through")
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
