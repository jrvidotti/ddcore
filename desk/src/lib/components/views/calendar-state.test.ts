import { describe, expect, it } from "vitest";
import type { Field } from "../../meta";
import { calendarRangeFilters, groupCalendarRows } from "./calendar-state";

const fields: Field[] = [
  { fieldname: "due_date", fieldtype: "Date" },
  { fieldname: "starts_at", fieldtype: "Datetime" },
];

describe("calendar days and query bounds", () => {
  it.each(["creation", "modified", "starts_at"])("groups %s by the site's day across UTC midnight", (field) => {
    const rows = [
      { id: "late", [field]: "2026-09-15T03:59:59.999999Z" },
      { id: "next", [field]: "2026-09-15T04:00:00.000Z" },
      { id: "empty", [field]: null },
      { id: "invalid", [field]: "not a date" },
    ];
    expect([...groupCalendarRows(rows, field, fields, "America/Cuiaba")]).toEqual([
      ["2026-09-14", [rows[0]]],
      ["2026-09-15", [rows[1]]],
    ]);
  });

  it.each(["creation", "modified", "starts_at"])("includes the whole final site day when filtering %s", (field) => {
    expect(calendarRangeFilters(field, fields, "2026-08-30", "2026-10-03", "America/Cuiaba")).toEqual([
      [field, ">=", "2026-08-30T04:00:00.000Z"],
      [field, "<=", "2026-10-04T03:59:59.999999Z"],
    ]);
  });

  it("converts each boundary using its own DST offset", () => {
    expect(calendarRangeFilters("creation", fields, "2026-03-28", "2026-03-29", "Europe/Lisbon")).toEqual([
      ["creation", ">=", "2026-03-28T00:00:00.000Z"],
      ["creation", "<=", "2026-03-29T22:59:59.999999Z"],
    ]);
  });

  it("keeps Date grouping and inclusive bounds as civil dates", () => {
    const rows = [{ id: "due", due_date: "2026-09-14" }, { id: "empty", due_date: "" }];
    expect([...groupCalendarRows(rows, "due_date", fields, "America/Cuiaba")]).toEqual([["2026-09-14", [rows[0]]]]);
    expect([...groupCalendarRows(rows, "due_date", fields, "America/Cuiaba", "2026-09-24")]).toEqual([
      ["2026-09-14", [rows[0]]], ["2026-09-24", [rows[1]]],
    ]);
    expect(calendarRangeFilters("due_date", fields, "2026-08-30", "2026-10-03", "Asia/Tokyo")).toEqual([
      ["due_date", ">=", "2026-08-30"], ["due_date", "<=", "2026-10-03"],
    ]);
  });
});
