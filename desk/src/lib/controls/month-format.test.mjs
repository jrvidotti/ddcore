import assert from "node:assert/strict";
import test from "node:test";

import {
  formatMonth,
  maskMonthInput,
  parseMonth,
} from "./month-format.ts";

test("formatMonth formats ISO date strings to MM/YYYY", () => {
  assert.equal(formatMonth("2026-09-01"), "09/2026");
  assert.equal(formatMonth("2026-09"), "09/2026");
  assert.equal(formatMonth("2026-12-31"), "12/2026");
  assert.equal(formatMonth("2026-01-15"), "01/2026");
  assert.equal(formatMonth("09/2026"), "09/2026");
  assert.equal(formatMonth(""), "");
  assert.equal(formatMonth(null), "");
  assert.equal(formatMonth(undefined), "");
});

test("parseMonth parses valid MM/YYYY strings", () => {
  assert.deepEqual(parseMonth("09/2026"), { year: 2026, month: 9, iso: "2026-09-01" });
  assert.deepEqual(parseMonth("1/2026"), { year: 2026, month: 1, iso: "2026-01-01" });
  assert.deepEqual(parseMonth("12/2030"), { year: 2030, month: 12, iso: "2030-12-01" });
  assert.deepEqual(parseMonth("2026-09-01"), { year: 2026, month: 9, iso: "2026-09-01" });
  assert.deepEqual(parseMonth("2026-09"), { year: 2026, month: 9, iso: "2026-09-01" });
});

test("parseMonth rejects invalid strings", () => {
  assert.equal(parseMonth(""), null);
  assert.equal(parseMonth("00/2026"), null);
  assert.equal(parseMonth("13/2026"), null);
  assert.equal(parseMonth("abc"), null);
  assert.equal(parseMonth("09/20"), null);
});

test("maskMonthInput masks typed input smoothly", () => {
  assert.equal(maskMonthInput(""), "");
  assert.equal(maskMonthInput("0"), "0");
  assert.equal(maskMonthInput("1"), "1");
  // Digit > 1 auto-completes month with 0 and slash
  assert.equal(maskMonthInput("2"), "02/");
  assert.equal(maskMonthInput("9"), "09/");
  // Two digits for month
  assert.equal(maskMonthInput("09"), "09/");
  assert.equal(maskMonthInput("12"), "12/");
  // Month 00 clamped to 01
  assert.equal(maskMonthInput("00"), "01/");
  // Month > 12 clamped to 12
  assert.equal(maskMonthInput("13"), "12/");
  assert.equal(maskMonthInput("99"), "12/");
  // Adding year digits
  assert.equal(maskMonthInput("092"), "09/2");
  assert.equal(maskMonthInput("0920"), "09/20");
  assert.equal(maskMonthInput("09202"), "09/202");
  assert.equal(maskMonthInput("092026"), "09/2026");
  // Max 6 digits
  assert.equal(maskMonthInput("092026999"), "09/2026");
  // With non-digits
  assert.equal(maskMonthInput("09/2026"), "09/2026");
});
