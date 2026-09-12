import { describe, expect, it } from "vitest";
import { cellWidthClass, fieldSlots, LINE_SLOTS, packLines, type LineCell } from "./form-layout";
import type { Field } from "../meta";

/** A line as "fieldname" per cell, or "-" for an alignment spacer of one slot. */
const cells = (line: LineCell[]) => line.map((c) => c.field?.fieldname ?? "-".repeat(c.slots));

const ROW = LINE_SLOTS; // a form line
const NARROW = LINE_SLOTS / 2; // a dialog line, half as wide

describe("fieldSlots", () => {
  it("sizes a field by its resolved width, clamped to the line", () => {
    expect(fieldSlots({ fieldtype: "Date" }, ROW)).toBe(1);
    expect(fieldSlots({ fieldtype: "Currency" }, ROW)).toBe(1);
    expect(fieldSlots({ fieldtype: "Data" }, ROW)).toBe(2);
    expect(fieldSlots({ fieldtype: "Small Text" }, ROW)).toBe(4);
    // a narrow line can hold no more than one full-width field
    expect(fieldSlots({ fieldtype: "Data" }, NARROW)).toBe(2);
    expect(fieldSlots({ fieldtype: "Small Text" }, NARROW)).toBe(2);
  });
});

describe("cellWidthClass", () => {
  it("names the cell width as a fraction of its line", () => {
    expect(cellWidthClass(1, ROW)).toBe("w-25");
    expect(cellWidthClass(2, ROW)).toBe("w-50");
    expect(cellWidthClass(4, ROW)).toBe("");
    expect(cellWidthClass(1, NARROW)).toBe("w-50");
    expect(cellWidthClass(2, NARROW)).toBe("");
  });
});

describe("packLines on a narrow (dialog) line", () => {
  it("puts full-width fields on their own lines", () => {
    const fields: Field[] = [
      { fieldname: "f1", fieldtype: "Data" },
      { fieldname: "f2", fieldtype: "Select" },
    ];
    const lines = packLines(fields, undefined, NARROW);
    expect(lines.map(cells)).toEqual([["f1"], ["f2"]]);
  });

  it("pairs consecutive half-width fields into a single line", () => {
    const fields: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
    ];
    const lines = packLines(fields, undefined, NARROW);
    expect(lines.map(cells)).toEqual([["d1", "d2"]]);
  });

  it("handles a mix of full-width and half-width fields", () => {
    const fields: Field[] = [
      { fieldname: "title", fieldtype: "Data" },
      { fieldname: "start", fieldtype: "Date" },
      { fieldname: "end", fieldtype: "Date" },
      { fieldname: "count", fieldtype: "Int" },
      { fieldname: "notes", fieldtype: "Small Text" },
    ];
    const lines = packLines(fields, undefined, NARROW);
    expect(lines.map(cells)).toEqual([
      ["title"], // 100% of the column
      ["start", "end"], // 50% + 50%
      ["count"], // 50%, followed by a full-width field
      ["notes"],
    ]);
  });

  it("never needs an alignment spacer", () => {
    const fields: Field[] = [
      { fieldname: "count", fieldtype: "Int" },
      { fieldname: "title", fieldtype: "Data" },
    ];
    const lines = packLines(fields, undefined, NARROW);
    expect(lines.map(cells)).toEqual([["count"], ["title"]]);
  });

  it("filters out hidden fields and dynamic invisible fields", () => {
    const fields: Field[] = [
      { fieldname: "f1", fieldtype: "Date" },
      { fieldname: "hidden1", fieldtype: "Date", hidden: true },
      { fieldname: "f2", fieldtype: "Date" },
      { fieldname: "invisible", fieldtype: "Date" },
    ];
    const lines = packLines(fields, (f) => f.fieldname !== "invisible", NARROW);
    expect(lines.map(cells)).toEqual([["f1", "f2"]]);
  });
});

describe("packLines on a form line", () => {
  it("pairs default fields at half a line", () => {
    const fields: Field[] = [
      { fieldname: "imovel", fieldtype: "Link" },
      { fieldname: "proprietario", fieldtype: "Link" },
      { fieldname: "locatario", fieldtype: "Link" },
    ];
    expect(packLines(fields, undefined, ROW).map(cells)).toEqual([
      ["imovel", "proprietario"],
      ["locatario"],
    ]);
  });

  it("fits four quarter-line fields on one line", () => {
    const fields: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
      { fieldname: "n1", fieldtype: "Int" },
      { fieldname: "v1", fieldtype: "Currency" },
      { fieldname: "d3", fieldtype: "Date" },
    ];
    expect(packLines(fields, undefined, ROW).map(cells)).toEqual([
      ["d1", "d2", "n1", "v1"],
      ["d3"],
    ]);
  });

  it("fills a line greedily with mixed widths", () => {
    const fields: Field[] = [
      { fieldname: "situacao", fieldtype: "Select" },
      { fieldname: "inicio", fieldtype: "Date" },
      { fieldname: "termino", fieldtype: "Date" },
    ];
    expect(packLines(fields, undefined, ROW).map(cells)).toEqual([["situacao", "inicio", "termino"]]);
  });

  it("keeps a full-width field on its own line", () => {
    const fields: Field[] = [
      { fieldname: "title", fieldtype: "Data" },
      { fieldname: "notes", fieldtype: "Small Text" },
      { fieldname: "code", fieldtype: "Data" },
    ];
    expect(packLines(fields, undefined, ROW).map(cells)).toEqual([["title"], ["notes"], ["code"]]);
  });

  it("pads so a half-line field never starts in the middle of a quarter", () => {
    const fields: Field[] = [
      { fieldname: "prazo", fieldtype: "Int" },
      { fieldname: "renovacao", fieldtype: "Check" },
    ];
    expect(packLines(fields, undefined, ROW).map(cells)).toEqual([["prazo", "-", "renovacao"]]);
  });

  it("moves a half-line field to the next line when the padding no longer fits", () => {
    const fields: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
      { fieldname: "d3", fieldtype: "Date" },
      { fieldname: "titulo", fieldtype: "Data" },
    ];
    expect(packLines(fields, undefined, ROW).map(cells)).toEqual([["d1", "d2", "d3"], ["titulo"]]);
  });
});
