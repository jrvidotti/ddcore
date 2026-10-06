package engine

import (
	"encoding/json"
	"testing"
)

// #90: goja read "2026-10-05 10:00:00-04:00" as midnight -04:00 and the
// Postgres timestamptz text form as NaN. Both now parse as V8 parses them,
// and every other use of Date behaves as before.
func TestDateParsesASpaceSeparatedInstant(t *testing.T) {
	out, err := evalNoDB(t, `const iso = (v: any) => new Date(v).toISOString();
const r: any = {
  spaced: iso(Date.parse("2026-10-05 10:00:00-04:00")),
  ctor: iso("2026-10-05 10:00:00-04:00"),
  t: iso("2026-10-05T10:00:00-04:00"),
  pg: iso("2026-10-05 22:31:52.767353+00"),
  spaceBeforeOffset: iso("2026-10-05 10:00 -0400"),
  lowerZ: iso("2026-10-05t10:00:00z"),
  dateOnly: iso("2026-10-05"),
  parts: new Date(Date.UTC(2026, 0, 2, 3, 4, 5)).toISOString(),
  ms: new Date(0).toISOString(),
  now: typeof Date.now() === "number",
  call: typeof Date() === "string",
  instance: new Date() instanceof Date && new Date().constructor === Date,
  garbage: isNaN(Date.parse("not a date")),
  sub: (() => { class D extends Date { y() { return this.getUTCFullYear(); } } return new D("2026-10-05 10:00:00Z").y(); })(),
  fromDate: iso(new Date(new Date("2026-10-05 10:00:00Z"))),
};
JSON.stringify(r)`)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{
		"spaced":            "2026-10-05T14:00:00.000Z",
		"ctor":              "2026-10-05T14:00:00.000Z",
		"t":                 "2026-10-05T14:00:00.000Z",
		"pg":                "2026-10-05T22:31:52.767Z",
		"spaceBeforeOffset": "2026-10-05T14:00:00.000Z",
		"lowerZ":            "2026-10-05T10:00:00.000Z",
		"dateOnly":          "2026-10-05T00:00:00.000Z",
		"parts":             "2026-01-02T03:04:05.000Z",
		"ms":                "1970-01-01T00:00:00.000Z",
		"now":               true,
		"call":              true,
		"instance":          true,
		"garbage":           true,
		"sub":               float64(2026),
		"fromDate":          "2026-10-05T10:00:00.000Z",
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
}
