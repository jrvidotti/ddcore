import { describe, expect, it } from "vitest";
import { exportChoice, exportChoices, exportUrl, supportsChildren } from "./export-options";
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

describe("export choice", () => {
  it("keeps the page on screen a CSV the browser writes itself", () => {
    expect(exportChoice("page-csv")).toBeNull();
  });

  it("reads NDJSON out of the choice, so the format asked for is the format written", () => {
    expect(exportChoice("all-ndjson")).toEqual({ format: "ndjson", children: false });
    expect(exportChoice("all-ndjson-children")).toEqual({ format: "ndjson", children: true });
    expect(exportChoice("all-csv")).toEqual({ format: "csv", children: false });
  });

  it("offers only combinations the server can honour", () => {
    for (const c of exportChoices) {
      const o = exportChoice(c);
      if (o) expect(supportsChildren(o.format) || !o.children).toBe(true);
    }
    // the page on screen is the only client-side export, and it is CSV
    expect(exportChoices.filter((c) => exportChoice(c) === null)).toEqual(["page-csv"]);
  });
});
