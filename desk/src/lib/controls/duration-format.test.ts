import { describe, expect, it } from "vitest";
import {
  durationSegments, formatDuration, joinDuration, parseDuration, segmentAt, splitDuration, stepDuration, typeDigit,
} from "./duration-format";

describe("formatDuration", () => {
  it("writes what the server writes", () => {
    expect(formatDuration(0)).toBe("0s");
    expect(formatDuration(45)).toBe("45s");
    expect(formatDuration(5400)).toBe("1h 30m");
    expect(formatDuration(93784)).toBe("1d 2h 3m 4s");
  });

  it("folds a hidden unit into the next one instead of dropping it", () => {
    expect(formatDuration(93784, { hideDays: true })).toBe("26h 3m 4s");
    expect(formatDuration(93784, { hideSeconds: true })).toBe("1d 2h 3m");
    expect(formatDuration(0, { hideSeconds: true })).toBe("0m");
  });

  it("shows nothing for an unset value, which is not zero", () => {
    expect(formatDuration(null)).toBe("");
    expect(formatDuration(undefined)).toBe("");
  });
});

describe("splitDuration and joinDuration", () => {
  it("round-trips through the control's boxes", () => {
    for (const secs of [0, 59, 3600, 93784]) {
      expect(joinDuration(splitDuration(secs))).toBe(secs);
    }
  });
  it("puts the days into the hours when days are hidden", () => {
    expect(splitDuration(93784, { hideDays: true })).toMatchObject({ days: 0, hours: 26 });
  });
});

describe("parseDuration", () => {
  it("reads what a person types", () => {
    expect(parseDuration("1d 2h 30m")).toBe(95400);
    expect(parseDuration("90m")).toBe(5400);
    expect(parseDuration("1:30")).toBe(5400);
    expect(parseDuration("1:30:15")).toBe(5415);
    expect(parseDuration("45")).toBe(45);
  });
  it("reads a bare number as minutes when seconds are hidden", () => {
    expect(parseDuration("45", { hideSeconds: true })).toBe(2700);
  });
  it("answers null for what it cannot read, so nothing becomes zero by accident", () => {
    expect(parseDuration("")).toBe(null);
    expect(parseDuration("soon")).toBe(null);
  });
});

const EN = { days: "d", hours: "h", minutes: "m", seconds: "s" };
const PT = { days: "d", hours: "h", minutes: "min", seconds: "s" };

describe("durationSegments", () => {
  it("writes one box with every unit two digits wide, and where each unit is", () => {
    expect(durationSegments(9000, {}, EN)).toEqual({
      text: "00d 02h 30m 00s",
      segs: [
        { unit: "days", start: 0, end: 2 },
        { unit: "hours", start: 4, end: 6 },
        { unit: "minutes", start: 8, end: 10 },
        { unit: "seconds", start: 12, end: 14 },
      ],
    });
  });
  it("follows a translated label's width", () => {
    const { text, segs } = durationSegments(9000, {}, PT);
    expect(text).toBe("00d 02h 30min 00s");
    expect(segs[3]).toEqual({ unit: "seconds", start: 14, end: 16 });
  });
  it("drops a hidden unit and lets the first one grow", () => {
    expect(durationSegments(93784, { hideDays: true, hideSeconds: true }, EN).text).toBe("26h 03m");
    expect(durationSegments(200 * 86400, {}, EN).text).toBe("200d 00h 00m 00s");
  });
});

describe("segmentAt", () => {
  const { segs } = durationSegments(0, {}, PT);
  it("finds the unit under the caret, its label included", () => {
    expect(segmentAt(segs, 0)).toBe(0);
    expect(segmentAt(segs, 2)).toBe(0);
    expect(segmentAt(segs, 4)).toBe(1);
    expect(segmentAt(segs, 12)).toBe(2);
    expect(segmentAt(segs, 99)).toBe(3);
  });
});

describe("stepDuration", () => {
  it("moves the total by the unit, carrying, never below zero", () => {
    expect(stepDuration(59 * 60, "minutes", 1)).toBe(3600);
    expect(stepDuration(3600, "seconds", -1)).toBe(3599);
    expect(stepDuration(30, "hours", -1)).toBe(0);
    expect(stepDuration(null, "days", 1)).toBe(86400);
  });
});

describe("typeDigit", () => {
  it("replaces, then appends, then moves on", () => {
    const a = typeDigit(0, {}, "minutes", "", "4");
    expect(a).toEqual({ secs: 240, buffer: "4", advance: false });
    expect(typeDigit(a.secs, {}, "minutes", a.buffer, "5")).toEqual({ secs: 45 * 60, buffer: "", advance: true });
  });
  it("moves on at once when no second digit would fit, and caps the unit", () => {
    expect(typeDigit(0, {}, "minutes", "", "7")).toEqual({ secs: 420, buffer: "", advance: true });
    expect(typeDigit(0, {}, "hours", "", "3").advance).toBe(true);
    expect(typeDigit(0, {}, "hours", "2", "9").secs).toBe(23 * 3600);
  });
  it("lets the first unit take as many digits as are typed", () => {
    let r = typeDigit(0, { hideDays: true }, "hours", "", "1");
    r = typeDigit(r.secs, { hideDays: true }, "hours", r.buffer, "3");
    r = typeDigit(r.secs, { hideDays: true }, "hours", r.buffer, "0");
    expect(r).toEqual({ secs: 130 * 3600, buffer: "130", advance: false });
  });
});
