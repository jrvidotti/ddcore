import { describe, expect, it } from "vitest";
import { exportChoice, exportChoices, exportUrl, pageCell, pageTable, supportsChildren } from "./export-options";
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
      orFilters: [["id", "like", "%ana%"]],
    }));
    expect(JSON.parse(p.get("or_filters")!)).toEqual([["id", "like", "%ana%"]]);
    expect(p.get("filters")).toBeNull();
  });

  it("escapes a doctype with a space", () => {
    expect(exportUrl({ doctype: "Project Milestone", format: "ndjson" }))
      .toContain("/api/export/Project%20Milestone?");
  });

  it("asks for the columns given, in their order", () => {
    expect(JSON.parse(params(exportUrl({ doctype: "Task", format: "csv", fields: ["subject", "id", "modified"] })).get("fields")!))
      .toEqual(["subject", "id", "modified"]);
    expect(params(exportUrl({ doctype: "Task", format: "csv", fields: [] })).get("fields")).toBeNull();
    expect(params(exportUrl({ doctype: "Task", format: "ndjson" })).get("fields")).toBeNull();
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
  it("writes the page on screen in the browser, as CSV or XLSX", () => {
    expect(exportChoice("page-csv")).toEqual({ scope: "page", format: "csv", children: false });
    expect(exportChoice("page-xlsx")).toEqual({ scope: "page", format: "xlsx", children: false });
  });

  it("reads the format out of the choice, so the format asked for is the format written", () => {
    expect(exportChoice("all-ndjson")).toEqual({ scope: "all", format: "ndjson", children: false });
    expect(exportChoice("all-ndjson-children")).toEqual({ scope: "all", format: "ndjson", children: true });
    expect(exportChoice("all-csv")).toEqual({ scope: "all", format: "csv", children: false });
    expect(exportChoice("all-xlsx")).toEqual({ scope: "all", format: "xlsx", children: false });
  });

  it("offers only combinations the server can honour", () => {
    for (const c of exportChoices) {
      const o = exportChoice(c);
      expect(supportsChildren(o.format) || !o.children).toBe(true);
      // the browser builds a table, never NDJSON
      if (o.scope === "page") expect(o.format).not.toBe("ndjson");
    }
    expect(exportChoices.filter((c) => exportChoice(c).scope === "page")).toEqual(["page-csv", "page-xlsx"]);
    expect(supportsChildren("xlsx")).toBe(false);
  });

  it("asks the server for a workbook without a separator or child tables", () => {
    const p = params(exportUrl({ doctype: "Task", format: "xlsx", children: true, fields: ["id", "subject"] }));
    expect(p.get("format")).toBe("xlsx");
    expect(p.get("sep")).toBeNull();
    expect(p.get("children")).toBeNull();
    expect(JSON.parse(p.get("fields")!)).toEqual(["id", "subject"]);
  });
});

describe("page on screen", () => {
  const columns = [
    { fieldname: "customer", fieldtype: "Link", options: "Customer", label: "Customer" },
    { fieldname: "total", fieldtype: "Currency", label: "Total" },
    { fieldname: "paid", fieldtype: "Check", label: "Paid" },
    { fieldname: "due", fieldtype: "Date", label: "Due" },
    { fieldname: "paid_at", fieldtype: "Datetime", label: "Paid at" },
    { fieldname: "at", fieldtype: "Time", label: "At" },
    { fieldname: "status", fieldtype: "Select", options: "Open\nPaid", label: "Status" },
  ] as any[];
  const row = { id: "PAY-1", customer: "C-1", total: "1234.5", paid: 1, due: "2026-03-01", paid_at: "2026-03-01T12:00:00Z", at: "18:00:00", status: "Paid", modified: "2026-03-01T12:00:00Z" };
  const titleOf = (dt: string, id: string) => (dt === "Customer" && id === "C-1" ? "ACME" : undefined);

  it("keeps the CSV as it was: a Link by its title, everything else as loaded", () => {
    const t = pageTable([row], columns, { idLabel: "ID", modifiedLabel: "Modified", format: "csv", titleOf });
    expect(t.header).toEqual(["ID", "Customer", "Total", "Paid", "Due", "Paid at", "At", "Status", "Modified"]);
    expect(t.cells[0]).toEqual(["PAY-1", "ACME", "1234.5", 1, "2026-03-01", "2026-03-01T12:00:00Z", "18:00:00", "Paid", "2026-03-01T12:00:00Z"]);
  });

  it("types the workbook's cells, with a Datetime on the site's clock", () => {
    useLocale("pt-BR", "BRL", "America/Sao_Paulo");
    const t = pageTable([row], columns, { idLabel: "ID", modifiedLabel: "Modified", format: "xlsx", titleOf });
    expect(t.header).toEqual(["ID", "Customer", "Total", "Paid", "Due", "Paid at", "At", "Status", "Modified"]);
    // 2026-03-01 is day 46082; 12:00 UTC is 09:00 in São Paulo
    expect(t.cells[0]).toEqual([
      "PAY-1", "ACME", 1234.5, true,
      { serial: 46082, kind: "date" },
      { serial: 46082.375, kind: "datetime" },
      { serial: 0.75, kind: "time" },
      "Paid",
      // the exact instant Data Import checks, not a day number
      "2026-03-01T12:00:00Z",
    ]);
  });

  it("leaves an empty cell empty and an unparseable one as text", () => {
    expect(pageCell({ due: "" }, columns[3], "due", "xlsx")).toBe("");
    expect(pageCell({ due: "soon" }, columns[3], "due", "xlsx")).toBe("soon");
    expect(pageCell({ total: "n/a" }, columns[1], "total", "xlsx")).toBe("n/a");
    expect(pageCell({ paid: 0 }, columns[2], "paid", "xlsx")).toBe(false);
  });
});
