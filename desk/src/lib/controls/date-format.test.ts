import { describe, it, expect } from "vitest";
import { useLocale } from "../locale.test";
import {
  daysInMonth, formatDateLocal, getCalendarDays, maskDateInput, parseDateLocal, stepDate,
} from "./date-format";
import { formatMonth, maskMonthInput, parseMonth, stepMonth } from "./month-format";

// This used to be date-format.test.mjs, asserting "dd/mm/yyyy" and Portuguese
// month names. It is now parameterised by locale — and it runs under vitest,
// where the rest of the suite is, so `npm test` actually covers it.

describe("formatDateLocal", () => {
  it("writes the date the way the locale does", () => {
    useLocale("pt-BR");
    expect(formatDateLocal("2026-03-01")).toBe("01/03/2026");
    expect(formatDateLocal("2026-12-31")).toBe("31/12/2026");

    useLocale("en-US", "USD");
    expect(formatDateLocal("2026-03-01")).toBe("03/01/2026");
    expect(formatDateLocal("2026-12-31")).toBe("12/31/2026");
  });

  it("passes through empty and unreadable values", () => {
    useLocale("pt-BR");
    expect(formatDateLocal("")).toBe("");
    expect(formatDateLocal(null)).toBe("");
    expect(formatDateLocal(undefined)).toBe("");
    expect(formatDateLocal("01/03/2026")).toBe("01/03/2026");
  });
});

describe("parseDateLocal", () => {
  it("reads the locale's order", () => {
    useLocale("pt-BR");
    expect(parseDateLocal("01/03/2026")).toEqual({ year: 2026, month: 3, day: 1, iso: "2026-03-01" });
    expect(parseDateLocal("29/02/2024")).toEqual({ year: 2024, month: 2, day: 29, iso: "2024-02-29" });

    useLocale("en-US", "USD");
    expect(parseDateLocal("03/01/2026")).toEqual({ year: 2026, month: 3, day: 1, iso: "2026-03-01" });
  });

  it("accepts ISO in every locale — it is the wire format", () => {
    for (const l of ["pt-BR", "en-US"]) {
      useLocale(l, l === "pt-BR" ? "BRL" : "USD");
      expect(parseDateLocal("2026-03-01")).toEqual({ year: 2026, month: 3, day: 1, iso: "2026-03-01" });
    }
  });

  it("rejects impossible dates", () => {
    useLocale("pt-BR");
    expect(parseDateLocal("")).toBe(null);
    expect(parseDateLocal("29/02/2023")).toBe(null); // 2023 is not a leap year
    expect(parseDateLocal("32/01/2026")).toBe(null);
    expect(parseDateLocal("01/13/2026")).toBe(null); // month 13 in a day-first locale
    expect(parseDateLocal("01/00/2026")).toBe(null);
    expect(parseDateLocal("invalid")).toBe(null);
  });
});

describe("maskDateInput", () => {
  it("inserts the separator in the locale's order", () => {
    useLocale("pt-BR");
    expect(maskDateInput("")).toBe("");
    expect(maskDateInput("0")).toBe("0");
    expect(maskDateInput("4")).toBe("04/"); // no day starts with 4
    expect(maskDateInput("01")).toBe("01/");
    expect(maskDateInput("010")).toBe("01/0");
    expect(maskDateInput("013")).toBe("01/03/"); // no month starts with 3
    expect(maskDateInput("0103")).toBe("01/03/");
    expect(maskDateInput("01032")).toBe("01/03/2");
    expect(maskDateInput("01032026")).toBe("01/03/2026");
    expect(maskDateInput("01/03/2026")).toBe("01/03/2026");
  });

  it("masks month-first where the locale is month-first", () => {
    useLocale("en-US", "USD");
    expect(maskDateInput("2")).toBe("02/"); // no month starts with 2
    expect(maskDateInput("03")).toBe("03/");
    expect(maskDateInput("0301")).toBe("03/01/");
    expect(maskDateInput("03012026")).toBe("03/01/2026");
  });
});

describe("month helpers", () => {
  it("follow the locale's month/year order", () => {
    useLocale("pt-BR");
    expect(formatMonth("2026-09-01")).toBe("09/2026");
    expect(parseMonth("09/2026")).toEqual({ year: 2026, month: 9, iso: "2026-09-01" });
    expect(maskMonthInput("092026")).toBe("09/2026");
    expect(maskMonthInput("9")).toBe("09/");
    // ISO is always accepted
    expect(parseMonth("2026-09")).toEqual({ year: 2026, month: 9, iso: "2026-09-01" });
    expect(parseMonth("13/2026")).toBe(null);
  });
});

describe("daysInMonth", () => {
  it("knows February", () => {
    expect(daysInMonth(2024, 2)).toBe(29);
    expect(daysInMonth(2023, 2)).toBe(28);
    expect(daysInMonth(2026, 1)).toBe(31);
    expect(daysInMonth(2026, 4)).toBe(30);
  });
});

describe("getCalendarDays", () => {
  it("returns whole weeks with the current-month flag", () => {
    // 1 March 2026 is a Sunday
    const days = getCalendarDays(2026, 3);
    expect(days.length % 7).toBe(0);
    expect(days.length).toBeGreaterThanOrEqual(35);
    expect(days[0].iso).toBe("2026-03-01");
    expect(days[0].isCurrentMonth).toBe(true);

    // 1 April 2026 is a Wednesday: three overflow days from March
    const apr = getCalendarDays(2026, 4);
    expect(apr[0].isCurrentMonth).toBe(false);
    expect(apr[0].month).toBe(3);
    expect(apr[3].isCurrentMonth).toBe(true);
    expect(apr[3].day).toBe(1);
  });
});

describe("stepDate", () => {
  const TODAY = "2026-09-16";

  it("steps the part under the caret, in the locale's order", () => {
    useLocale("pt-BR");
    expect(stepDate("16/09/2026", 0, 1, TODAY)).toEqual({ text: "17/09/2026", iso: "2026-09-17", start: 0, end: 2 });
    expect(stepDate("16/09/2026", 2, 1, TODAY)?.text).toBe("17/09/2026"); // right after the day still counts
    expect(stepDate("16/09/2026", 4, -1, TODAY)).toEqual({ text: "16/08/2026", iso: "2026-08-16", start: 3, end: 5 });
    expect(stepDate("16/09/2026", 10, 1, TODAY)).toEqual({ text: "16/09/2027", iso: "2027-09-16", start: 6, end: 10 });

    useLocale("en-US", "USD");
    expect(stepDate("09/16/2026", 0, 1, TODAY)).toEqual({ text: "10/16/2026", iso: "2026-10-16", start: 0, end: 2 });
    expect(stepDate("09/16/2026", 4, 1, TODAY)).toEqual({ text: "09/17/2026", iso: "2026-09-17", start: 3, end: 5 });
  });

  it("wraps each part without carrying, and clamps the day", () => {
    useLocale("pt-BR");
    expect(stepDate("30/09/2026", 0, 1, TODAY)?.iso).toBe("2026-09-01");
    expect(stepDate("01/09/2026", 0, -1, TODAY)?.iso).toBe("2026-09-30");
    expect(stepDate("16/12/2026", 3, 1, TODAY)?.iso).toBe("2026-01-16");
    expect(stepDate("31/01/2026", 3, 1, TODAY)?.iso).toBe("2026-02-28");
    expect(stepDate("29/02/2024", 6, 1, TODAY)?.iso).toBe("2025-02-28");
  });

  it("fills an empty input with today, and leaves a half-typed one alone", () => {
    useLocale("pt-BR");
    expect(stepDate("", 0, 1, TODAY)).toEqual({ text: "16/09/2026", iso: "2026-09-16", start: 0, end: 2 });
    expect(stepDate("16/0", 4, 1, TODAY)).toBe(null);
  });

  it("reads a pasted ISO in its own order", () => {
    useLocale("pt-BR");
    expect(stepDate("2026-09-16", 0, 1, TODAY)).toEqual({ text: "16/09/2027", iso: "2027-09-16", start: 6, end: 10 });
  });
});

describe("stepMonth", () => {
  const TODAY = "2026-09-16";

  it("steps the month or the year under the caret", () => {
    useLocale("pt-BR");
    expect(stepMonth("09/2026", 0, 1, TODAY)).toEqual({ text: "10/2026", iso: "2026-10-01", start: 0, end: 2 });
    expect(stepMonth("09/2026", 5, -1, TODAY)).toEqual({ text: "09/2025", iso: "2025-09-01", start: 3, end: 7 });
    expect(stepMonth("12/2026", 1, 1, TODAY)?.iso).toBe("2026-01-01");
    expect(stepMonth("", 0, 1, TODAY)?.text).toBe("09/2026");
    expect(stepMonth("0", 1, 1, TODAY)).toBe(null);
  });
});
