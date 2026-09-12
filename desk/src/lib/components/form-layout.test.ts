import { describe, expect, it } from "vitest";
import { fieldsByRow, packColumnLines, formRows } from "./form-layout";
import type { Field } from "../meta";

describe("fieldsByRow", () => {
  it("keeps fields with the same position in their declared columns on one visual row", () => {
    expect(fieldsByRow([
      ["cep", "logradouro", "numero", "complemento"],
      ["bairro", "municipio", "estado"],
    ])).toEqual([
      ["cep", "bairro"],
      ["logradouro", "municipio"],
      ["numero", "estado"],
      ["complemento", undefined],
    ]);
  });

  it("preserves every declared column when a middle column is shorter", () => {
    expect(fieldsByRow([
      ["a", "b"],
      ["c"],
      ["d", "e", "f"],
    ])).toEqual([
      ["a", "c", "d"],
      ["b", undefined, "e"],
      [undefined, undefined, "f"],
    ]);
  });

  it("does not let statically hidden fields consume the first visual row", () => {
    expect(fieldsByRow([
      [{ name: "series", hidden: true }, { name: "tipo" }],
      [{ name: "email" }],
    ])).toEqual([
      [{ name: "tipo" }, { name: "email" }],
    ]);
  });
});

describe("packColumnLines", () => {
  it("puts full-width fields on their own lines", () => {
    const fields: Field[] = [
      { fieldname: "f1", fieldtype: "Data" },
      { fieldname: "f2", fieldtype: "Select" },
    ];
    const lines = packColumnLines(fields);
    expect(lines).toHaveLength(2);
    expect(lines[0]).toEqual([fields[0]]);
    expect(lines[1]).toEqual([fields[1]]);
  });

  it("pairs consecutive half-width fields into a single line", () => {
    const fields: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
    ];
    const lines = packColumnLines(fields);
    expect(lines).toHaveLength(1);
    expect(lines[0]).toEqual([fields[0], fields[1]]);
  });

  it("handles a mix of full-width and half-width fields", () => {
    const fields: Field[] = [
      { fieldname: "title", fieldtype: "Data" },
      { fieldname: "start", fieldtype: "Date" },
      { fieldname: "end", fieldtype: "Date" },
      { fieldname: "count", fieldtype: "Int" },
      { fieldname: "notes", fieldtype: "Small Text" },
    ];
    const lines = packColumnLines(fields);
    expect(lines).toHaveLength(4);
    expect(lines[0]).toEqual([fields[0]]); // title (100%)
    expect(lines[1]).toEqual([fields[1], fields[2]]); // start + end (50% + 50%)
    expect(lines[2]).toEqual([fields[3]]); // count (50%, followed by full)
    expect(lines[3]).toEqual([fields[4]]); // notes (100%)
  });

  it("filters out hidden fields and dynamic invisible fields", () => {
    const fields: Field[] = [
      { fieldname: "f1", fieldtype: "Date" },
      { fieldname: "hidden1", fieldtype: "Date", hidden: true },
      { fieldname: "f2", fieldtype: "Date" },
      { fieldname: "invisible", fieldtype: "Date" },
    ];
    const lines = packColumnLines(fields, (f) => f.fieldname !== "invisible");
    expect(lines).toHaveLength(1);
    expect(lines[0]).toEqual([fields[0], fields[2]]);
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

    const rows = formRows([col0, col1]);
    expect(rows).toHaveLength(2);

    // Row 0: [imovel] in col 0, [situacao] in col 1
    expect(rows[0][0]).toEqual([col0[0]]);
    expect(rows[0][1]).toEqual([col1[0]]);

    // Row 1: [proprietario] in col 0, [inicio, termino] in col 1
    expect(rows[1][0]).toEqual([col0[1]]);
    expect(rows[1][1]).toEqual([col1[1], col1[2]]);
  });

  it("handles uneven columns by filling shorter columns with empty arrays", () => {
    const col0: Field[] = [
      { fieldname: "a", fieldtype: "Data" },
    ];
    const col1: Field[] = [
      { fieldname: "b", fieldtype: "Data" },
      { fieldname: "c", fieldtype: "Data" },
    ];

    const rows = formRows([col0, col1]);
    expect(rows).toHaveLength(2);
    expect(rows[0][0]).toEqual([col0[0]]);
    expect(rows[0][1]).toEqual([col1[0]]);
    expect(rows[1][0]).toEqual([]);
    expect(rows[1][1]).toEqual([col1[1]]);
  });

  it("handles single-column layouts seamlessly", () => {
    const col0: Field[] = [
      { fieldname: "d1", fieldtype: "Date" },
      { fieldname: "d2", fieldtype: "Date" },
      { fieldname: "text", fieldtype: "Data" },
    ];

    const rows = formRows([col0]);
    expect(rows).toHaveLength(2);
    expect(rows[0][0]).toEqual([col0[0], col0[1]]);
    expect(rows[1][0]).toEqual([col0[2]]);
  });
});
