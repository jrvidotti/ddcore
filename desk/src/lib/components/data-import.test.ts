import { describe, expect, it } from "vitest";
import type { DataImportResult } from "$lib/api";
import { canImport, errorRowsCsv, importModes, templateUrl } from "./data-import";

const result: DataImportResult = {
  doctype: "Project", mode: "insert", dryRun: true,
  file: { name: "p.csv", format: "csv", sep: ";", sha256: "x" },
  headers: ["Code", "Title", "Notes"],
  columns: [
    { index: 0, header: "Code", fieldname: "code", label: "Code", status: "mapped" },
    { index: 1, header: "Title", fieldname: "title", label: "Title", status: "mapped" },
    { index: 2, header: "Notes", status: "unknown", reason: "No field matches this column" },
  ],
  fields: [{ fieldname: "code", label: "Code" }, { fieldname: "title", label: "Title" }, { fieldname: "description", label: "Description" }],
  counts: { rows: 2, inserted: 1, updated: 0, errors: 1 },
  rows: [
    { row: 2, status: "inserted", id: "P-1" },
    { row: 3, status: "error", message: 'Title is "required"', cells: ["P-2", "", "a;b"] },
  ],
};

describe("data import", () => {
  it("gates the button and the modes on import plus create or write", () => {
    expect(canImport({ import: true, create: true })).toBe(true);
    expect(canImport({ import: true, write: true })).toBe(true);
    expect(canImport({ import: true, read: true })).toBe(false);
    expect(canImport({ create: true, write: true })).toBe(false);
    expect(canImport(undefined)).toBe(false);
    expect(importModes({ import: true, create: true, write: true })).toEqual(["insert", "update"]);
    expect(importModes({ import: true, write: true })).toEqual(["update"]);
  });

  it("hands back the failed rows with an error column, quoted", () => {
    const csv = errorRowsCsv(result, "Error", ";");
    expect(csv).toBe('Code;Title;Notes;Error\r\nP-2;;"a;b";"Title is ""required"""');
  });

  it("builds the template url", () => {
    expect(templateUrl("Sales Invoice", ";")).toBe("/api/data-import/Sales%20Invoice/template?sep=%3B");
  });
});
