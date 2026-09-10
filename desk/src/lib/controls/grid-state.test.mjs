import assert from "node:assert/strict";
import test from "node:test";

import {
  applyRowChanges,
  confirmRowRemoval,
  createChildDraft,
  fileNameParts,
  gridEditMode,
} from "./grid-state.ts";

test("grid editing stays inline unless dialog mode is explicit", () => {
  assert.equal(gridEditMode({ fieldtype: "Table" }), "inline");
  assert.equal(gridEditMode({ fieldtype: "Table", gridEditMode: "dialog" }), "dialog");
});

test("a child draft does not mutate the current rows before apply", () => {
  const rows = [{ name: "ROW-1" }];

  const draft = createChildDraft("anexos", "Anexo Documento", "Pessoa", rows.length);

  assert.equal(rows.length, 1);
  assert.deepEqual(draft, {
    doctype: "Anexo Documento",
    parentfield: "anexos",
    parenttype: "Pessoa",
    idx: 2,
    __islocal: true,
  });
});

test("applying edits updates the existing row", () => {
  const row = { name: "ROW-1", descricao: "Antes" };

  applyRowChanges(row, { descricao: "Depois" });

  assert.deepEqual(row, { name: "ROW-1", descricao: "Depois" });
});

test("row removal runs only after confirmation", async () => {
  let removals = 0;

  assert.equal(await confirmRowRemoval(async () => false, () => removals++), false);
  assert.equal(removals, 0);
  assert.equal(await confirmRowRemoval(async () => true, () => removals++), true);
  assert.equal(removals, 1);
});

test("file name parts preserve the extension from a long multi-dot name", () => {
  assert.deepEqual(fileNameParts("/files/contrato.assinado.final.pdf"), {
    full: "contrato.assinado.final.pdf",
    stem: "contrato.assinado.final",
    extension: ".pdf",
  });
});

test("file name parts support names without an extension", () => {
  assert.deepEqual(fileNameParts("/files/LEIA-ME"), {
    full: "LEIA-ME",
    stem: "LEIA-ME",
    extension: "",
  });
});
