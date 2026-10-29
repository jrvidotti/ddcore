import { describe, expect, it } from "vitest";
import { formatTime, maskTimeInput, parseTime, stepTime, timeSlots } from "./time-format";

describe("formatTime", () => {
  it("drops zero seconds and keeps the others", () => {
    expect(formatTime("08:00:00")).toBe("08:00");
    expect(formatTime("08:00:30")).toBe("08:00:30");
    expect(formatTime("8:05")).toBe("08:05");
    expect(formatTime(null)).toBe("");
    expect(formatTime("")).toBe("");
  });

  it("hands back what it cannot read", () => {
    expect(formatTime("soon")).toBe("soon");
  });
});

describe("parseTime", () => {
  it("reads HH:MM and HH:MM:SS into the wire format", () => {
    expect(parseTime("08:00")?.iso).toBe("08:00:00");
    expect(parseTime("8:5")?.iso).toBe("08:05:00");
    expect(parseTime("23:59:59")?.iso).toBe("23:59:59");
    expect(parseTime("08:00:00.250")?.iso).toBe("08:00:00");
  });

  it("refuses what is not a time of day", () => {
    expect(parseTime("24:00")).toBeNull();
    expect(parseTime("12:60")).toBeNull();
    expect(parseTime("08")).toBeNull();
    expect(parseTime("08:")).toBeNull();
    expect(parseTime("")).toBeNull();
  });
});

describe("maskTimeInput", () => {
  it("puts the colon in as the digits arrive", () => {
    expect(maskTimeInput("0")).toBe("0");
    expect(maskTimeInput("08")).toBe("08:");
    expect(maskTimeInput("080")).toBe("08:0");
    expect(maskTimeInput("0800")).toBe("08:00");
    expect(maskTimeInput("08:00")).toBe("08:00");
    expect(maskTimeInput("08003")).toBe("08:00:3");
    expect(maskTimeInput("080030")).toBe("08:00:30");
    expect(maskTimeInput("0800301")).toBe("08:00:30");
  });

  it("completes a digit that cannot start a two-digit part", () => {
    expect(maskTimeInput("8")).toBe("08:");
    expect(maskTimeInput("2")).toBe("2");
    expect(maskTimeInput("087")).toBe("08:07");
    expect(maskTimeInput("08:5")).toBe("08:5");
  });

  it("clamps each part to what a time can hold", () => {
    expect(maskTimeInput("25")).toBe("23:");
    expect(maskTimeInput("0899")).toBe("08:59");
  });

  it("ignores what is not a digit", () => {
    expect(maskTimeInput("ab")).toBe("");
    expect(maskTimeInput("")).toBe("");
  });
});

describe("stepTime", () => {
  it("steps the part under the caret and selects it", () => {
    expect(stepTime("08:00", 1, 1, "12:34:00")).toEqual({ text: "09:00", iso: "09:00:00", start: 0, end: 2 });
    expect(stepTime("08:00", 4, -1, "12:34:00")).toEqual({ text: "08:59", iso: "08:59:00", start: 3, end: 5 });
    expect(stepTime("08:00:30", 7, 1, "12:34:00")).toEqual({ text: "08:00:31", iso: "08:00:31", start: 6, end: 8 });
  });

  it("wraps each part without carrying", () => {
    expect(stepTime("23:59", 1, 1, "")?.iso).toBe("00:59:00");
    expect(stepTime("23:59", 4, 1, "")?.iso).toBe("23:00:00");
  });

  it("fills an empty input with the current time", () => {
    expect(stepTime("", 0, 1, "12:34:00")).toEqual({ text: "12:34", iso: "12:34:00", start: 0, end: 2 });
  });

  it("leaves text that does not parse yet alone", () => {
    expect(stepTime("08:", 3, 1, "12:34:00")).toBeNull();
  });
});

describe("timeSlots", () => {
  it("lists the day in steps", () => {
    const slots = timeSlots(30);
    expect(slots).toHaveLength(48);
    expect(slots[0]).toBe("00:00:00");
    expect(slots[17]).toBe("08:30:00");
    expect(slots[47]).toBe("23:30:00");
  });
});
