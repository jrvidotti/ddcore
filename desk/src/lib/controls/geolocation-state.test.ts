import { describe, expect, it } from "vitest";
import { addGeometry, appendVertex, canFinish, draftGeometry, fromLatLng, removeFeature, toLatLng } from "./geolocation-state";

describe("geolocation editing", () => {
  it("turns clicks into GeoJSON positions, longitude first, rounded", () => {
    expect(fromLatLng(-23.55050001, -46.63330004)).toEqual([-46.6333, -23.5505]);
    expect(toLatLng([-46.6333, -23.5505])).toEqual([-23.5505, -46.6333]);
    // a map panned past the antimeridian
    expect(fromLatLng(10, 190)).toEqual([-170, 10]);
  });

  it("adds shapes and removes them, leaving null when nothing is left", () => {
    const one = addGeometry(null, { type: "Point", coordinates: [1, 2] });
    expect(one?.features).toHaveLength(1);
    const two = addGeometry(one, { type: "LineString", coordinates: [[0, 0], [1, 1]] });
    expect(two?.features.map((f) => f.geometry.type)).toEqual(["Point", "LineString"]);
    expect(removeFeature(two, 0)?.features.map((f) => f.geometry.type)).toEqual(["LineString"]);
    expect(removeFeature(one, 0)).toBeNull();
  });

  it("ignores the second click of a double-click", () => {
    let d = appendVertex([], [0, 0]);
    d = appendVertex(d, [1, 1]);
    d = appendVertex(d, [1, 1]);
    expect(d).toEqual([[0, 0], [1, 1]]);
  });

  it("finishes a line at two vertices and a polygon at three, closing its ring", () => {
    expect(canFinish("line", [[0, 0]])).toBe(false);
    expect(draftGeometry("line", [[0, 0], [1, 1]])).toEqual({ type: "LineString", coordinates: [[0, 0], [1, 1]] });
    expect(draftGeometry("polygon", [[0, 0], [1, 1]])).toBeNull();
    const poly = addGeometry(null, draftGeometry("polygon", [[0, 0], [1, 0], [1, 1]])!);
    expect(poly?.features[0].geometry.coordinates).toEqual([[[0, 0], [1, 0], [1, 1], [0, 0]]]);
    expect(canFinish("point", [[0, 0], [1, 1]])).toBe(false);
  });
});
