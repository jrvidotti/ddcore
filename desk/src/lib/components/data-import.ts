// The pure half of the Data Import dialog: what the browser tells the server
// about how the person's spreadsheet writes numbers and dates, who may import,
// and the file of failed rows handed back to fix and upload again.

import type { DataImportOptions, DataImportResult } from "$lib/api";
import { csvSep, toCsv } from "$lib/csv";
import { dateShape, decimalSep } from "$lib/locale";

/**
 * A spreadsheet is written in its author's locale, which is the desk's: the
 * decimal separator and the date order come from there. The CSV separator is
 * left to the server, which reads it from the header line — a file saved by
 * another program is not bound to the desk's choice.
 */
export function importOptionsFromLocale(): Pick<DataImportOptions, "decimal" | "dateOrder"> {
  const order = dateShape().order.map((p) => p[0]).join("");
  return {
    decimal: decimalSep() === "," ? "," : ".",
    dateOrder: order === "mdy" || order === "ymd" ? order : "dmy",
  };
}

/** Whether the import button shows: the import right plus create or write. */
export function canImport(perms: Record<string, boolean> | undefined): boolean {
  return !!perms?.import && (!!perms.create || !!perms.write);
}

/** The modes this user may pick, in the order the dialog offers them. */
export function importModes(perms: Record<string, boolean> | undefined): ("insert" | "update")[] {
  const out: ("insert" | "update")[] = [];
  if (perms?.import && perms.create) out.push("insert");
  if (perms?.import && perms.write) out.push("update");
  return out;
}

/**
 * The rows that failed, as the file they came from plus an "Error" column. The
 * server ignores a column it cannot match, so the same file imports again once
 * the rows are fixed.
 */
export function errorRowsCsv(res: DataImportResult, errorHeader: string, sep: string = csvSep()): string {
  const failed = res.rows.filter((r) => r.status === "error");
  return toCsv(
    [...res.headers, errorHeader],
    failed.map((r) => [...(r.cells ?? res.headers.map(() => "")), r.message ?? ""]),
    sep,
  );
}

/** The URL of the header-only CSV that imports new records. */
export function templateUrl(doctype: string, sep: string = csvSep()): string {
  return `/api/data-import/${encodeURIComponent(doctype)}/template?sep=${encodeURIComponent(sep)}`;
}
