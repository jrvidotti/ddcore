import { describe, it, expect } from "vitest";
import { csvCell, csvSep, toCsv } from "./csv";
import { useLocale } from "./locale.test";

describe("csvCell", () => {
  it("escapes quotes by doubling them, not as JSON", () => {
    useLocale("pt-BR");
    expect(csvCell('Rua "do Meio"')).toBe('"Rua ""do Meio"""');
    // the old defect: JSON.stringify produced Rua \"do Meio\"
    expect(csvCell('Rua "do Meio"')).not.toContain("\\");
  });
  it("quotes only when it has to, and knows its own separator", () => {
    useLocale("pt-BR");
    expect(csvCell("Simples")).toBe("Simples");
    expect(csvCell("a;b")).toBe('"a;b"');
    expect(csvCell("linha1\nlinha2")).toBe('"linha1\nlinha2"');
    expect(csvCell("a,b")).toBe("a,b"); // not the separator in pt-BR

    useLocale("en-US", "USD");
    expect(csvCell("a,b")).toBe('"a,b"'); // it is in en-US
    expect(csvCell("a;b")).toBe("a;b");
  });
  it("leaves empties and numbers unquoted", () => {
    expect(csvCell(null)).toBe("");
    expect(csvCell(undefined)).toBe("");
    expect(csvCell(0)).toBe("0");
    expect(csvCell(1234.5)).toBe("1234.5");
    expect(csvCell(false)).toBe("false");
  });
  it("serialises objects", () => {
    expect(csvCell({ a: 1 })).toBe('"{""a"":1}"');
  });
});

describe("csvSep", () => {
  // Excel reads a CSV in its own locale: where the decimal mark is a comma,
  // a comma cannot also separate the columns.
  it("is ; where the decimal mark is a comma, and , where it is a dot", () => {
    useLocale("pt-BR");
    expect(csvSep()).toBe(";");
    useLocale("en-US", "USD");
    expect(csvSep()).toBe(",");
  });
});

describe("toCsv", () => {
  it("builds a header and rows with the locale's separator and CRLF", () => {
    useLocale("pt-BR");
    expect(toCsv(["Nome", "Valor"], [["Ana", 10], ['B "x"', null]]))
      .toBe('Nome;Valor\r\nAna;10\r\n"B ""x""";');

    useLocale("en-US", "USD");
    expect(toCsv(["Name", "Value"], [["Ana", 10]])).toBe("Name,Value\r\nAna,10");
  });
});
