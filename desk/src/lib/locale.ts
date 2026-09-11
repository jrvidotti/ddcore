// Everything regional the desk needs, derived from the boot rather than
// hard-coded: which locale to format in, which currency, which timezone.
//
// Every accessor here is *lazy*. The boot is asynchronous, so a formatter
// built at module scope is built before the answer exists — which was already
// wrong before i18n, it just happened to be wrong in the right language.
// Building on first use, and memoising per key, is what makes them correct.
//
// Formatters are memoised because `new Intl.NumberFormat` is expensive and
// list rendering calls these once per cell.

import { boot } from "./boot.svelte";

const memo = new Map<string, any>();

function cached<T>(key: string, build: () => T): T {
  if (!memo.has(key)) memo.set(key, build());
  return memo.get(key) as T;
}

/** Drops the memoised formatters. Called when the boot changes language. */
export function resetLocale() {
  memo.clear();
}

/** The BCP 47 locale the desk formats in. */
export function locale(): string {
  return boot.data?.lang || "en";
}

/** ISO 4217 code from `ddcore.json:currency`. */
export function currencyCode(): string {
  return boot.data?.site?.currency || "USD";
}

/**
 * The site's timezone. One timezone per site: a `Datetime` is an instant and
 * is shown in *the site's* zone, not the viewer's, so that `due_date < today()`
 * gives the same answer on both sides of the wire.
 */
export function timezone(): string {
  return boot.data?.site?.timezone || "UTC";
}

export function numberFmt(opts: Intl.NumberFormatOptions = {}): Intl.NumberFormat {
  return cached(`n:${locale()}:${JSON.stringify(opts)}`, () => new Intl.NumberFormat(locale(), opts));
}

export function currencyFmt(opts: Intl.NumberFormatOptions = {}): Intl.NumberFormat {
  return cached(`c:${locale()}:${currencyCode()}:${JSON.stringify(opts)}`,
    () => new Intl.NumberFormat(locale(), { style: "currency", currency: currencyCode(), ...opts }));
}

export function dateFmt(opts: Intl.DateTimeFormatOptions = {}): Intl.DateTimeFormat {
  return cached(`d:${locale()}:${timezone()}:${JSON.stringify(opts)}`,
    () => new Intl.DateTimeFormat(locale(), opts));
}

export function relativeFmt(): Intl.RelativeTimeFormat {
  return cached(`r:${locale()}`, () => new Intl.RelativeTimeFormat(locale(), { numeric: "auto" }));
}

/** The decimal separator of the current locale ("," in pt-BR, "." in en-US). */
export function decimalSep(): string {
  return cached(`sep:d:${locale()}`, () =>
    numberFmt().formatToParts(1.1).find((p) => p.type === "decimal")?.value ?? ".");
}

/** The group separator of the current locale ("." in pt-BR, "," in en-US). */
export function groupSep(): string {
  return cached(`sep:g:${locale()}`, () =>
    numberFmt().formatToParts(1000).find((p) => p.type === "group")?.value ?? ",");
}

/** The currency's symbol as this locale writes it ("R$", "$", "€"). */
export function currencySymbol(): string {
  return cached(`cur:sym:${locale()}:${currencyCode()}`, () =>
    currencyFmt().formatToParts(0).find((p) => p.type === "currency")?.value ?? currencyCode());
}

/** Whether the symbol comes before the number, as it does in pt-BR and en-US. */
export function currencyBefore(): boolean {
  return cached(`cur:pre:${locale()}:${currencyCode()}`, () => {
    const parts = currencyFmt().formatToParts(1);
    const cur = parts.findIndex((p) => p.type === "currency");
    const num = parts.findIndex((p) => p.type === "integer");
    return cur >= 0 && num >= 0 ? cur < num : true;
  });
}

/** The order the locale writes a date in, and the separator between the parts. */
export interface DateShape {
  order: ("day" | "month" | "year")[];
  sep: string;
  /** A hint for a text input: "dd/mm/yyyy", "mm/dd/yyyy", "yyyy-mm-dd"… */
  placeholder: string;
}

const PROBE = new Date(Date.UTC(2033, 10, 22)); // 2033-11-22: d, m and y all distinct

/**
 * Derives the date order from the locale instead of assuming one. Fixing
 * "dd/mm/yyyy" is the same mistake as fixing "R$": it happens to be right in
 * one place and silently wrong everywhere else.
 */
export function dateShape(): DateShape {
  return cached(`shape:${locale()}`, () => {
    const parts = new Intl.DateTimeFormat(locale(), {
      day: "2-digit", month: "2-digit", year: "numeric", timeZone: "UTC",
    }).formatToParts(PROBE);
    const order = parts.filter((p) => p.type === "day" || p.type === "month" || p.type === "year")
      .map((p) => p.type as "day" | "month" | "year");
    const sep = parts.find((p) => p.type === "literal")?.value.trim() || "/";
    const token: Record<"day" | "month" | "year", string> = { day: "dd", month: "mm", year: "yyyy" };
    const shape: ("day" | "month" | "year")[] = order.length === 3 ? order : ["day", "month", "year"];
    return { order: shape, sep, placeholder: shape.map((o) => token[o]).join(sep) };
  });
}

/** The month/year order, for the Month control. */
export function monthShape(): { monthFirst: boolean; sep: string; placeholder: string } {
  return cached(`mshape:${locale()}`, () => {
    const { order, sep } = dateShape();
    const monthFirst = order.indexOf("month") < order.indexOf("year");
    return { monthFirst, sep, placeholder: monthFirst ? `mm${sep}yyyy` : `yyyy${sep}mm` };
  });
}

/** Month names in the current locale, January first. */
export function monthNames(style: "long" | "short" = "long"): string[] {
  return cached(`months:${locale()}:${style}`, () => {
    const fmt = new Intl.DateTimeFormat(locale(), { month: style, timeZone: "UTC" });
    return Array.from({ length: 12 }, (_, m) =>
      capitalize(fmt.format(new Date(Date.UTC(2033, m, 1)))));
  });
}

/** Weekday names in the current locale, Sunday first (the calendar grid's order). */
export function dayNamesShort(): string[] {
  return cached(`days:${locale()}`, () => {
    // 2033-01-02 was a Sunday
    const fmt = new Intl.DateTimeFormat(locale(), { weekday: "short", timeZone: "UTC" });
    return Array.from({ length: 7 }, (_, i) =>
      capitalize(fmt.format(new Date(Date.UTC(2033, 0, 2 + i))).replace(/\.$/, "")));
  });
}

function capitalize(s: string): string {
  return s ? s[0].toLocaleUpperCase(locale()) + s.slice(1) : s;
}
