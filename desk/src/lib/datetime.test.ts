import { describe, it, expect } from "vitest";
import { addDays, addMonths, daysInMonth, fromDatetimeLocal, monthEnd, monthStart, parseDatetime, toDatetimeLocal, today } from "./datetime";

describe("addMonths", () => {
  it("caps day to end of month, like the server", () => {
    // internal/js/prelude.js: utils.addMonths("2026-01-31", 1) === "2026-02-28"
    expect(addMonths("2026-01-31", 1)).toBe("2026-02-28");
    expect(addMonths("2024-01-31", 1)).toBe("2024-02-29"); // leap year
    expect(addMonths("2026-03-31", -1)).toBe("2026-02-28");
    expect(addMonths("2026-05-31", 1)).toBe("2026-06-30");
  });
  it("rolls the year in both directions", () => {
    expect(addMonths("2026-12-15", 1)).toBe("2027-01-15");
    expect(addMonths("2026-01-15", -1)).toBe("2025-12-15");
    expect(addMonths("2026-01-15", -13)).toBe("2024-12-15");
    expect(addMonths("2026-01-15", 25)).toBe("2028-02-15");
  });
  it("does not alter date when n is zero", () => {
    expect(addMonths("2026-02-28", 0)).toBe("2026-02-28");
  });
  it("accepts a datetime and returns civil date only", () => {
    expect(addMonths("2026-01-31T23:30:00Z", 1)).toBe("2026-02-28");
  });
});

describe("addDays", () => {
  it("rolls month and year", () => {
    expect(addDays("2026-02-28", 1)).toBe("2026-03-01");
    expect(addDays("2024-02-28", 1)).toBe("2024-02-29");
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
    expect(addDays("2026-01-01", -1)).toBe("2025-12-31");
  });
});

describe("monthStart / monthEnd", () => {
  it("delimits the month", () => {
    expect(monthStart("2026-02-17")).toBe("2026-02-01");
    expect(monthEnd("2026-02-17")).toBe("2026-02-28");
    expect(monthEnd("2024-02-01")).toBe("2024-02-29");
    expect(monthEnd("2026-12-05")).toBe("2026-12-31");
  });
  it("with no argument uses today in the site's timezone", () => {
    expect(monthStart()).toBe(today().slice(0, 8) + "01");
    expect(monthEnd().slice(0, 7)).toBe(today().slice(0, 7));
  });
});

describe("daysInMonth", () => {
  it("handles February", () => {
    expect(daysInMonth(2026, 1)).toBe(28);
    expect(daysInMonth(2024, 1)).toBe(29);
    expect(daysInMonth(2000, 1)).toBe(29);
    expect(daysInMonth(1900, 1)).toBe(28);
  });
});

describe("today", () => {
  it("is the civil date in the site's timezone, not the browser's", () => {
    // 2027-01-01T02:30Z is still 31/12 in São Paulo (UTC-3) and already 01/01 in UTC
    const instant = new Date("2027-01-01T02:30:00Z");
    expect(today(instant, "America/Sao_Paulo")).toBe("2026-12-31");
    expect(today(instant, "UTC")).toBe("2027-01-01");
    expect(today(instant, "Asia/Tokyo")).toBe("2027-01-01");
  });
  it("defaults to the site timezone, which is UTC when no boot has answered", () => {
    const n = new Date();
    expect(today()).toBe(
      `${n.getUTCFullYear()}-${String(n.getUTCMonth() + 1).padStart(2, "0")}-${String(n.getUTCDate()).padStart(2, "0")}`,
    );
  });
});

describe("datetime-local", () => {
  // B18: a form in America/Cuiaba (UTC-4) used to render and save back a
  // value four hours off, because the browser's zone stood in for the site's.
  it("renders the instant in the site's timezone (B18)", () => {
    const iso = "2026-01-31T16:00:00.000Z";
    expect(toDatetimeLocal(iso, "America/Cuiaba")).toBe("2026-01-31T12:00");
    expect(toDatetimeLocal(iso, "UTC")).toBe("2026-01-31T16:00");
    expect(toDatetimeLocal(iso, "Asia/Tokyo")).toBe("2026-02-01T01:00");
  });

  it("round-trips through the site's timezone", () => {
    for (const tz of ["UTC", "America/Cuiaba", "America/Sao_Paulo", "Asia/Tokyo", "Europe/Lisbon"]) {
      for (const iso of ["2026-06-15T11:45:00.000Z", "2026-12-01T02:59:00.000Z", "2026-03-01T00:00:00.000Z"]) {
        expect(fromDatetimeLocal(toDatetimeLocal(iso, tz), tz)).toBe(iso);
      }
    }
  });

  it("lands on the right side of a DST change", () => {
    // Europe/Lisbon springs forward at 01:00 on 2026-03-29 (UTC+0 → UTC+1).
    // A single offset probe would read the wrong offset for one of these.
    expect(fromDatetimeLocal("2026-03-29T00:30", "Europe/Lisbon")).toBe("2026-03-29T00:30:00.000Z");
    expect(fromDatetimeLocal("2026-03-29T03:30", "Europe/Lisbon")).toBe("2026-03-29T02:30:00.000Z");
    // and back in the autumn, when the offset drops again
    expect(fromDatetimeLocal("2026-10-25T00:30", "Europe/Lisbon")).toBe("2026-10-24T23:30:00.000Z");
  });

  it("handles empty and invalid input", () => {
    expect(toDatetimeLocal(null)).toBe("");
    expect(toDatetimeLocal("")).toBe("");
    expect(toDatetimeLocal("not a date")).toBe("");
    expect(fromDatetimeLocal("")).toBe(null);
    expect(fromDatetimeLocal("xx")).toBe(null);
  });

  it("accepts the Postgres format with a space", () => {
    expect(parseDatetime("2026-01-31 12:00:00")?.getHours()).toBe(12);
  });
});
