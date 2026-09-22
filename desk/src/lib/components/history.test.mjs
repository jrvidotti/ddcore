import assert from "node:assert/strict";
import test from "node:test";

import { diffTable, formatDiffValue, parseVersion, extractFileName } from "./history.ts";

test("extractFileName returns clean file name from path", () => {
  assert.equal(
    extractFileName("/private/files/WhatsApp_Image_2026-09-09_at_14_27_59_340731000.jpeg"),
    "WhatsApp_Image_2026-09-09_at_14_27_59_340731000.jpeg"
  );
  assert.equal(extractFileName(""), "");
});

test("formatDiffValue formats booleans, docstatus, attachments and nulls correctly", () => {
  assert.deepEqual(formatDiffValue(null), { value: null, formatted: "—", isAttach: false });
  assert.deepEqual(formatDiffValue(true, { fieldtype: "Check" }), { value: true, formatted: "Sim", isAttach: false });
  assert.deepEqual(formatDiffValue(false, { fieldtype: "Check" }), { value: false, formatted: "Não", isAttach: false });
  assert.deepEqual(formatDiffValue(0, undefined, "docstatus"), { value: 0, formatted: "Rascunho", isAttach: false });
  assert.deepEqual(formatDiffValue(1, undefined, "docstatus"), { value: 1, formatted: "Enviado", isAttach: false });
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
  };

  const before = [];
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
  };

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
    field: (name) => {
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
  };

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
  assert.equal(tableChange.tableDiff.added.length, 1);
  assert.equal(tableChange.tableDiff.added[0].fields.arquivo.formatted, "c.pdf");
});

test("parseVersion accurately reflects PES-00002 real versions from database", () => {
  const fakeFrm = {
    doctype: "Pessoa",
    field: (name) => {
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
      return fields[name];
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
  };

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
  assert.equal(tableChange.tableDiff.modified.length, 1);
  const rowMod = tableChange.tableDiff.modified[0];
  assert.equal(rowMod.rowIdx, 1);
  assert.equal(rowMod.changes.length, 2);
  const tipo = rowMod.changes.find(c => c.field === "tipo_documento");
  assert.equal(tipo.formattedFrom, "Documento Pessoal");
  assert.equal(tipo.formattedTo, "Matrícula");
  const arq = rowMod.changes.find(c => c.field === "arquivo");
  assert.equal(arq.isAttach, true);
  assert.equal(arq.formattedFrom, "WhatsApp_Image_2026-09-09_at_14_27_59_340731000.jpeg");
  assert.equal(arq.formattedTo, "WhatsApp_Image_2026-09-09_at_14_40_39_737162000.jpeg");
});
