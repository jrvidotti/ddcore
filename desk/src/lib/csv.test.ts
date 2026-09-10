import { describe, it, expect } from "vitest";
import { csvCell, toCsv } from "./csv";

describe("csvCell", () => {
  it("escapa aspas dobrando-as, e não como JSON", () => {
    expect(csvCell('Rua "do Meio"')).toBe('"Rua ""do Meio"""');
    // o defeito anterior: JSON.stringify produzia Rua \"do Meio\"
    expect(csvCell('Rua "do Meio"')).not.toContain("\\");
  });
  it("cita apenas quando precisa", () => {
    expect(csvCell("Simples")).toBe("Simples");
    expect(csvCell("a;b")).toBe('"a;b"');
    expect(csvCell("linha1\nlinha2")).toBe('"linha1\nlinha2"');
    expect(csvCell("a,b")).toBe("a,b"); // vírgula não é o separador
  });
  it("trata vazios e números sem aspas", () => {
    expect(csvCell(null)).toBe("");
    expect(csvCell(undefined)).toBe("");
    expect(csvCell(0)).toBe("0");
    expect(csvCell(1234.5)).toBe("1234.5");
    expect(csvCell(false)).toBe("false");
  });
  it("serializa objetos", () => {
    expect(csvCell({ a: 1 })).toBe('"{""a"":1}"');
  });
});

describe("toCsv", () => {
  it("monta cabeçalho e linhas separados por ; e CRLF", () => {
    const out = toCsv(["Nome", "Valor"], [["Ana", 10], ['B "x"', null]]);
    expect(out).toBe('Nome;Valor\r\nAna;10\r\n"B ""x""";');
  });
});
