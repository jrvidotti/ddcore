import { describe, expect, it } from "vitest";
import type { Field } from "../../meta";
import { dayDiff, ganttBar, ganttRangeFilters, ganttWindow, shiftAnchor, weekStart } from "./gantt-state";

const fields: Field[] = [
  { fieldname: "start_date", fieldtype: "Date" },
  { fieldname: "due_date", fieldtype: "Date" },
  { fieldname: "ends_at", fieldtype: "Datetime" },
];
const opts = { startField: "start_date", endField: "due_date", progressField: "progress" };

describe("gantt window", () => {
  it("counts days and finds the week's Sunday", () => {
    expect(dayDiff("2026-02-27", "2026-03-02")).toBe(3);
    expect(weekStart("2026-09-17")).toBe("2026-09-13");
    expect(weekStart("2026-09-13")).toBe("2026-09-13");
  });

  it("builds day, week and month windows", () => {
    const day = ganttWindow("2026-09-17", "day");
    expect([day.start, day.end, day.days, day.columns.length]).toEqual(["2026-09-06", "2026-09-26", 21, 21]);
    const week = ganttWindow("2026-09-17", "week");
    expect([week.start, week.end, week.days, week.columns[1].start]).toEqual(["2026-09-06", "2026-11-28", 84, "2026-09-13"]);
    const month = ganttWindow("2026-09-17", "month");
    expect([month.start, month.end, month.days]).toEqual(["2026-08-01", "2027-07-31", 365]);
    expect(month.columns.map((c) => c.days).slice(0, 3)).toEqual([31, 30, 31]);
  });

  it("moves the anchor by a step of each scale", () => {
    expect(shiftAnchor("2026-09-17", "day", 1)).toBe("2026-09-24");
    expect(shiftAnchor("2026-09-17", "week", -1)).toBe("2026-08-20");
    expect(shiftAnchor("2026-09-17", "month", 1)).toBe("2026-12-01");
  });

  it("queries the rows overlapping the window", () => {
    const win = ganttWindow("2026-09-17", "day");
    expect(ganttRangeFilters("start_date", "due_date", fields, win)).toEqual([
      ["start_date", "<=", "2026-09-26"],
      ["due_date", ">=", "2026-09-06"],
    ]);
    const [, end] = ganttRangeFilters("start_date", "ends_at", fields, win, "UTC");
    expect(end).toEqual(["ends_at", ">=", "2026-09-06T00:00:00.000Z"]);
  });
});

describe("gantt bars", () => {
  const win = ganttWindow("2026-09-24", "day"); // 2026-09-13 .. 2026-10-03, 21 days

  it("places a bar inside the window", () => {
    const bar = ganttBar({ start_date: "2026-09-14", due_date: "2026-09-20", progress: 40 }, opts, fields, win)!;
    expect(bar.left).toBeCloseTo((1 / 21) * 100);
    expect(bar.width).toBeCloseTo((7 / 21) * 100);
    expect([bar.clippedStart, bar.clippedEnd, bar.progress]).toEqual([false, false, 40]);
  });

  it("clips bars at the window edges", () => {
    const bar = ganttBar({ start_date: "2026-09-01", due_date: "2026-10-30" }, opts, fields, win)!;
    expect([bar.left, bar.width, bar.clippedStart, bar.clippedEnd, bar.progress]).toEqual([0, 100, true, true, null]);
  });

  it("draws a one-day bar when the end precedes the start, and clamps progress", () => {
    const bar = ganttBar({ start_date: "2026-09-13", due_date: "2026-09-01", progress: 250 }, opts, fields, win)!;
    expect([bar.left, bar.end, bar.progress]).toEqual([0, "2026-09-13", 100]);
    expect(bar.width).toBeCloseTo(100 / 21);
  });

  it("skips rows outside the window or without both dates", () => {
    expect(ganttBar({ start_date: "2026-10-04", due_date: "2026-10-05" }, opts, fields, win)).toBeNull();
    expect(ganttBar({ start_date: "2026-09-01", due_date: "2026-09-12" }, opts, fields, win)).toBeNull();
    expect(ganttBar({ start_date: "2026-09-14", due_date: null }, opts, fields, win)).toBeNull();
  });
});
