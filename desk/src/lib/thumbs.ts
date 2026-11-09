// The ?w= thumbnail of a public image (#154). The widths are the server's
// storage.ThumbWidths (internal/storage/storage.go); internal/js/prelude.js
// has the same function for server code.
export const THUMB_WIDTHS = [64, 128, 256, 512] as const;

/**
 * The URL of the smallest thumbnail at least `width` pixels wide (at most the
 * largest), for a public image; any other URL unchanged.
 */
export function thumbUrl(fileUrl: string | null | undefined, width: number): string {
  const n = Number(width);
  if (!(n > 0)) throw new Error("files.thumbUrl: width must be a positive number of pixels");
  const url = String(fileUrl ?? "");
  if (!/^\/files\/[^/?#]+\.(png|jpe?g|gif|webp)$/i.test(url)) return url;
  const w = THUMB_WIDTHS.find((t) => t >= n) ?? THUMB_WIDTHS[THUMB_WIDTHS.length - 1];
  return `${url}?w=${w}`;
}
