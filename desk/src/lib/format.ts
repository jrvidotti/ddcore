import type { Field } from "./meta";
import { formatMonth } from "./controls/month-format.ts";
import { getLinkTitle } from "./titles.svelte";

export { formatMonth };

const currencyFmt = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });
const numberFmt = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 2 });

export const formatCurrency = (v: any) => currencyFmt.format(Number(v) || 0);
export const formatNumber = (v: any, precision?: number) =>
  precision !== undefined ? new Intl.NumberFormat("pt-BR", { minimumFractionDigits: precision, maximumFractionDigits: precision }).format(Number(v) || 0) : numberFmt.format(Number(v) || 0);

export function formatDate(v: any): string {
  if (!v) return "";
  const s = String(v).slice(0, 10);
  const [y, m, d] = s.split("-");
  return d && m && y ? `${d}/${m}/${y}` : String(v);
}

export function formatDatetime(v: any): string {
  if (!v) return "";
  const d = new Date(v);
  if (isNaN(d.getTime())) return String(v);
  return d.toLocaleString("pt-BR", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

export function formatValue(v: any, f?: Partial<Field>): string {
  if (v === null || v === undefined) return "";
  switch (f?.fieldtype) {
    case "Currency": return formatCurrency(v);
    case "Percent": return formatNumber(v, f.precision ?? 2) + "%";
    case "Float": return formatNumber(v, f.precision);
    case "Int": return String(Math.round(Number(v)));
    case "Check": return v ? "✓" : "";
    case "Date": return (f as any)?.options === "month" || (f as any)?.format === "mm/yyyy" ? formatMonth(v) : formatDate(v);
    case "Month": return formatMonth(v);
    case "Datetime": return formatDatetime(v);
    case "Link": return (f?.options ? getLinkTitle(f.options, v) : "") || String(v);
  }
  return String(v);
}

/** Parses "1.234,56" or "1234.56" into a number. */
export function parseNumber(s: string): number | null {
  if (s === null || s === undefined) return null;
  let t = String(s).trim().replace(/[R$\s%]/g, "");
  if (t === "" || t === "-") return null;
  if (t.includes(",")) t = t.replace(/\./g, "").replace(",", ".");
  const n = Number(t);
  return isNaN(n) ? null : n;
}

export function timeAgo(v: any): string {
  const d = new Date(v);
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 60) return "agora";
  if (diff < 3600) return `${Math.floor(diff / 60)} min atrás`;
  if (diff < 86400) return `${Math.floor(diff / 3600)} h atrás`;
  if (diff < 86400 * 30) return `${Math.floor(diff / 86400)} d atrás`;
  return formatDate(v);
}

export const statusColor = (v: string): string => {
  const s = String(v || "").toLowerCase();
  if (/pago|vigente|dispon|ativo|conclu|aprovad|enviado/.test(s)) return "green";
  if (/atras|cancel|rescind|vencid|erro|inativ/.test(s)) return "red";
  if (/parcial|pendente|rascunho|aguard/.test(s)) return "orange";
  if (/alugad|encerr/.test(s)) return "blue";
  return "gray";
};
