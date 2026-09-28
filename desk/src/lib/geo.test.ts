import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { formatGeo, fromLatLon, GeoError, normalizeGeo, summarizeGeo } from "./geo";
import { formatValue } from "./format";

// The same file the Go tests read (internal/geo): the map commits what the
// server stores, so the two normalizers cannot disagree.
const fixture = JSON.parse(
  // vitest runs from `desk/`, next to the Go tree that owns the fixture
  readFileSync(resolve(process.cwd(), "../internal/geo/testdata/cases.json"), "utf8"),
);

const kindOf = (fn: () => unknown): string | null => {
  try {
    fn();
    return null;
  } catch (e) {
    return e instanceof GeoError ? e.kind : "not a GeoError";
  }
};

describe("the shared fixture", () => {
  for (const c of fixture.normalize) {
    it(`normalize: ${c.name}`, () => {
      if (c.error) expect(kindOf(() => normalizeGeo(c.in))).toBe(c.error);
      else expect(normalizeGeo(c.in)).toEqual(c.out);
    });
  }
  for (const c of fixture.latlon) {
    it(`latlon: ${c.name}`, () => {
      if (c.error) return expect(kindOf(() => fromLatLon(c.in, c.decimal))).toBe(c.error);
      const got = fromLatLon(c.in, c.decimal);
      if (c.out === null) expect(got).toBeNull();
      else expect(summarizeGeo(got).point).toEqual(c.out);
    });
  }
  for (const c of fixture.summary) {
    it(`summary: ${c.name}`, () => {
      expect(summarizeGeo(c.in)).toEqual({ points: c.points, lines: c.lines, polygons: c.polygons, point: c.point });
    });
  }
});

describe("normalizeGeo", () => {
  it("is stable: a stored value normalized again, or read back as JSON, is the same", () => {
    for (const c of fixture.normalize) {
      if (c.error || !c.out) continue;
      const once = normalizeGeo(c.in);
      expect(normalizeGeo(once)).toEqual(once);
      expect(normalizeGeo(JSON.stringify(once))).toEqual(once);
    }
  });

  it("refuses more than 500 shapes and more than 64 KiB", () => {
    const point = (i: number) => ({ type: "Feature", geometry: { type: "Point", coordinates: [i % 180, 0] } });
    const many = Array.from({ length: 501 }, (_, i) => point(i));
    expect(kindOf(() => normalizeGeo({ type: "FeatureCollection", features: many }))).toBe("tooMany");
    expect(kindOf(() => normalizeGeo({ type: "FeatureCollection", features: many.slice(0, 500) }))).toBeNull();
    const line = Array.from({ length: 3000 }, (_, i) => [-179.1234567 + i / 1e4, -89.1234567]);
    expect(kindOf(() => normalizeGeo({ type: "LineString", coordinates: line }))).toBe("tooLarge");
  });

  it("rounds half away from zero, as the server does", () => {
    expect(normalizeGeo({ type: "Point", coordinates: [-0.00000005, 0.00000005] })!.features[0].geometry.coordinates).toEqual([-0.0000001, 0.0000001]);
  });
});

describe("formatGeo", () => {
  it("says a point as lat, lon and anything else as what it holds", () => {
    expect(formatGeo({ type: "Point", coordinates: [-46.6333, -23.5505] })).toBe("-23.55050, -46.63330");
    expect(formatGeo({ type: "MultiPoint", coordinates: [[1, 2], [3, 4]] })).toBe("{0} points".replace("{0}", "2"));
    expect(formatGeo({
      type: "FeatureCollection", features: [
        { type: "Feature", geometry: { type: "LineString", coordinates: [[0, 0], [1, 1]] } },
        { type: "Feature", geometry: { type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1]]] } },
        { type: "Feature", geometry: { type: "Polygon", coordinates: [[[0, 0], [2, 0], [2, 2]]] } },
      ],
    })).toBe("1 line, 2 polygons");
    expect(formatGeo(null)).toBe("");
    expect(formatGeo("not a place")).toBe("");
  });

  it("is what a list cell shows", () => {
    expect(formatValue({ type: "Point", coordinates: [10, 20] }, { fieldtype: "Geolocation" })).toBe("20.00000, 10.00000");
  });
});
