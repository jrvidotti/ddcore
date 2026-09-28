// Text helpers shared by the search palette and the controls that filter as
// the user types.

/** Lowercases and strips accents, as the server's search does. */
export function fold(s: string): string {
  return (s || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
}
