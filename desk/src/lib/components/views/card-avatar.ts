import { hashColor } from "../../format";

/**
 * The initials of a card title: the first letter of its first and last words,
 * uppercase. Punctuation and spacing are ignored, and one word gives one letter.
 */
export function initials(title: string): string {
  const words = String(title ?? "").normalize("NFC").split(/[^\p{L}\p{M}\p{N}]+/u).filter(Boolean);
  if (!words.length) return "";
  const first = (word: string) => Array.from(word)[0].toUpperCase();
  return words.length === 1 ? first(words[0]) : first(words[0]) + first(words[words.length - 1]);
}

/** The letter in a person's avatar: their name or email's first, "U" for nobody. */
export function avatarInitial(user: string): string {
  return (user || "U").trim().charAt(0).toUpperCase();
}

/** The avatar colour of a title, the same every time for the same title. */
export function avatarColor(title: string): string {
  return hashColor(String(title ?? ""));
}
