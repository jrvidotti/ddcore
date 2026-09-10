// CSV export shared by the list and the report views.
//
// The target is Excel in pt-BR, so the separator is ";" and the file carries a
// UTF-8 BOM — without it Excel reads "Endereço" as "EndereÃ§o". Quoting follows
// RFC 4180 (double the quotes, wrap the cell), *not* JSON.stringify, which
// would escape a quote as \" and leave Excel with a broken cell.

export const CSV_SEP = ";";

/** Renders one value as a CSV cell, quoting only when it has to. */
export function csvCell(v: unknown, sep: string = CSV_SEP): string {
  if (v === null || v === undefined) return "";
  const s = typeof v === "object" ? JSON.stringify(v) : String(v);
  // a leading separator/quote or any newline forces quoting
  return s.includes(sep) || /["\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

/** Joins a header and rows into a CSV document (CRLF, as spreadsheets expect). */
export function toCsv(header: unknown[], rows: unknown[][], sep: string = CSV_SEP): string {
  return [header, ...rows].map((r) => r.map((c) => csvCell(c, sep)).join(sep)).join("\r\n");
}

/** Offers `content` to the browser as a download, releasing the Object URL. */
export function downloadCsv(filename: string, content: string): void {
  const url = URL.createObjectURL(new Blob(["﻿" + content], { type: "text/csv;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  // revoke only after the click has been handled, or the download aborts
  setTimeout(() => URL.revokeObjectURL(url), 5000);
}
