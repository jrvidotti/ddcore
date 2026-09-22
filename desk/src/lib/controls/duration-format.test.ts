import { describe, expect, it } from "vitest";
import { formatDuration, joinDuration, parseDuration, splitDuration } from "./duration-format";

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
