// Helpers for Month/Year ("mm/aaaa") formatting, masking and parsing.

export const MONTH_NAMES_SHORT = [
  "Jan", "Fev", "Mar", "Abr", "Mai", "Jun",
  "Jul", "Ago", "Set", "Out", "Nov", "Dez",
];

export const MONTH_NAMES_FULL = [
  "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
  "Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro",
];

/** Formats an ISO date ("2026-09-01" or "2026-09") into "09/2026". */
export function formatMonth(v: any): string {
  if (!v) return "";
  const s = String(v).trim().slice(0, 10);
  if (/^\d{2}\/\d{4}$/.test(s)) return s;

  // Handle "YYYY-MM" or "YYYY-MM-DD"
  const mIso = s.match(/^(\d{4})-(\d{2})(?:-\d{2})?/);
  if (mIso) {
    const [, y, m] = mIso;
    return `${m}/${y}`;
  }

  // Handle "MM/YYYY" or "M/YYYY"
  const mSlash = s.match(/^(\d{1,2})\/(\d{4})/);
  if (mSlash) {
    const [, m, y] = mSlash;
    return `${m.padStart(2, "0")}/${y}`;
  }

  return s;
}

export interface ParsedMonth {
  year: number;
  month: number;
  iso: string; // "YYYY-MM-01"
}

/** Parses "09/2026" or "2026-09-01" into year, month and canonical ISO date. */
export function parseMonth(text: string): ParsedMonth | null {
  if (!text) return null;
  const s = String(text).trim();

  // Try "MM/YYYY"
  const mSlash = s.match(/^(\d{1,2})\/(\d{4})$/);
  if (mSlash) {
    const m = parseInt(mSlash[1], 10);
    const y = parseInt(mSlash[2], 10);
    if (m >= 1 && m <= 12 && y >= 1000 && y <= 9999) {
      const padM = String(m).padStart(2, "0");
      return { year: y, month: m, iso: `${y}-${padM}-01` };
    }
    return null;
  }

  // Try "YYYY-MM" or "YYYY-MM-DD"
  const mIso = s.match(/^(\d{4})-(\d{1,2})(?:-(\d{1,2}))?$/);
  if (mIso) {
    const y = parseInt(mIso[1], 10);
    const m = parseInt(mIso[2], 10);
    if (m >= 1 && m <= 12 && y >= 1000 && y <= 9999) {
      const padM = String(m).padStart(2, "0");
      return { year: y, month: m, iso: `${y}-${padM}-01` };
    }
    return null;
  }

  return null;
}

/** Formats typed digits into "mm/aaaa". Automatically handles leading zeros and slash insertion. */
export function maskMonthInput(raw: string): string {
  if (!raw) return "";
  const digits = raw.replace(/\D/g, "").slice(0, 6);
  if (!digits) return "";

  if (digits.length === 1) {
    const d = digits;
    // Months 2-9 cannot be preceded by 1, so auto-prefix with 0: "02/"
    if (d >= "2" && d <= "9") {
      return `0${d}/`;
    }
    return d;
  }

  let m = digits.slice(0, 2);
  const y = digits.slice(2, 6);
  const numM = parseInt(m, 10);

  if (numM === 0) m = "01";
  else if (numM > 12) m = "12";

  return y ? `${m}/${y}` : `${m}/`;
}
