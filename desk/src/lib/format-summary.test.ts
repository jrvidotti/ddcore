import { describe, it, expect } from "vitest";
import { formatNumber, formatDate, formatCurrency, formatSummary } from "./format";

// A summary card is formatted by its datatype; without one, by what the value is.
describe("formatSummary", () => {
  it("shows a Data card as its text, not as 0", () => {
    expect(formatSummary({ value: "sistemas", datatype: "Data" })).toBe("sistemas");
  });
  it("shows a string with no datatype as text", () => {
    expect(formatSummary({ value: "reused" })).toBe("reused");
    expect(formatSummary({ value: null })).toBe("");
  });
  it("formats a number with no datatype as a number, as before", () => {
    expect(formatSummary({ value: 1234.5 })).toBe(formatNumber(1234.5));
  });
  it("formats the numeric datatypes", () => {
    expect(formatSummary({ value: 7, datatype: "Int" })).toBe("7");
    expect(formatSummary({ value: 10, datatype: "Currency" })).toBe(formatCurrency(10));
    expect(formatSummary({ value: 1.5, datatype: "Float" })).toBe(formatNumber(1.5));
  });
  it("formats a Date card as a civil date", () => {
    expect(formatSummary({ value: "2026-03-01", datatype: "Date" })).toBe(formatDate("2026-03-01"));
  });
});
