/**
 * The desk's half of the rich-text contract. The server is the guarantee —
 * every write goes through `internal/richtext` — and this is what keeps a value
 * the server never saw from reaching an `{@html}`: a row written by
 * `ddcore.db.sql`, restored from an older archive, or put there before this
 * release.
 *
 * The allowlist mirrors `internal/richtext/richtext.go`, and
 * `internal/richtext/testdata/cases.json` is run against both.
 */
import DOMPurify from "dompurify";

const TAGS = [
  "p", "br", "hr", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6",
  "strong", "b", "em", "i", "u", "s", "del", "code", "pre", "ul", "ol", "li",
  "a", "img", "table", "thead", "tbody", "tr", "th", "td",
];
const ATTRS = ["href", "title", "target", "rel", "src", "alt", "width", "height", "start", "class", "align"];

/** An image is a file this site serves; see the Go side for why. */
const IMAGE_SRC = /^\/(private\/)?files\/[A-Za-z0-9_%+-][A-Za-z0-9._%+-]*(\/[A-Za-z0-9_%+-][A-Za-z0-9._%+-]*)*$/;

let hooked = false;
function purifier() {
  if (!hooked) {
    DOMPurify.addHook("afterSanitizeAttributes", (node: any) => {
      if (node.tagName === "IMG" && !IMAGE_SRC.test(node.getAttribute("src") || "")) {
        node.removeAttribute("src");
      }
      if (node.tagName === "CODE") {
        const cls = node.getAttribute("class") || "";
        if (cls && !/^language-[A-Za-z0-9+#_.-]{1,20}$/.test(cls)) node.removeAttribute("class");
      } else if (node.tagName !== "TH" && node.tagName !== "TD" && node.hasAttribute?.("class")) {
        node.removeAttribute("class");
      }
    });
    hooked = true;
  }
  return DOMPurify;
}

/** Cleans markup before it is rendered with `{@html}`. */
export function sanitizeHtml(html: string): string {
  return purifier().sanitize(html ?? "", {
    ALLOWED_TAGS: TAGS,
    ALLOWED_ATTR: ATTRS,
    ALLOWED_URI_REGEXP: /^(?:https?|mailto|tel):|^[^a-z]|^[a-z+.-]+(?:[^a-z+.:-]|$)/i,
  });
}

// A value is markup when it carries a closing tag, or a void element with an
// attribute — which is everything an editor or this code writes. A lone
// opening tag is not enough: `compare a<b and b>c` has one, and reading that
// sentence as markup deletes the words between the brackets. Mirrors
// LooksLikeHTML in internal/richtext.
const CLOSING_TAG = /<\/(p|div|blockquote|h[1-6]|strong|b|em|i|u|s|del|code|pre|ul|ol|li|a|table|thead|tbody|tr|th|td|span)\s*>/i;
const VOID_TAG = /<(br|hr)\s*\/?>|<img\s[^<>]*=[^<>]*>/i;

/** Whether a stored value was written as markup, and not as plain text. */
export const looksLikeHtml = (s: string) => CLOSING_TAG.test(s ?? "") || VOID_TAG.test(s ?? "");

const escapeText = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

/** Plain text as paragraphs: a blank line separates, a newline breaks. */
export function fromPlainText(s: string): string {
  return (s ?? "")
    .replace(/\r\n?/g, "\n")
    .split("\n\n")
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) => `<p>${escapeText(p).replace(/\n/g, "<br>")}</p>`)
    .join("");
}

/**
 * What a Text Editor value reads as. A value written before rich text existed
 * is plain text, and reading it as markup would delete a `<` and the rest of
 * the line with it.
 */
export function normalizeRichText(s: string): string {
  if (!s) return "";
  if (!looksLikeHtml(s)) return sanitizeHtml(fromPlainText(s));
  const out = sanitizeHtml(s);
  // cleaning can leave text with no block around it, and the next pass would
  // then read that text as plain and escape its entities again
  return out && !BLOCK_LEVEL.test(out) ? `<p>${out}</p>` : out;
}

const BLOCK_LEVEL = /<(p|h[1-6]|ul|ol|li|blockquote|pre|hr|table)(\s[^<>]*)?\/?>/i;

/** True when cleaned markup shows nothing: what an emptied editor leaves. */
export function isEmptyRichText(s: string): boolean {
  if (/<img/i.test(s ?? "")) return false;
  return htmlToText(s ?? "").trim() === "";
}

const ENTITIES: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', "#39": "'", nbsp: " " };

/**
 * Markup reduced to readable text, for a list cell, a child-table cell and a
 * version diff. It uses no DOM, so list code runs the same in a test.
 */
export function htmlToText(html: string): string {
  return (html ?? "")
    .replace(/<\/(p|div|h[1-6]|li|tr|blockquote|pre)>|<br\s*\/?>/gi, "\n")
    .replace(/<[^>]*>/g, "")
    .replace(/&(#?[a-z0-9]+);/gi, (m, e) => ENTITIES[String(e).toLowerCase()] ?? m)
    .replace(/[ \t]+/g, " ")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

/** A list cell: one line, however many paragraphs the value has. */
export const htmlToLine = (html: string) => htmlToText(html).replace(/\n+/g, " ");
