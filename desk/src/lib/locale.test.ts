import { describe, it, expect, beforeEach } from "vitest";
import { boot } from "./boot.svelte";
import {
  currencyBefore, currencySymbol, dateShape, dayNamesShort, decimalSep,
  groupSep, monthNames, monthShape, resetLocale,
} from "./locale";

/** Puts the desk in a locale, the way a boot would. */
export function useLocale(lang: string, currency = "BRL", timezone = "UTC") {
  boot.data = {
    user: "Administrator", roles: [], userDoc: null, lang,
    apps: [], workspaces: [], doctypes: {}, reports: {},
    site: { name: "test", currency, timezone, dev: true, scheduler: false, version: "0.1.0" },
    loaded: Date.now(),
  };
  resetLocale();
}

beforeEach(() => useLocale("pt-BR"));

describe("separators", () => {
  it("come from the locale, not from an assumption", () => {
    useLocale("pt-BR");
    expect(decimalSep()).toBe(",");
    expect(groupSep()).toBe(".");

    useLocale("en-US", "USD");
    expect(decimalSep()).toBe(".");
    expect(groupSep()).toBe(",");
  });
});

describe("currency", () => {
  it("takes its symbol and its position from locale plus code", () => {
    useLocale("pt-BR", "BRL");
    expect(currencySymbol()).toBe("R$");
    expect(currencyBefore()).toBe(true);

    useLocale("en-US", "USD");
    expect(currencySymbol()).toBe("$");
    expect(currencyBefore()).toBe(true);

    // the symbol follows the currency code, not the language
    useLocale("en-US", "BRL");
    expect(currencySymbol()).toContain("R$");
  });
});

describe("date shape", () => {
  it("is derived, so it is not day-first everywhere", () => {
    useLocale("pt-BR");
    expect(dateShape().order).toEqual(["day", "month", "year"]);
    expect(dateShape().placeholder).toBe("dd/mm/yyyy");

    useLocale("en-US", "USD");
    expect(dateShape().order).toEqual(["month", "day", "year"]);
    expect(dateShape().placeholder).toBe("mm/dd/yyyy");
  });

  it("carries into the month shape", () => {
    useLocale("pt-BR");
    expect(monthShape().monthFirst).toBe(true);
    expect(monthShape().placeholder).toBe("mm/yyyy");
  });
});

describe("names", () => {
  it("month and weekday names come from Intl", () => {
    useLocale("pt-BR");
    expect(monthNames("long")[0]).toBe("Janeiro");
    expect(monthNames("long")[2]).toBe("Março");
    expect(dayNamesShort()).toHaveLength(7);
    expect(dayNamesShort()[0].toLowerCase()).toContain("dom");

    useLocale("en-US", "USD");
    expect(monthNames("long")[0]).toBe("January");
    expect(dayNamesShort()[0]).toBe("Sun");
  });
});

describe("memoisation", () => {
  it("is per language, and resetLocale drops it", () => {
    useLocale("pt-BR");
    const a = decimalSep();
    useLocale("en-US", "USD");
    expect(decimalSep()).not.toBe(a);
  });
});
