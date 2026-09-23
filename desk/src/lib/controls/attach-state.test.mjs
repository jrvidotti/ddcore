import assert from "node:assert/strict";
import test from "node:test";

import { attachAction, fileLabel, formatFileSize, runAttachOperation, storedName } from "./attach-state.ts";

test("a required existing attachment can be replaced but not cleared", () => {
  assert.equal(attachAction("/files/documento.pdf", true), "replace");
});

test("an optional existing attachment can be removed", () => {
  assert.equal(attachAction("/files/documento.pdf", false), "remove");
});

test("an empty attachment can be selected", () => {
  assert.equal(attachAction(null, true), "attach");
});

test("an attachment operation reports busy for its full async lifetime", async () => {
  const states = [];
  let busyDuringUpload = false;

  await runAttachOperation((busy) => states.push(busy), async () => { busyDuringUpload = states.at(-1); });

  assert.equal(busyDuringUpload, true);
  assert.deepEqual(states, [true, false]);
});

test("an attachment operation clears busy after an upload error", async () => {
  const states = [];

  await assert.rejects(() => runAttachOperation((busy) => states.push(busy), async () => { throw new Error("upload falhou"); }), /upload falhou/);

  assert.deepEqual(states, [true, false]);
});

test("a file is labelled by its original name, falling back to the stored one", () => {
  assert.equal(fileLabel({ file_name: "contrato.pdf" }, "/private/files/ab12.pdf"), "contrato.pdf");
  assert.equal(fileLabel(null, "/private/files/ab12.pdf"), "ab12.pdf");
  assert.equal(storedName(null), "");
});

test("file sizes read as bytes, KB and MB", () => {
  assert.equal(formatFileSize(512), "512 B");
  assert.equal(formatFileSize(1536), "1.5 KB");
  assert.equal(formatFileSize(5 * 1024 * 1024), "5.0 MB");
  assert.equal(formatFileSize(null), "");
});
