/**
 * The desk's copy of the framework's rounding rule.
 *
 * Shared with internal/num (Go, which rounds on every write and is therefore
 * authoritative) and internal/js/prelude.js (the app runtime). All three are
 * asserted against internal/num/testdata/rounding.json: two copies of a table
 * drift, and the day they drift is the day a form and the database disagree
 * about a cent.
 *
 * The rounding is defined over the number's shortest decimal representation —
 * the digits String(n) prints — and not over the binary double, because
 * someone who types 1.005 stores 1.00499999999999989 and expects 1.01 back.
 */

export type RoundingMode = "commercial" | "bankers";

/**
 * The number's shortest decimal representation, as [negative, int, frac].
 *
 * String(n) is that representation already, except that JS switches to
 * exponent notation at 1e21 and below 1e-6 where Go's %f never does. Those
 * magnitudes are past anything a money column holds, so they are expanded
 * rather than handled: the answer has to be the same in both languages.
 */
function decimalDigits(n: number): [boolean, string, string] {
  let s = String(n);
  const neg = s[0] === "-";
  if (neg) s = s.slice(1);
  const e = s.indexOf("e");
  if (e >= 0) {
    const exp = Number(s.slice(e + 1));
    const [i, f = ""] = s.slice(0, e).split(".");
    if (exp >= 0) {
      const pad = exp - f.length;
      s = pad >= 0 ? i + f + "0".repeat(pad) : i + f.slice(0, exp) + "." + f.slice(exp);
    } else {
      s = "0." + "0".repeat(-exp - i.length) + i + f;
    }
  }
  const dot = s.indexOf(".");
  return dot < 0 ? [neg, s, ""] : [neg, s.slice(0, dot), s.slice(dot + 1)];
}

/**
 * Decides on the discarded digits alone: a leading digit above or below 5
 * settles it, and a leading 5 with anything non-zero after it is above half,
 * not at it.
 */
function roundsUp(kept: string, rest: string, mode: RoundingMode): boolean {
  if (!rest || rest[0] < "5") return false;
  if (rest[0] > "5" || /[1-9]/.test(rest.slice(1))) return true;
  if (mode === "bankers") return (kept.charCodeAt(kept.length - 1) - 48) % 2 === 1;
  return true;
}

function increment(d: string): string {
  const b = d.split("");
  for (let i = b.length - 1; i >= 0; i--) {
    if (b[i] !== "9") {
      b[i] = String.fromCharCode(b[i].charCodeAt(0) + 1);
      return b.join("");
    }
    b[i] = "0";
  }
  return "1" + b.join("");
}

function insertPoint(d: string, p: number): string {
  if (p === 0) return d;
  while (d.length <= p) d = "0" + d;
  return d.slice(0, d.length - p) + "." + d.slice(d.length - p);
}

/** Rounds n to p decimal places. Anything it cannot round it returns untouched. */
export function round(n: number, p: number, mode: RoundingMode = "commercial"): number {
  if (typeof n !== "number" || !isFinite(n) || p < 0) return n;
  const [neg, intPart, frac] = decimalDigits(n);
  if (frac.length <= p) return n;
  let kept = intPart + frac.slice(0, p);
  const rest = frac.slice(p);
  if (roundsUp(kept, rest, mode)) kept = increment(kept);
  const v = Number((neg ? "-" : "") + insertPoint(kept, p));
  return v === 0 ? 0 : v; // never hand back a negative zero
}
