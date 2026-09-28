// Signature: the pure half of the drawing pad. The server stores a PNG data
// URL and checks it again (internal/signature); these decide what the form
// shows and what it sends.

export const SIGNATURE_PREFIX = "data:image/png;base64,";

/** The largest image the server accepts, in pixels. */
export const MAX_WIDTH = 2000;
export const MAX_HEIGHT = 1000;

/**
 * Whether a value may be put in an <img src>: a PNG data URL and nothing else,
 * so a value that reached the document some other way never becomes a script
 * URL or a request to another site.
 */
export function isSignatureDataUrl(v: unknown): v is string {
  if (typeof v !== "string" || !v.startsWith(SIGNATURE_PREFIX)) return false;
  const body = v.slice(SIGNATURE_PREFIX.length);
  return body.length > 0 && /^[A-Za-z0-9+/]+={0,2}$/.test(body);
}

export interface Bounds { x: number; y: number; width: number; height: number }

/**
 * The box around every pixel that is not fully transparent, or null for an
 * empty pad. It is what the stored image is cropped to, so a signature is as
 * small as its strokes and not the size of the pad.
 */
export function trimBounds(img: { width: number; height: number; data: ArrayLike<number> }): Bounds | null {
  const { width, height, data } = img;
  let minX = width, minY = height, maxX = -1, maxY = -1;
  for (let y = 0; y < height; y++) {
    const row = y * width * 4;
    for (let x = 0; x < width; x++) {
      if (data[row + x * 4 + 3] === 0) continue;
      if (x < minX) minX = x;
      if (x > maxX) maxX = x;
      if (y < minY) minY = y;
      if (y > maxY) maxY = y;
    }
  }
  if (maxX < 0) return null;
  return { x: minX, y: minY, width: maxX - minX + 1, height: maxY - minY + 1 };
}

/** Grows b by pad on each side, without leaving a w×h canvas. */
export function padBounds(b: Bounds, pad: number, w: number, h: number): Bounds {
  const x = Math.max(0, b.x - pad), y = Math.max(0, b.y - pad);
  return { x, y, width: Math.min(w, b.x + b.width + pad) - x, height: Math.min(h, b.y + b.height + pad) - y };
}

/** The size the crop is drawn at: its own, or scaled down to fit the server's limits. */
export function fitSize(width: number, height: number): { width: number; height: number } {
  const scale = Math.min(1, MAX_WIDTH / width, MAX_HEIGHT / height);
  return { width: Math.max(1, Math.floor(width * scale)), height: Math.max(1, Math.floor(height * scale)) };
}
