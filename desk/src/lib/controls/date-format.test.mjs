import assert from "node:assert/strict";
import test from "node:test";

import {
  formatDateBr,
  parseDateBr,
  maskDateInput,
  getCalendarDays,
  daysInMonth,
} from "./date-format.ts";

test("formatDateBr formats ISO date strings to DD/MM/YYYY", () => {
  assert.equal(formatDateBr("2026-03-01"), "01/03/2026");
  assert.equal(formatDateBr("2026-12-31"), "31/12/2026");
  assert.equal(formatDateBr("01/03/2026"), "01/03/2026");
  assert.equal(formatDateBr(""), "");
  assert.equal(formatDateBr(null), "");
  assert.equal(formatDateBr(undefined), "");
});

test("daysInMonth handles leap years and variable month lengths", () => {
  assert.equal(daysInMonth(2024, 2), 29); // leap year
  assert.equal(daysInMonth(2023, 2), 28); // non-leap year
  assert.equal(daysInMonth(2026, 1), 31);
  assert.equal(daysInMonth(2026, 4), 30);
});

test("parseDateBr parses valid DD/MM/YYYY and YYYY-MM-DD strings", () => {
  assert.deepEqual(parseDateBr("01/03/2026"), { year: 2026, month: 3, day: 1, iso: "2026-03-01" });
  assert.deepEqual(parseDateBr("29/02/2024"), { year: 2024, month: 2, day: 29, iso: "2024-02-29" });
  assert.deepEqual(parseDateBr("2026-03-01"), { year: 2026, month: 3, day: 1, iso: "2026-03-01" });
});

test("parseDateBr rejects invalid dates", () => {
  assert.equal(parseDateBr(""), null);
  assert.equal(parseDateBr("29/02/2023"), null); // 2023 is not leap
  assert.equal(parseDateBr("32/01/2026"), null);
  assert.equal(parseDateBr("01/13/2026"), null);
  assert.equal(parseDateBr("01/00/2026"), null);
  assert.equal(parseDateBr("invalid"), null);
});

test("maskDateInput formats typed digits into DD/MM/YYYY", () => {
  assert.equal(maskDateInput(""), "");
  assert.equal(maskDateInput("0"), "0");
  assert.equal(maskDateInput("4"), "04/");
  assert.equal(maskDateInput("01"), "01/");
  assert.equal(maskDateInput("010"), "01/0");
  assert.equal(maskDateInput("013"), "01/03/");
  assert.equal(maskDateInput("0103"), "01/03/");
  assert.equal(maskDateInput("01032"), "01/03/2");
  assert.equal(maskDateInput("01032026"), "01/03/2026");
  assert.equal(maskDateInput("01/03/2026"), "01/03/2026");
});

test("getCalendarDays returns full weeks grid with current month flags", () => {
  // March 2026: March 1 is a Sunday
  const days = getCalendarDays(2026, 3);
  assert.equal(days.length % 7, 0);
  assert.ok(days.length >= 35);
  // First day should be 2026-03-01 since March 1 2026 is Sunday
  assert.equal(days[0].iso, "2026-03-01");
  assert.equal(days[0].isCurrentMonth, true);

  // April 2026: April 1 is Wednesday (3 overflow days from March: 29, 30, 31)
  const aprDays = getCalendarDays(2026, 4);
  assert.equal(aprDays[0].isCurrentMonth, false);
  assert.equal(aprDays[0].month, 3);
  assert.equal(aprDays[3].isCurrentMonth, true);
  assert.equal(aprDays[3].day, 1);
});
