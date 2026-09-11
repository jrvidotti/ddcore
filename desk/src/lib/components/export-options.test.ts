import { describe, expect, it } from "vitest";
import { exportUrl, supportsChildren } from "./export-options";
import { useLocale } from "../locale.test";

const params = (url: string) => new URLSearchParams(url.split("?")[1]);

describe("export URL", () => {
  it("carries the list's filters, so the file and the screen agree", () => {
    const p = params(exportUrl({
      doctype: "Task",
      format: "csv",
      filters: [["status", "=", "Open"], ["docstatus", "=", 0]],
    }));
    expect(JSON.parse(p.get("filters")!)).toEqual([["status", "=", "Open"], ["docstatus", "=", 0]]);
    expect(p.get("format")).toBe("csv");
  });

  it("sends the search as or_filters, separate from the filters", () => {
    const p = params(exportUrl({
      doctype: "Task",
      format: "csv",
      orFilters: [["name", "like", "%ana%"]],
    }));
    expect(JSON.parse(p.get("or_filters")!)).toEqual([["name", "like", "%ana%"]]);
    expect(p.get("filters")).toBeNull();
  });

  it("escapes a doctype with a space", () => {
    expect(exportUrl({ doctype: "Project Milestone", format: "ndjson" }))
      .toContain("/api/export/Project%20Milestone?");
  });

  it("asks for the child tables only where they fit", () => {
    expect(params(exportUrl({ doctype: "Task", format: "ndjson", children: true })).get("children")).toBe("1");
    // a CSV cannot nest children; asking anyway would only make the server refuse
    expect(params(exportUrl({ doctype: "Task", format: "csv", children: true })).get("children")).toBeNull();
    expect(supportsChildren("csv")).toBe(false);
    expect(supportsChildren("ndjson")).toBe(true);
  });

  it("sends the separator only for CSV, since only CSV has one", () => {
    useLocale("en-US", "USD");
    expect(params(exportUrl({ doctype: "Task", format: "csv" })).get("sep")).toBe(",");
    expect(params(exportUrl({ doctype: "Task", format: "ndjson" })).get("sep")).toBeNull();
  });

  it("uses the separator the reader's Excel expects", () => {
    // where the decimal mark is a comma, the column separator cannot also be
    // one — the same rule csv.ts follows for the client-side download
    useLocale("pt-BR");
    expect(params(exportUrl({ doctype: "Task", format: "csv" })).get("sep")).toBe(";");
    useLocale("en-US", "USD");
    expect(params(exportUrl({ doctype: "Task", format: "csv" })).get("sep")).toBe(",");
  });
});
