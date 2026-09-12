import { describe, expect, it } from "vitest";
import { cellWidthClass, columnSlots, fieldSlots, formRows, packColumnLines, type LineCell } from "./form-layout";
import type { Field } from "../meta";

/** A line as "fieldname" per cell, or "-" for an alignment spacer of one slot. */
const cells = (line: LineCell[]) => line.map((c) => c.field?.fieldname ?? "-".repeat(c.slots));

const COLUMN = 2; // a column of a section split by a Column Break
const ROW = 4; // the only column of a section without a Column Break

describe("columnSlots", () => {
  it("gives the whole line to a lone column and half of it to each of two", () => {
    expect(columnSlots(1)).toBe(ROW);
    expect(columnSlots(2)).toBe(COLUMN);
    expect(columnSlots(3)).toBe(COLUMN);
  });
});

describe("fieldSlots", () => {
  it("sizes a field by its resolved width, clamped to the column", () => {
    expect(fieldSlots({ fieldtype: "Date" }, ROW)).toBe(1);
    expect(fieldSlots({ fieldtype: "Currency" }, ROW)).toBe(1);
    expect(fieldSlots({ fieldtype: "Data" }, ROW)).toBe(2);
    expect(fieldSlots({ fieldtype: "Small Text" }, ROW)).toBe(4);
    // a two-slot column can hold no more than a full column
    expect(fieldSlots({ fieldtype: "Data" }, COLUMN)).toBe(2);
    expect(fieldSlots({ fieldtype: "Small Text" }, COLUMN)).toBe(2);
  });
});

describe("cellWidthClass", () => {
  it("names the cell width as a fraction of its column", () => {
    expect(cellWidthClass(1, ROW)).toBe("w-25");
    expect(cellWidthClass(2, ROW)).toBe("w-50");
    expect(cellWidthClass(4, ROW)).toBe("");
    expect(cellWidthClass(1, COLUMN)).toBe("w-50");
    expect(cellWidthClass(2, COLUMN)).toBe("");
  });
});

describe("packColumnLines in a column of a multi-column section", () => {
  it("puts full-width fields on their own lines", () => {
    const fields: Field[] = [
      { fieldname: "f1", fieldtype: "Data" },
      { fieldname: "f2", fieldtype: "Select" },
    ];
    const lines = packColumnLines(fields, undefined, COLUMN);
    expect(lines.map(cells)).toEqual([["f1"], ["f2"]]);
  });

  it("pairs consecutive half-width fields into a single line", () => {
    const fields: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
    ];
    const lines = packColumnLines(fields, undefined, COLUMN);
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
    const lines = packColumnLines(fields, undefined, COLUMN);
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
    const lines = packColumnLines(fields, undefined, COLUMN);
    expect(lines.map(cells)).toEqual([["count"], ["title"]]);
  });

  it("filters out hidden fields and dynamic invisible fields", () => {
    const fields: Field[] = [
      { fieldname: "f1", fieldtype: "Date" },
      { fieldname: "hidden1", fieldtype: "Date", hidden: true },
      { fieldname: "f2", fieldtype: "Date" },
      { fieldname: "invisible", fieldtype: "Date" },
    ];
    const lines = packColumnLines(fields, (f) => f.fieldname !== "invisible", COLUMN);
    expect(lines.map(cells)).toEqual([["f1", "f2"]]);
  });
});

describe("packColumnLines in a single-column section", () => {
  it("pairs default fields at half a line", () => {
    const fields: Field[] = [
      { fieldname: "imovel", fieldtype: "Link" },
      { fieldname: "proprietario", fieldtype: "Link" },
      { fieldname: "locatario", fieldtype: "Link" },
    ];
    expect(packColumnLines(fields, undefined, ROW).map(cells)).toEqual([
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
    expect(packColumnLines(fields, undefined, ROW).map(cells)).toEqual([
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
    expect(packColumnLines(fields, undefined, ROW).map(cells)).toEqual([["situacao", "inicio", "termino"]]);
  });

  it("keeps a full-width field on its own line", () => {
    const fields: Field[] = [
      { fieldname: "title", fieldtype: "Data" },
      { fieldname: "notes", fieldtype: "Small Text" },
      { fieldname: "code", fieldtype: "Data" },
    ];
    expect(packColumnLines(fields, undefined, ROW).map(cells)).toEqual([["title"], ["notes"], ["code"]]);
  });

  it("pads so a half-line field never starts in the middle of a quarter", () => {
    const fields: Field[] = [
      { fieldname: "prazo", fieldtype: "Int" },
      { fieldname: "renovacao", fieldtype: "Check" },
    ];
    expect(packColumnLines(fields, undefined, ROW).map(cells)).toEqual([["prazo", "-", "renovacao"]]);
  });

  it("moves a half-line field to the next line when the padding no longer fits", () => {
    const fields: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
      { fieldname: "d3", fieldtype: "Date" },
      { fieldname: "titulo", fieldtype: "Data" },
    ];
    expect(packColumnLines(fields, undefined, ROW).map(cells)).toEqual([["d1", "d2", "d3"], ["titulo"]]);
  });
});

describe("formRows", () => {
  it("aligns lines across columns into horizontal rows", () => {
    const col0: Field[] = [
      { fieldname: "imovel", fieldtype: "Link" },
      { fieldname: "proprietario", fieldtype: "Link" },
    ];
    const col1: Field[] = [
      { fieldname: "situacao", fieldtype: "Select" },
      { fieldname: "inicio", fieldtype: "Date" },
      { fieldname: "termino", fieldtype: "Date" },
    ];

    const rows = formRows([col0, col1], undefined, columnSlots(2));
    expect(rows).toHaveLength(2);
    expect(rows.map((row) => row.map(cells))).toEqual([
      [["imovel"], ["situacao"]],
      [["proprietario"], ["inicio", "termino"]],
    ]);
  });

  it("handles uneven columns by filling shorter columns with empty arrays", () => {
    const col0: Field[] = [{ fieldname: "a", fieldtype: "Data" }];
    const col1: Field[] = [
      { fieldname: "b", fieldtype: "Data" },
      { fieldname: "c", fieldtype: "Data" },
    ];

    const rows = formRows([col0, col1], undefined, columnSlots(2));
    expect(rows.map((row) => row.map(cells))).toEqual([
      [["a"], ["b"]],
      [[], ["c"]],
    ]);
  });

  it("gives the whole line to a single column", () => {
    const col0: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
      { fieldname: "text", fieldtype: "Data" },
    ];

    const rows = formRows([col0], undefined, columnSlots(1));
    expect(rows.map((row) => row.map(cells))).toEqual([[["d1", "d2", "text"]]]);
  });
});
