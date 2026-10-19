import assert from "node:assert/strict";
import { describe, expect, it, test } from "vitest";
import { diffTable, extractFileName, formatDiffValue, formatMultiSelect, matchesHistory, parseVersion, signatureDiff } from "./history";

const childMeta = {
  name: "Item",
  app: "x",
  label: "Item",
  fields: [{ fieldname: "qty", label: "Qty", fieldtype: "Int" }],
} as any;

describe("diffTable row identity", () => {
  it("pairs rows by id", () => {
    const d = diffTable(
      [{ id: "r1", idx: 1, qty: 1 }, { id: "r2", idx: 2, qty: 5 }],
      [{ id: "r2", idx: 1, qty: 6 }],
      childMeta,
    );
    expect(d.removed.map((r) => r.rowName)).toEqual(["r1"]);
    expect(d.added).toEqual([]);
    expect(d.modified.map((m) => m.rowName)).toEqual(["r2"]);
  });

  // A Version written before 0.17 kept its child rows keyed by `name`.
  it("pairs the rows of a Version written before 0.17 by their name", () => {
    const d = diffTable(
      [{ name: "r1", idx: 1, qty: 1 }, { name: "r2", idx: 2, qty: 5 }],
      [{ name: "r2", idx: 1, qty: 6 }],
      childMeta,
    );
    expect(d.removed.map((r) => r.rowName)).toEqual(["r1"]);
    expect(d.added).toEqual([]);
    expect(d.modified.map((m) => m.rowName)).toEqual(["r2"]);
    expect(d.modified[0].changes.map((c) => c.field)).toEqual(["qty"]);
  });
});

describe("Table MultiSelect in history", () => {
  const tagMeta = { name: "Note Tag", app: "x", label: "Note Tag", fields: [{ fieldname: "tag", label: "Tag", fieldtype: "Link", options: "Tag" }] } as any;

  it("shows the values, not a table of rows", () => {
    expect(formatMultiSelect([{ id: "a", tag: "red" }, { id: "b", tag: "blue" }], tagMeta)).toBe("red, blue");
    expect(formatMultiSelect([], tagMeta)).toBe("—");
  });
});

describe("parseVersion", () => {
  it("marks the Version a delete wrote, whose data is the document rather than a diff", () => {
    const v = parseVersion({ id: "v1", owner: "Admin", creation: "2026-09-28 10:00:00", data: { deleted: { id: "m1", title: "m1" } } });
    expect(v.deleted).toBe(true);
    expect(v.changes).toEqual([]);
  });

  it("leaves an ordinary Version unmarked", () => {
    const v = parseVersion({ id: "v2", owner: "Admin", creation: "2026-09-28 10:00:00", data: JSON.stringify({ changed: { title: ["a", "b"] } }) });
    expect(v.deleted).toBe(false);
    expect(v.changes.map((c) => c.field)).toEqual(["title"]);
  });
});

describe("Signature in history", () => {
  const a = "sha256:0123456789ab", b = "sha256:ba9876543210";
  const sig = { fieldname: "signed", label: "Signed by", fieldtype: "Signature" } as any;

  it("says what happened, never showing the marker", () => {
    expect(signatureDiff(null, a)).toEqual(["—", "Signed"]);
    expect(signatureDiff(a, null)).toEqual(["Signed", "Removed"]);
    expect(signatureDiff(a, b)).toEqual(["Signed", "Changed"]);
    expect(formatDiffValue(a, sig).formatted).toBe("Signed");
    expect(formatDiffValue(null, sig).formatted).toBe("—");
  });

  it("reads a Version's markers on the document and in child rows", () => {
    const frm: any = {
      field: (f: string) => (f === "signed" ? sig : f === "stops" ? { fieldname: "stops", fieldtype: "Table", options: "Stop", label: "Stops" } : undefined),
      meta: { children: { Stop: { name: "Stop", fields: [sig] } } },
    };
    const v = parseVersion({ id: "V-1", data: { changed: { signed: [a, b], stops: [[{ id: "r1", signed: null }], [{ id: "r1", signed: a }]] } } }, frm);
    const doc = v.changes.find((c) => c.field === "signed")!;
    expect([doc.formattedOld, doc.formattedNew]).toEqual(["Signed", "Changed"]);
    const row = v.changes.find((c) => c.field === "stops")!.tableDiff!.modified[0].changes[0];
    expect([row.formattedFrom, row.formattedTo]).toEqual(["—", "Signed"]);
    expect(JSON.stringify(v.changes.map((c) => [c.formattedOld, c.formattedNew]))).not.toContain("sha256");
  });
});

describe("Geolocation in history", () => {
  const area = { fieldname: "area", label: "Area", fieldtype: "Geolocation" } as any;
  const point = { type: "FeatureCollection", features: [{ type: "Feature", geometry: { type: "Point", coordinates: [-46.6333, -23.5505] }, properties: {} }] };
  const line = { type: "FeatureCollection", features: [{ type: "Feature", geometry: { type: "LineString", coordinates: [[0, 0], [1, 1]] }, properties: {} }] };

  it("says each side in words, never as GeoJSON", () => {
    expect(formatDiffValue(point, area).formatted).toBe("-23.55050, -46.63330");
    expect(formatDiffValue(line, area).formatted).toBe("1 line");
    expect(formatDiffValue(null, area).formatted).toBe("—");
    expect(formatDiffValue(point, area).formatted).not.toContain("FeatureCollection");
  });
});

test("extractFileName returns clean file name from path", () => {
  assert.equal(
    extractFileName("/private/files/WhatsApp_Image_2026-09-09_at_14_27_59_340731000.jpeg"),
    "WhatsApp_Image_2026-09-09_at_14_27_59_340731000.jpeg"
  );
  assert.equal(extractFileName(""), "");
});

test("formatDiffValue formats booleans, docstatus, attachments and nulls correctly", () => {
  assert.deepEqual(formatDiffValue(null), { value: null, formatted: "—", isAttach: false });
  assert.deepEqual(formatDiffValue(true, { fieldtype: "Check" }), { value: true, formatted: "Yes", isAttach: false });
  assert.deepEqual(formatDiffValue(false, { fieldtype: "Check" }), { value: false, formatted: "No", isAttach: false });
  assert.deepEqual(formatDiffValue(0, undefined, "docstatus"), { value: 0, formatted: "Draft", isAttach: false });
  assert.deepEqual(formatDiffValue(1, undefined, "docstatus"), { value: 1, formatted: "Submitted", isAttach: false });
  assert.deepEqual(formatDiffValue(2, undefined, "docstatus"), { value: 2, formatted: "Cancelled", isAttach: false });
  assert.deepEqual(formatDiffValue("/private/files/doc.pdf"), {
    value: "/private/files/doc.pdf",
    formatted: "doc.pdf",
    isAttach: true,
    url: "/private/files/doc.pdf",
  });
});

test("diffTable detects added rows and strips internal metadata", () => {
  const childMeta = {
    name: "Anexo Documento",
    fields: [
      { fieldname: "tipo_documento", label: "Tipo", fieldtype: "Select" },
      { fieldname: "arquivo", label: "Arquivo", fieldtype: "Attach" },
      { fieldname: "descricao", label: "Descrição", fieldtype: "Data" },
    ],
  } as any;

  const before: any[] = [];
  const after = [
    {
      idx: 1,
      name: "3mj649p8s0",
      owner: "Admin",
      parent: "PES-00002",
      arquivo: "/private/files/img.jpeg",
      creation: "2026-09-10T07:27:37.647441-04:00",
      tipo_documento: "Documento Pessoal",
      descricao: "TESTE",
    },
  ];

  const diff = diffTable(before, after, childMeta);
  assert.equal(diff.added.length, 1);
  assert.equal(diff.removed.length, 0);
  assert.equal(diff.modified.length, 0);

  const row = diff.added[0];
  assert.equal(row.rowIdx, 1);
  assert.equal(row.rowName, "3mj649p8s0");
  assert.equal(row.fields.tipo_documento.label, "Tipo");
  assert.equal(row.fields.tipo_documento.formatted, "Documento Pessoal");
  assert.equal(row.fields.arquivo.isAttach, true);
  assert.equal(row.fields.arquivo.formatted, "img.jpeg");
  // Internal fields must be stripped:
  assert.equal(row.fields.parent, undefined);
  assert.equal(row.fields.owner, undefined);
  assert.equal(row.fields.creation, undefined);
});

test("diffTable detects modified rows and isolates changed fields", () => {
  const childMeta = {
    name: "Anexo Documento",
    fields: [
      { fieldname: "tipo_documento", label: "Tipo", fieldtype: "Select" },
      { fieldname: "arquivo", label: "Arquivo", fieldtype: "Attach" },
      { fieldname: "descricao", label: "Descrição", fieldtype: "Data" },
    ],
  } as any;

  const before = [
    {
      idx: 1,
      name: "3mj649p8s0",
      arquivo: "/private/files/old.jpeg",
      tipo_documento: "Documento Pessoal",
      descricao: "TESTE",
      modified: "2026-09-10T07:29:08.733148-04:00",
    },
  ];

  const after = [
    {
      idx: 1,
      name: "3mj649p8s0",
      arquivo: "/private/files/new.jpeg",
      tipo_documento: "Matrícula",
      descricao: "TESTE",
      modified: "2026-09-10T09:15:56.153557-04:00",
    },
  ];

  const diff = diffTable(before, after, childMeta);
  assert.equal(diff.added.length, 0);
  assert.equal(diff.removed.length, 0);
  assert.equal(diff.modified.length, 1);

  const mod = diff.modified[0];
  assert.equal(mod.rowIdx, 1);
  assert.equal(mod.changes.length, 2);

  const tipoChange = mod.changes.find((c) => c.field === "tipo_documento");
  assert.ok(tipoChange);
  assert.equal(tipoChange.label, "Tipo");
  assert.equal(tipoChange.formattedFrom, "Documento Pessoal");
  assert.equal(tipoChange.formattedTo, "Matrícula");

  const arqChange = mod.changes.find((c) => c.field === "arquivo");
  assert.ok(arqChange);
  assert.equal(arqChange.formattedFrom, "old.jpeg");
  assert.equal(arqChange.formattedTo, "new.jpeg");
  assert.equal(arqChange.isAttach, true);

  // 'descricao' (identical) and 'modified' (internal) must not appear in changes:
  assert.equal(mod.changes.find((c) => c.field === "descricao"), undefined);
  assert.equal(mod.changes.find((c) => c.field === "modified"), undefined);
});

test("parseVersion processes full version with standard and child table changes", () => {
  const fakeFrm = {
    doctype: "Pessoa",
    field: (name: string) => {
      if (name === "email") return { fieldname: "email", label: "E-mail", fieldtype: "Data" };
      if (name === "anexos") return { fieldname: "anexos", label: "Anexos", fieldtype: "Table", options: "Anexo Documento" };
      return undefined;
    },
    meta: {
      children: {
        "Anexo Documento": {
          name: "Anexo Documento",
          fields: [
            { fieldname: "tipo_documento", label: "Tipo", fieldtype: "Select" },
            { fieldname: "arquivo", label: "Arquivo", fieldtype: "Attach" },
          ],
        },
      },
    },
  } as any;

  const rawVersion = {
    id: "v123",
    owner: "Admin",
    creation: new Date().toISOString(),
    data: JSON.stringify({
      changed: {
        email: ["old@example.com", "new@example.com"],
        anexos: [
          [],
          [{ name: "row1", idx: 1, tipo_documento: "Contrato", arquivo: "/files/c.pdf" }],
        ],
      },
    }),
  };

  const parsed = parseVersion(rawVersion, fakeFrm);
  assert.equal(parsed.owner, "Admin");
  assert.equal(parsed.changes.length, 2);

  const emailChange = parsed.changes.find((c) => c.field === "email");
  assert.ok(emailChange);
  assert.equal(emailChange.label, "E-mail");
  assert.equal(emailChange.formattedOld, "old@example.com");
  assert.equal(emailChange.formattedNew, "new@example.com");

  const tableChange = parsed.changes.find((c) => c.field === "anexos");
  assert.ok(tableChange);
  assert.equal(tableChange.isTable, true);
  assert.equal(tableChange.tableDiff!.added.length, 1);
  assert.equal(tableChange.tableDiff!.added[0].fields.arquivo.formatted, "c.pdf");
});

test("parseVersion accurately reflects PES-00002 real versions from database", () => {
  const fakeFrm = {
    doctype: "Pessoa",
    field: (name: string) => {
      const fields = {
        email: { fieldname: "email", label: "E-mail", fieldtype: "Data" },
        cep: { fieldname: "cep", label: "CEP", fieldtype: "Data" },
        endereco: { fieldname: "endereco", label: "Logradouro", fieldtype: "Data" },
        numero: { fieldname: "numero", label: "Número", fieldtype: "Data" },
        complemento: { fieldname: "complemento", label: "Complemento", fieldtype: "Data" },
        bairro: { fieldname: "bairro", label: "Bairro", fieldtype: "Data" },
        municipio: { fieldname: "municipio", label: "Município", fieldtype: "Data" },
        estado: { fieldname: "estado", label: "UF", fieldtype: "Select" },
        anexos: { fieldname: "anexos", label: "Anexos", fieldtype: "Table", options: "Anexo Documento" },
      };
      return (fields as Record<string, any>)[name];
    },
    meta: {
      children: {
        "Anexo Documento": {
          name: "Anexo Documento",
          fields: [
            { fieldname: "tipo_documento", label: "Tipo", fieldtype: "Select" },
            { fieldname: "descricao", label: "Descrição", fieldtype: "Data" },
            { fieldname: "arquivo", label: "Arquivo", fieldtype: "Attach" },
            { fieldname: "validade", label: "Validade", fieldtype: "Date" },
          ],
        },
      },
    },
  } as any;

  const vRow1 = {
    name: "ntfgsx5nn4",
    owner: "Admin",
    creation: "2026-09-10 13:15:56.142761+00",
    data: {
      changed: {
        anexos: [
          [{ idx: 1, name: "3mj649p8s0", owner: "Admin", parent: "PES-00002", arquivo: "/private/files/WhatsApp_Image_2026-09-09_at_14_27_59_340731000.jpeg", doctype: "Anexo Documento", creation: "2026-09-10T07:27:37.647441-04:00", modified: "2026-09-10T07:29:08.733148-04:00", validade: null, descricao: "TESTE", docstatus: 0, parenttype: "Pessoa", modified_by: "Admin", parentfield: "anexos", tipo_documento: "Documento Pessoal" }],
          [{ idx: 1, name: "3mj649p8s0", owner: "Admin", parent: "PES-00002", arquivo: "/private/files/WhatsApp_Image_2026-09-09_at_14_40_39_737162000.jpeg", doctype: "Anexo Documento", creation: "2026-09-10T07:27:37.647441-04:00", modified: "2026-09-10T09:15:56.153557-04:00", validade: null, descricao: "TESTE", docstatus: 0, parenttype: "Pessoa", modified_by: "Admin", parentfield: "anexos", tipo_documento: "Matrícula" }]
        ]
      }
    }
  };

  const p1 = parseVersion(vRow1, fakeFrm);
  assert.equal(p1.changes.length, 1);
  const tableChange = p1.changes[0];
  assert.equal(tableChange.field, "anexos");
  assert.equal(tableChange.tableDiff!.modified.length, 1);
  const rowMod = tableChange.tableDiff!.modified[0];
  assert.equal(rowMod.rowIdx, 1);
  assert.equal(rowMod.changes.length, 2);
  const tipo = rowMod.changes.find(c => c.field === "tipo_documento");
  assert.equal(tipo!.formattedFrom, "Documento Pessoal");
  assert.equal(tipo!.formattedTo, "Matrícula");
  const arq = rowMod.changes.find(c => c.field === "arquivo");
  assert.equal(arq!.isAttach, true);
  assert.equal(arq!.formattedFrom, "WhatsApp_Image_2026-09-09_at_14_27_59_340731000.jpeg");
  assert.equal(arq!.formattedTo, "WhatsApp_Image_2026-09-09_at_14_40_39_737162000.jpeg");
});

describe("formatDiffValue shows a value as a list does", () => {
  it("shows a Select by its label and a Currency at its precision", () => {
    const select = { fieldtype: "Select", options: ["Open", "Closed"], optionLabels: ["Aberto", "Fechado"] } as any;
    expect(formatDiffValue("Closed", select).formatted).toBe("Fechado");
    expect(formatDiffValue("1.2345", { fieldtype: "Currency", precision: 4 } as any).formatted).toContain("2345");
  });
});

describe("matchesHistory", () => {
  const version = (data: any) => parseVersion({ id: "v1", owner: "Ana", creation: "2026-01-01 10:00:00", data }, undefined);
  const rows = version({
    changed: {
      items: [
        [{ id: "r1", idx: 1, qty: 77 }, { id: "r2", idx: 2, qty: 5 }],
        [{ id: "r2", idx: 1, qty: 6 }, { id: "r3", idx: 2, note: "fresh" }],
      ],
      title: ["Old", "New"],
    },
  });

  it("matches owner, field and value", () => {
    expect(matchesHistory(rows, "ana")).toBe(true);
    expect(matchesHistory(rows, "title")).toBe(true);
    expect(matchesHistory(rows, "new")).toBe(true);
    expect(matchesHistory(rows, "zzz")).toBe(false);
  });

  it("matches added, modified and removed rows", () => {
    expect(matchesHistory(rows, "fresh")).toBe(true);
    expect(matchesHistory(rows, "6")).toBe(true);
    // 77 only appears in the removed row r1
    expect(matchesHistory(rows, "77")).toBe(true);
  });
});
