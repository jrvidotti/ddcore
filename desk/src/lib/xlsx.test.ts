import { describe, expect, it } from "vitest";
import { columnName, crc32, sheetXml, toXlsx, xmlText, zipStore } from "./xlsx";

const enc = new TextEncoder();
const dec = new TextDecoder();

/** Reads a stored ZIP through its central directory, as a spreadsheet would. */
function unzip(z: Uint8Array): Record<string, string> {
  const v = new DataView(z.buffer, z.byteOffset, z.byteLength);
  const end = z.length - 22;
  expect(v.getUint32(end, true)).toBe(0x06054b50);
  const n = v.getUint16(end + 10, true);
  let p = v.getUint32(end + 16, true);
  const out: Record<string, string> = {};
  for (let i = 0; i < n; i++) {
    expect(v.getUint32(p, true)).toBe(0x02014b50);
    const crc = v.getUint32(p + 16, true), size = v.getUint32(p + 20, true), nameLen = v.getUint16(p + 28, true);
    const off = v.getUint32(p + 42, true);
    const name = dec.decode(z.subarray(p + 46, p + 46 + nameLen));
    expect(v.getUint32(off, true)).toBe(0x04034b50);
    const localName = v.getUint16(off + 26, true);
    const data = z.subarray(off + 30 + localName, off + 30 + localName + size);
    expect(crc32(data)).toBe(crc);
    out[name] = dec.decode(data);
    p += 46 + nameLen;
  }
  return out;
}

describe("xlsx", () => {
  it("computes the standard CRC-32", () => {
    expect(crc32(enc.encode("123456789"))).toBe(0xcbf43926);
    expect(crc32(new Uint8Array())).toBe(0);
  });
  it("names columns like a spreadsheet", () => {
    expect([0, 25, 26, 27, 701, 702].map(columnName)).toEqual(["A", "Z", "AA", "AB", "ZZ", "AAA"]);
  });
  it("escapes XML and drops the control characters it cannot hold", () => {
    expect(xmlText(`a<b>&"c"\u0001`)).toBe("a&lt;b&gt;&amp;&quot;c&quot;");
  });
  it("writes numbers, booleans and text as their own cell types, empty cells not at all", () => {
    const xml = sheetXml(["Name", "Qty"], [["Ana", 3], [null, true]]);
    expect(xml).toContain('<c r="A1" s="1" t="inlineStr"><is><t xml:space="preserve">Name</t></is></c>');
    expect(xml).toContain('<c r="B2"><v>3</v></c>');
    expect(xml).toContain('<row r="3"><c r="B3" t="b"><v>1</v></c></row>');
  });
  it("round-trips a zip of stored entries", () => {
    const files = unzip(zipStore([{ name: "a.txt", data: enc.encode("hello") }, { name: "ç/b.xml", data: enc.encode("<x/>") }]));
    expect(files).toEqual({ "a.txt": "hello", "ç/b.xml": "<x/>" });
  });
  it("packages a workbook with the parts a spreadsheet needs", () => {
    const files = unzip(toXlsx(["A"], [["Endereço"]], "Students: [2026]"));
    expect(Object.keys(files).sort()).toEqual([
      "[Content_Types].xml", "_rels/.rels", "xl/_rels/workbook.xml.rels", "xl/styles.xml", "xl/workbook.xml", "xl/worksheets/sheet1.xml",
    ]);
    expect(files["xl/workbook.xml"]).toContain('<sheet name="Students   2026" sheetId="1" r:id="rId1"/>');
    expect(files["xl/worksheets/sheet1.xml"]).toContain("Endereço");
  });
});
