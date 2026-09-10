import assert from "node:assert/strict";
import test from "node:test";

import { isSectionCollapsed, toggleSection } from "./section-state.ts";

test("a collapsible section starts open and closes on the first toggle", () => {
  const collapsed = {};

  assert.equal(isSectionCollapsed(collapsed, 0), false);
  toggleSection(collapsed, 0);
  assert.equal(isSectionCollapsed(collapsed, 0), true);
});

test("a closed section opens on the next toggle", () => {
  const collapsed = { 0: true };

  toggleSection(collapsed, 0);
  assert.equal(isSectionCollapsed(collapsed, 0), false);
});
