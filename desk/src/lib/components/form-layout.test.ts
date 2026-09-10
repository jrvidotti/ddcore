import { describe, expect, it } from "vitest";
import { fieldsByRow } from "./form-layout";

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
