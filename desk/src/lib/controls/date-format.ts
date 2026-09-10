// Pure helper functions for Brazilian Date ("dd/mm/aaaa") formatting, parsing, masking and calendar grids.

export const DAY_NAMES_SHORT = ["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"];

export const MONTH_NAMES = [
  "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
  "Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro",
];

/** Formats an ISO date ("2026-03-01") into "01/03/2026". */
export function formatDateBr(v: any): string {
  if (!v) return "";
  const s = String(v).trim().slice(0, 10);
  if (/^\d{2}\/\d{2}\/\d{4}$/.test(s)) return s;

  const mIso = s.match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (mIso) {
    const [, y, m, d] = mIso;
    return `${d}/${m}/${y}`;
  }

  const mSlash = s.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})/);
  if (mSlash) {
    const [, d, m, y] = mSlash;
    return `${d.padStart(2, "0")}/${m.padStart(2, "0")}/${y}`;
  }

  return s;
}

export function daysInMonth(year: number, month: number): number {
  return new Date(year, month, 0).getDate();
}

export interface ParsedDate {
  year: number;
  month: number;
  day: number;
  iso: string; // "YYYY-MM-DD"
}

/** Parses "01/03/2026" or "2026-03-01" into year, month, day and ISO string. */
export function parseDateBr(text: string): ParsedDate | null {
  if (!text) return null;
  const s = String(text).trim();

  // Try "DD/MM/YYYY"
  const mSlash = s.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})$/);
  if (mSlash) {
    const d = parseInt(mSlash[1], 10);
    const m = parseInt(mSlash[2], 10);
    const y = parseInt(mSlash[3], 10);
    if (m >= 1 && m <= 12 && y >= 1000 && y <= 9999) {
      const maxDays = daysInMonth(y, m);
      if (d >= 1 && d <= maxDays) {
        const padD = String(d).padStart(2, "0");
        const padM = String(m).padStart(2, "0");
        return { year: y, month: m, day: d, iso: `${y}-${padM}-${padD}` };
      }
    }
    return null;
  }

  // Try "YYYY-MM-DD"
  const mIso = s.match(/^(\d{4})-(\d{1,2})-(\d{1,2})$/);
  if (mIso) {
    const y = parseInt(mIso[1], 10);
    const m = parseInt(mIso[2], 10);
    const d = parseInt(mIso[3], 10);
    if (m >= 1 && m <= 12 && y >= 1000 && y <= 9999) {
      const maxDays = daysInMonth(y, m);
      if (d >= 1 && d <= maxDays) {
        const padD = String(d).padStart(2, "0");
        const padM = String(m).padStart(2, "0");
        return { year: y, month: m, day: d, iso: `${y}-${padM}-${padD}` };
      }
    }
    return null;
  }

  return null;
}

/** Formats typed digits into "dd/mm/aaaa" with auto slash insertion. */
export function maskDateInput(raw: string): string {
  if (!raw) return "";
  const digits = raw.replace(/\D/g, "").slice(0, 8);
  if (!digits) return "";

  if (digits.length === 1) {
    const d = digits;
    if (d >= "4" && d <= "9") return `0${d}/`;
    return d;
  }

  let dayStr = digits.slice(0, 2);
  let dayNum = parseInt(dayStr, 10);
  if (dayNum === 0) dayStr = "01";
  else if (dayNum > 31) dayStr = "31";

  if (digits.length === 2) {
    return `${dayStr}/`;
  }

  if (digits.length === 3) {
    const mFirst = digits[2];
    if (mFirst >= "2" && mFirst <= "9") {
      return `${dayStr}/0${mFirst}/`;
    }
    return `${dayStr}/${mFirst}`;
  }

  let monthStr = digits.slice(2, 4);
  let monthNum = parseInt(monthStr, 10);
  if (monthNum === 0) monthStr = "01";
  else if (monthNum > 12) monthStr = "12";

  if (digits.length === 4) {
    return `${dayStr}/${monthStr}/`;
  }

  const yearStr = digits.slice(4, 8);
  return `${dayStr}/${monthStr}/${yearStr}`;
}

export interface CalendarDay {
  day: number;
  month: number;
  year: number;
  isCurrentMonth: boolean;
  iso: string;
}

/** Returns 35 or 42 days grid for a calendar month. */
export function getCalendarDays(year: number, month: number): CalendarDay[] {
  const firstDayOfWeek = new Date(year, month - 1, 1).getDay(); // 0 = Sun, 1 = Mon ...
  const daysInCurrent = daysInMonth(year, month);
  const prevMonth = month === 1 ? 12 : month - 1;
  const prevYear = month === 1 ? year - 1 : year;
  const daysInPrev = daysInMonth(prevYear, prevMonth);

  const nextMonth = month === 12 ? 1 : month + 1;
  const nextYear = month === 12 ? year + 1 : year;

  const days: CalendarDay[] = [];

  // Previous month overflow days
  for (let i = firstDayOfWeek - 1; i >= 0; i--) {
    const d = daysInPrev - i;
    const padD = String(d).padStart(2, "0");
    const padM = String(prevMonth).padStart(2, "0");
    days.push({
      day: d,
      month: prevMonth,
      year: prevYear,
      isCurrentMonth: false,
      iso: `${prevYear}-${padM}-${padD}`,
    });
  }

  // Current month days
  for (let d = 1; d <= daysInCurrent; d++) {
    const padD = String(d).padStart(2, "0");
    const padM = String(month).padStart(2, "0");
    days.push({
      day: d,
      month,
      year,
      isCurrentMonth: true,
      iso: `${year}-${padM}-${padD}`,
    });
  }

  // Next month overflow days (fill to multiple of 7, at least 35 or 42)
  const remaining = (7 - (days.length % 7)) % 7;
  const totalWanted = days.length + remaining < 35 ? 35 : days.length + remaining;
  const needNext = totalWanted - days.length;

  for (let d = 1; d <= needNext; d++) {
    const padD = String(d).padStart(2, "0");
    const padM = String(nextMonth).padStart(2, "0");
    days.push({
      day: d,
      month: nextMonth,
      year: nextYear,
      isCurrentMonth: false,
      iso: `${nextYear}-${padM}-${padD}`,
    });
  }

  return days;
}
