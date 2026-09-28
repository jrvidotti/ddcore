// The editing a Geolocation map does, without the map: what a click adds,
// what a finished line or polygon becomes, what a deletion leaves. Every
// result goes through normalizeGeo, so the control commits what the server
// stores.
import { normalizeGeo, roundCoord, type GeoFeatureCollection, type GeoGeometry, type GeoPosition } from "$lib/geo";

export type GeoMode = "none" | "point" | "line" | "polygon" | "delete";

/** GeoJSON's [lon, lat] as Leaflet's [lat, lng]. */
export const toLatLng = (p: GeoPosition): [number, number] => [p[1], p[0]];

/** A click on the map, rounded as the server will round it. */
export const fromLatLng = (lat: number, lng: number): GeoPosition => [roundCoord(wrapLng(lng)), roundCoord(lat)];

/** A map panned across the antimeridian reports longitudes past ±180. */
function wrapLng(lng: number): number {
  if (lng >= -180 && lng <= 180) return lng;
  return ((((lng + 180) % 360) + 360) % 360) - 180;
}

/** The value with one more shape; null only if the result is empty. */
export function addGeometry(fc: GeoFeatureCollection | null, g: GeoGeometry): GeoFeatureCollection | null {
  return normalizeGeo({ type: "FeatureCollection", features: [...(fc?.features ?? []), { type: "Feature", geometry: g, properties: {} }] });
}

/** The value without feature i; null when nothing is left. */
export function removeFeature(fc: GeoFeatureCollection | null, i: number): GeoFeatureCollection | null {
  if (!fc) return null;
  return normalizeGeo({ type: "FeatureCollection", features: fc.features.filter((_, j) => j !== i) });
}

/**
 * The vertices after a click. A click on the vertex just placed adds nothing:
 * the two clicks of a double-click, which finishes the shape, land there.
 */
export function appendVertex(draft: GeoPosition[], p: GeoPosition): GeoPosition[] {
  const last = draft[draft.length - 1];
  if (last && last[0] === p[0] && last[1] === p[1]) return draft;
  return [...draft, p];
}

/** Whether the vertices drawn so far make a shape: 2 for a line, 3 for a polygon. */
export const canFinish = (mode: GeoMode, draft: GeoPosition[]): boolean =>
  (mode === "line" && draft.length >= 2) || (mode === "polygon" && draft.length >= 3);

/** The shape the vertices make, or null while they make none. */
export function draftGeometry(mode: GeoMode, draft: GeoPosition[]): GeoGeometry | null {
  if (!canFinish(mode, draft)) return null;
  if (mode === "line") return { type: "LineString", coordinates: draft };
  // normalizeGeo closes the ring
  return { type: "Polygon", coordinates: [draft] };
}
