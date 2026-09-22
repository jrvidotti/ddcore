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
