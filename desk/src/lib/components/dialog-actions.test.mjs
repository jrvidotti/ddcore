import assert from "node:assert/strict";
import test from "node:test";

import { runDialogAction } from "./dialog-actions.ts";

test("a dialog action owns the busy state for its full async lifetime", async () => {
  const dialog = { busy: false };
  let busyDuringAction = false;

  await runDialogAction(async () => { busyDuringAction = dialog.busy; }, {}, dialog);

  assert.equal(busyDuringAction, true);
  assert.equal(dialog.busy, false);
});

test("a dialog action clears busy after an error", async () => {
  const dialog = { busy: false };

  await assert.rejects(() => runDialogAction(async () => { throw new Error("falhou"); }, {}, dialog), /falhou/);

  assert.equal(dialog.busy, false);
});
