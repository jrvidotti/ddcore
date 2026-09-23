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

/** The avatar colour of a title, the same every time for the same title. */
export function avatarColor(title: string): string {
  return hashColor(String(title ?? ""));
}
