/**
 * A Rating is a whole number of stars, 0 to `options` (five by default). Zero
 * and "not rated" are different things: clearing the field stores null, so a
 * report can tell an unrated document from a bad one.
 */
export const DEFAULT_RATING_MAX = 5;

/** How many stars the field declares. Mirrors meta.Field.RatingMax in Go. */
export function ratingMax(options: any): number {
  const n = typeof options === "number" ? options : parseInt(String(options ?? ""), 10);
  return Number.isFinite(n) && n >= 1 && n <= 10 ? n : DEFAULT_RATING_MAX;
}

/** Clamps a rating to [0, max]. null or empty remains null (unrated). */
export function clampRating(value: any, max: number): number | null {
  if (value === null || value === undefined || value === "") return null;
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return null;
  return Math.max(0, Math.min(max, n));
}

/** What clicking star `star` does: picking the current one clears the field. */
export function ratingOnPick(current: any, star: number): number | null {
  return Number(current) === star ? null : star;
}

/** The keyboard: arrows move by one, Home and End go to the ends. */
export function ratingOnKey(current: any, key: string, max: number): number | null | undefined {
  const n = Number(current) || 0;
  switch (key) {
    case "ArrowRight":
    case "ArrowUp":
      return Math.min(max, n + 1);
    case "ArrowLeft":
    case "ArrowDown":
      return Math.max(0, n - 1);
    case "Home":
      return 0;
    case "End":
      return max;
    case "Delete":
    case "Backspace":
      return null;
  }
  return undefined; // not ours: let the browser have it
}

/** The stars to draw, filled up to the value. */
export const ratingStars = (value: any, max: number): boolean[] =>
  Array.from({ length: max }, (_, i) => i < (Number(value) || 0));
