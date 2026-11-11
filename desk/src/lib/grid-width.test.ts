import { describe, expect, it } from "vitest";
import { fitColumnWidth, inlineGridMinWidth, numericColumnMin, splitLastWord } from "./grid-width";

describe("inlineGridMinWidth", () => {
  it("keeps the average of 88px a column when no number needs more", () => {
    const cols = [{ fieldname: "a", fieldtype: "Link", columns: 4 }, { fieldname: "b", fieldtype: "Data", columns: 2 }] as any;
    expect(inlineGridMinWidth(cols, 72)).toBe(2 * 88 + 72);
  });

  it("widens the table until a narrow currency column holds its value", () => {
    // 1 of 11 columns: the currency's share of what the fixed 72px leave must be 104px (precision 0)
    const cols = [
      { fieldname: "g", fieldtype: "Link", columns: 4 },
      ...Array.from({ length: 7 }, (_, i) => ({ fieldname: `c${i}`, fieldtype: "Currency", precision: 0, columns: 1 })),
    ] as any;
    expect(inlineGridMinWidth(cols, 72)).toBe(104 * 11 + 72);
  });

  it("gives a currency with cents more room than one without", () => {
    expect(numericColumnMin({ fieldname: "x", fieldtype: "Currency", precision: 0 } as any)).toBe(104);
    expect(numericColumnMin({ fieldname: "x", fieldtype: "Currency" } as any)).toBe(124);
    expect(numericColumnMin({ fieldname: "x", fieldtype: "Data" } as any)).toBeUndefined();
  });
});

describe("fit layout", () => {
  it("leaves a read-only column to its values and keeps room for an input", () => {
    expect(fitColumnWidth({ fieldname: "x", fieldtype: "Currency", precision: 0 } as any, false)).toBeUndefined();
    expect(fitColumnWidth({ fieldname: "x", fieldtype: "Currency", precision: 0 } as any, true)).toBe(104);
    expect(fitColumnWidth({ fieldname: "x", fieldtype: "Link" } as any, true)).toBe(160);
    expect(fitColumnWidth({ fieldname: "x", fieldtype: "Data", gridMaxWidth: 120 } as any, true)).toBe(120);
  });

  it("splits a label before its last word", () => {
    expect(splitLastWord("Plan. Sales")).toEqual(["Plan. ", "Sales"]);
    expect(splitLastWord("OTB")).toEqual(["", "OTB"]);
    expect(splitLastWord("")).toEqual(["", ""]);
  });
});

describe("a Currency column shown in a larger scale", () => {
  it("reserves the room of the shorter number", () => {
    expect(numericColumnMin({ fieldtype: "Currency", precision: 0 } as any)).toBe(104);
    expect(numericColumnMin({ fieldtype: "Currency", precision: 0, displayScale: 1000 } as any)).toBe(88);
    expect(numericColumnMin({ fieldtype: "Currency", precision: 0, displayScale: 1 } as any)).toBe(104);
  });
});
