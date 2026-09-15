import { fromDatetimeLocal, toDatetimeLocal } from "../../datetime";
import type { Field } from "../../meta";

export function isCalendarDatetime(fields: Field[], field: string): boolean {
  if (field === "creation" || field === "modified") return true;
  return fields.find((f) => f.fieldname === field)?.fieldtype === "Datetime";
}

export function groupCalendarRows<T extends Record<string, any>>(rows: T[], field: string, fields: Field[], tz?: string): Map<string, T[]> {
  const groups = new Map<string, T[]>();
  const isDatetime = isCalendarDatetime(fields, field);
  for (const row of rows) {
    const value = row[field];
    const iso = isDatetime ? toDatetimeLocal(value, tz).slice(0, 10) : String(value ?? "").slice(0, 10);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(iso)) continue;
    const group = groups.get(iso) || [];
    group.push(row);
    groups.set(iso, group);
  }
  return groups;
}

export function calendarRangeFilters(field: string, fields: Field[], startIso: string, endIso: string, tz?: string): any[][] {
  const isDatetime = isCalendarDatetime(fields, field);
  // Postgres timestamps have microsecond precision; include the final day's
  // last instant after converting its wall-clock time in the site's zone.
  const rangeStart = isDatetime ? fromDatetimeLocal(`${startIso}T00:00`, tz) : startIso;
  const rangeEnd = isDatetime ? fromDatetimeLocal(`${endIso}T23:59`, tz)?.replace(":00.000Z", ":59.999999Z") : endIso;
  return [[field, ">=", rangeStart], [field, "<=", rangeEnd]];
}
