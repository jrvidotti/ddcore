import { describe, expect, it } from "vitest";
import type { Field } from "../../meta";
import { calendarLanes, calendarRangeFilters, calendarSpanFilters, groupCalendarRows } from "./calendar-state";

const fields: Field[] = [
  { fieldname: "due_date", fieldtype: "Date" },
  { fieldname: "starts_at", fieldtype: "Datetime" },
  { fieldname: "end_date", fieldtype: "Date" },
  { fieldname: "ends_at", fieldtype: "Datetime" },
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

// Two weeks, Sunday 2026-11-01 to Saturday 2026-11-14.
const grid = Array.from({ length: 14 }, (_, i) => ({ iso: `2026-11-${String(i + 1).padStart(2, "0")}` }));
const opts = { field: "due_date", endField: "end_date" };
/** Each day's lanes as row ids (null for a gap). */
function laneIds(map: ReturnType<typeof calendarLanes>) {
  return Object.fromEntries([...map].map(([iso, slots]) => [iso.slice(8), slots.map((s) => s?.row.id ?? null)]));
}

describe("calendar spans", () => {
  it("covers every day from start to end, rounded and labelled at the first", () => {
    const row = { id: "bf", due_date: "2026-11-02", end_date: "2026-11-04" };
    const lanes = calendarLanes([row], opts, fields, grid);
    expect(laneIds(lanes)).toEqual({ "02": ["bf"], "03": ["bf"], "04": ["bf"] });
    expect(lanes.get("2026-11-02")![0]).toEqual({ row, start: true, end: false, label: true });
    expect(lanes.get("2026-11-03")![0]).toEqual({ row, start: false, end: false, label: false });
    expect(lanes.get("2026-11-04")![0]).toEqual({ row, start: false, end: true, label: false });
  });

  it("splits at the week boundary and labels the new week's piece", () => {
    const row = { id: "long", due_date: "2026-11-06", end_date: "2026-11-09" };
    const lanes = calendarLanes([row], opts, fields, grid);
    expect(Object.keys(laneIds(lanes))).toEqual(["06", "07", "08", "09"]);
    expect(lanes.get("2026-11-07")![0]).toMatchObject({ start: false, end: false, label: false });
    expect(lanes.get("2026-11-08")![0]).toMatchObject({ start: false, end: false, label: true });
  });

  it("clips spans that start before the grid or end after it", () => {
    const row = { id: "wide", due_date: "2026-10-20", end_date: "2026-11-30" };
    const lanes = calendarLanes([row], opts, fields, grid);
    expect(lanes.size).toBe(14);
    expect(lanes.get("2026-11-01")![0]).toMatchObject({ start: false, label: true });
    expect(lanes.get("2026-11-14")![0]).toMatchObject({ end: false });
  });

  it("keeps a span in one lane and reuses lanes once free", () => {
    const rows = [
      { id: "a", due_date: "2026-11-02", end_date: "2026-11-05" },
      { id: "b", due_date: "2026-11-03", end_date: "2026-11-04" },
      { id: "c", due_date: "2026-11-04" },
      { id: "d", due_date: "2026-11-05" },
    ];
    expect(laneIds(calendarLanes(rows, opts, fields, grid))).toEqual({
      "02": ["a"], "03": ["a", "b"], "04": ["a", "b", "c"], "05": ["a", "d"],
    });
  });

  it("leaves a gap under a lane still taken by a longer span", () => {
    const rows = [
      { id: "short", due_date: "2026-11-02", end_date: "2026-11-02" },
      { id: "long", due_date: "2026-11-02", end_date: "2026-11-04" },
      { id: "later", due_date: "2026-11-03" },
    ];
    // longer spans take the upper lanes, so "short" goes under "long"
    expect(laneIds(calendarLanes(rows, opts, fields, grid))).toEqual({
      "02": ["long", "short"], "03": ["long", "later"], "04": ["long"],
    });
    // "second" keeps lane 1 on the 3rd though "first" has ended: a gap above it
    const staggered = [{ id: "first", due_date: "2026-11-01", end_date: "2026-11-02" }, { id: "second", due_date: "2026-11-02", end_date: "2026-11-03" }];
    expect(laneIds(calendarLanes(staggered, opts, fields, grid))).toEqual({ "01": ["first"], "02": ["first", "second"], "03": [null, "second"] });
  });

  it("falls back to one day without an end, or with an end before the start", () => {
    const rows = [
      { id: "none", due_date: "2026-11-02", end_date: null },
      { id: "back", due_date: "2026-11-05", end_date: "2026-11-03" },
      { id: "undated", due_date: null },
    ];
    expect(laneIds(calendarLanes(rows, opts, fields, grid))).toEqual({ "02": ["none"], "05": ["back"] });
    expect(laneIds(calendarLanes(rows, { field: "due_date" }, fields, grid, undefined, "2026-11-10"))).toEqual({ "02": ["none"], "05": ["back"], "10": ["undated"] });
    expect(calendarLanes(rows, opts, fields, grid).get("2026-11-02")![0]).toMatchObject({ start: true, end: true, label: true });
  });

  it("ends a Datetime span on the site's day", () => {
    const row = { id: "t", starts_at: "2026-11-02T12:00:00Z", ends_at: "2026-11-04T03:00:00Z" };
    expect(laneIds(calendarLanes([row], { field: "starts_at", endField: "ends_at" }, fields, grid, "America/Cuiaba"))).toEqual({ "02": ["t"], "03": ["t"] });
  });

  it("queries rows overlapping the grid, and rows without an end starting in it", () => {
    expect(calendarSpanFilters("due_date", "end_date", fields, "2026-11-01", "2026-12-05")).toEqual([
      [["due_date", "<=", "2026-12-05"], ["end_date", ">=", "2026-11-01"]],
      [["due_date", ">=", "2026-11-01"], ["due_date", "<=", "2026-12-05"], ["end_date", "is", "not set"]],
    ]);
    expect(calendarSpanFilters("starts_at", "ends_at", fields, "2026-11-01", "2026-12-05", "America/Cuiaba")).toEqual([
      [["starts_at", "<=", "2026-12-06T03:59:59.999999Z"], ["ends_at", ">=", "2026-11-01T04:00:00.000Z"]],
      [["starts_at", ">=", "2026-11-01T04:00:00.000Z"], ["starts_at", "<=", "2026-12-06T03:59:59.999999Z"], ["ends_at", "is", "not set"]],
    ]);
  });
});
