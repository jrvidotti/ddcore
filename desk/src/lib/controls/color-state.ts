/**
 * A Color is stored as lowercase `#rrggbb`. The same normalisation runs on the
 * server (`normalizeColor` in internal/engine/doc.go): two spellings of one
 * colour must be one value in a filter and one swatch on screen.
 */
const HEX = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i;

export function normalizeColor(v: any): string | null {
  const m = String(v ?? "").trim().toLowerCase().match(HEX);
  if (!m) return null;
  const h = m[1];
  return "#" + (h.length === 3 ? h[0] + h[0] + h[1] + h[1] + h[2] + h[2] : h);
}

/** Whether a colour is dark enough to need light text on top of it. */
export function isDarkColor(v: any): boolean {
  const hex = normalizeColor(v);
  if (!hex) return false;
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  // perceived luminance, the usual sRGB weights
  return (0.299 * r + 0.587 * g + 0.114 * b) / 255 < 0.6;
}

/**
 * The Basic tab of the picker: ten hues across, light to dark down, with a
 * grey column first. Read row by row, ten to a row.
 */
export const BASIC_COLORS: string[] = [
  // grey     red        orange     amber      yellow     green      teal       blue       indigo     pink
  "#ffffff", "#fecaca", "#fed7aa", "#fde68a", "#fef08a", "#bbf7d0", "#99f6e4", "#bfdbfe", "#c7d2fe", "#fbcfe8",
  "#d1d5db", "#f87171", "#fb923c", "#fbbf24", "#facc15", "#4ade80", "#2dd4bf", "#60a5fa", "#818cf8", "#f472b6",
  "#6b7280", "#dc2626", "#ea580c", "#d97706", "#ca8a04", "#16a34a", "#0d9488", "#2563eb", "#4f46e5", "#db2777",
  "#000000", "#991b1b", "#9a3412", "#92400e", "#854d0e", "#166534", "#115e59", "#1e40af", "#3730a3", "#9d174d",
];

export interface Rgb { r: number; g: number; b: number }
/** Hue in degrees [0, 360), saturation and value in [0, 1]. */
export interface Hsv { h: number; s: number; v: number }

export function hexToRgb(v: any): Rgb | null {
  const hex = normalizeColor(v);
  if (!hex) return null;
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  return { r, g, b };
}

export function rgbToHex({ r, g, b }: Rgb): string {
  const byte = (n: number) => Math.round(Math.min(255, Math.max(0, n || 0))).toString(16).padStart(2, "0");
  return "#" + byte(r) + byte(g) + byte(b);
}

export function rgbToHsv({ r, g, b }: Rgb): Hsv {
  const [rr, gg, bb] = [r / 255, g / 255, b / 255];
  const max = Math.max(rr, gg, bb);
  const d = max - Math.min(rr, gg, bb);
  let h = 0;
  if (d) {
    if (max === rr) h = ((gg - bb) / d) % 6;
    else if (max === gg) h = (bb - rr) / d + 2;
    else h = (rr - gg) / d + 4;
    h = (h * 60 + 360) % 360;
  }
  return { h, s: max ? d / max : 0, v: max };
}

export function hsvToRgb({ h, s, v }: Hsv): Rgb {
  const f = (n: number) => {
    const k = (n + h / 60) % 6;
    return Math.round(255 * (v - v * s * Math.max(0, Math.min(k, 4 - k, 1))));
  };
  return { r: f(5), g: f(3), b: f(1) };
}
