// A Geolocation field's value: GeoJSON, stored as a FeatureCollection. This
// mirrors internal/geo — both run internal/geo/testdata/cases.json — so what
// the map commits is what the server keeps, and an unchanged value saved
// again is equal to itself.
import { __ } from "./boot.svelte";

/** `[longitude, latitude]`: GeoJSON puts the longitude first. */
export type GeoPosition = [number, number];
export type GeoGeometry =
  | { type: "Point"; coordinates: GeoPosition }
  | { type: "MultiPoint"; coordinates: GeoPosition[] }
  | { type: "LineString"; coordinates: GeoPosition[] }
  | { type: "Polygon"; coordinates: GeoPosition[][] };
export interface GeoFeature { type: "Feature"; geometry: GeoGeometry; properties: Record<string, never> }
export interface GeoFeatureCollection { type: "FeatureCollection"; features: GeoFeature[] }

export const MAX_FEATURES = 500;
export const MAX_BYTES = 64 * 1024;

/** Why a value is not a Geolocation; `kind` matches the Go package's. */
export class GeoError extends Error {
  constructor(public kind: "format" | "geometry" | "coordinates" | "range" | "shape" | "tooMany" | "tooLarge") {
    super(`geolocation: ${kind}`);
  }
}

const isObject = (v: unknown): v is Record<string, any> => typeof v === "object" && v !== null && !Array.isArray(v);

/** 7 decimals, half away from zero as Go's math.Round, and never -0. */
export function roundCoord(n: number): number {
  const r = (Math.sign(n) * Math.round(Math.abs(n) * 1e7)) / 1e7;
  return r === 0 ? 0 : r;
}

function position(v: unknown): GeoPosition {
  if (!Array.isArray(v) || v.length < 2) throw new GeoError("coordinates");
  const [lon, lat] = v;
  if (typeof lon !== "number" || typeof lat !== "number" || !Number.isFinite(lon) || !Number.isFinite(lat)) {
    throw new GeoError("coordinates");
  }
  if (lon < -180 || lon > 180 || lat < -90 || lat > 90) throw new GeoError("range");
  return [roundCoord(lon), roundCoord(lat)];
}

function positions(v: unknown, min: number): GeoPosition[] {
  if (!Array.isArray(v)) throw new GeoError("coordinates");
  const out = v.map(position);
  if (out.length < min) throw new GeoError("shape");
  return out;
}

function polygon(v: unknown): GeoPosition[][] {
  if (!Array.isArray(v)) throw new GeoError("coordinates");
  if (v.length === 0) throw new GeoError("shape");
  return v.map((r) => {
    const ring = positions(r, 0);
    const first = ring[0], last = ring[ring.length - 1];
    if (ring.length > 0 && (first[0] !== last[0] || first[1] !== last[1])) ring.push([first[0], first[1]]);
    if (ring.length < 4) throw new GeoError("shape");
    return ring;
  });
}

function geometry(g: Record<string, any>): GeoGeometry {
  switch (g.type) {
    case "Point": return { type: "Point", coordinates: position(g.coordinates) };
    case "MultiPoint": return { type: "MultiPoint", coordinates: positions(g.coordinates, 1) };
    case "LineString": return { type: "LineString", coordinates: positions(g.coordinates, 2) };
    case "Polygon": return { type: "Polygon", coordinates: polygon(g.coordinates) };
  }
  throw new GeoError("geometry");
}

const wrap = (geometry: GeoGeometry): GeoFeature => ({ type: "Feature", geometry, properties: {} });

function feature(v: unknown): GeoFeature {
  if (!isObject(v) || v.type !== "Feature") throw new GeoError("format");
  if (!isObject(v.geometry)) throw new GeoError("geometry");
  return wrap(geometry(v.geometry));
}

/**
 * The value a Geolocation stores, or null when it holds nothing. Takes a JSON
 * string or a decoded value: a FeatureCollection, a Feature or a bare Point,
 * MultiPoint, LineString or Polygon. Throws a GeoError otherwise.
 */
export function normalizeGeo(v: unknown): GeoFeatureCollection | null {
  if (v === null || v === undefined) return null;
  if (typeof v === "string") {
    const s = v.trim();
    if (s === "" || s === "null") return null;
    try {
      v = JSON.parse(s);
    } catch {
      throw new GeoError("format");
    }
    if (v === null) return null;
  }
  if (!isObject(v)) throw new GeoError("format");
  let features: GeoFeature[];
  switch (v.type) {
    case "FeatureCollection": {
      if (v.features !== null && v.features !== undefined && !Array.isArray(v.features)) throw new GeoError("format");
      const list: unknown[] = v.features ?? [];
      if (list.length > MAX_FEATURES) throw new GeoError("tooMany");
      features = list.map(feature);
      break;
    }
    case "Feature":
      features = [feature(v)];
      break;
    case "Point": case "MultiPoint": case "LineString": case "Polygon":
      features = [wrap(geometry(v))];
      break;
    case "MultiLineString": case "MultiPolygon": case "GeometryCollection":
      throw new GeoError("geometry");
    default:
      throw new GeoError("format");
  }
  if (features.length === 0) return null;
  const fc: GeoFeatureCollection = { type: "FeatureCollection", features };
  if (new TextEncoder().encode(JSON.stringify(fc)).length > MAX_BYTES) throw new GeoError("tooLarge");
  return fc;
}

/** normalizeGeo, with a value it refuses read as nothing. */
export function readGeo(v: unknown): GeoFeatureCollection | null {
  try {
    return normalizeGeo(v);
  } catch {
    return null;
  }
}

export interface GeoSummary {
  points: number;
  lines: number;
  polygons: number;
  /** `[lon, lat]`, only when the value is one point and nothing else. */
  point: GeoPosition | null;
}

export function summarizeGeo(v: unknown): GeoSummary {
  const s: GeoSummary = { points: 0, lines: 0, polygons: 0, point: null };
  const fc = readGeo(v);
  if (!fc) return s;
  let only: GeoPosition | null = null;
  for (const f of fc.features) {
    const g = f.geometry;
    if (g.type === "Point") { s.points++; only = g.coordinates; }
    else if (g.type === "MultiPoint") { s.points += g.coordinates.length; if (g.coordinates.length === 1) only = g.coordinates[0]; }
    else if (g.type === "LineString") s.lines++;
    else s.polygons++;
  }
  if (s.points === 1 && s.lines === 0 && s.polygons === 0) s.point = only;
  return s;
}

/**
 * A Geolocation in words, as print says it: one point as "lat, lon" — the
 * order people read — or what the value holds, "2 points, 1 polygon".
 */
export function formatGeo(v: unknown): string {
  const s = summarizeGeo(v);
  if (s.point) return `${s.point[1].toFixed(5)}, ${s.point[0].toFixed(5)}`;
  // each key written out, so the catalogue extractor finds it
  const parts: string[] = [];
  if (s.points) parts.push(s.points === 1 ? __("1 point") : __("{0} points", [String(s.points)]));
  if (s.lines) parts.push(s.lines === 1 ? __("1 line") : __("{0} lines", [String(s.lines)]));
  if (s.polygons) parts.push(s.polygons === 1 ? __("1 polygon") : __("{0} polygons", [String(s.polygons)]));
  return parts.join(", ");
}

/**
 * A point typed latitude first: `lat; lon` always, `lat, lon` only when the
 * decimal separator is "." (with "," the comma is inside the numbers).
 */
export function fromLatLon(s: string, decimal = "."): GeoFeatureCollection | null {
  s = s.trim();
  if (s === "") return null;
  let parts: string[];
  if (s.includes(";")) parts = s.split(";");
  else if (decimal === "." || decimal === "") parts = s.split(",");
  else throw new GeoError("format");
  if (parts.length !== 2) throw new GeoError("format");
  const n = parts.map((p) => {
    p = p.trim();
    if (decimal === ",") p = p.replace(/,/g, ".");
    // Number("") is 0 and Number("0x1") is 1: only a plain decimal is a coordinate
    if (!/^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/.test(p)) throw new GeoError("format");
    return Number(p);
  });
  return normalizeGeo({ type: "Point", coordinates: [n[1], n[0]] });
}
