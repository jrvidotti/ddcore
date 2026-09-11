import { describe, it, expect } from "vitest";
import { round, type RoundingMode } from "./round";
// The contract itself, not a fixture: internal/num (Go) and
// internal/js/prelude.js assert this same file.
import table from "../../../internal/num/testdata/rounding.json";

const vectors = table.cases as { why?: string; v: number; p: number; commercial: number; bankers: number }[];

describe("round", () => {
  it("has vectors to check", () => {
    expect(vectors.length).toBeGreaterThan(20);
  });

  for (const mode of ["commercial", "bankers"] as RoundingMode[]) {
    it(`matches the shared vectors (${mode})`, () => {
      for (const c of vectors) {
        const want = mode === "bankers" ? c.bankers : c.commercial;
        expect(round(c.v, c.p, mode), `${c.v} @${c.p} — ${c.why ?? ""}`).toBe(want);
      }
    });
  }

  it("never returns a negative zero", () => {
    expect(Object.is(round(-0.001, 2), -0)).toBe(false);
    expect(Object.is(round(-0.4, 0), -0)).toBe(false);
  });

  it("is idempotent", () => {
    for (const c of vectors) {
      const once = round(c.v, c.p);
      expect(round(once, c.p)).toBe(once);
    }
  });

  it("leaves alone what it cannot round", () => {
    expect(round(NaN, 2)).toBeNaN();
    expect(round(Infinity, 2)).toBe(Infinity);
    expect(round(1.005, -1)).toBe(1.005);
  });
});
