import { describe, it, expect } from "vitest";
import { displayScale, fieldCurrencyPrecision, formatNumber, formatValue, formatCurrency, unscale } from "./format";
import { useLocale } from "./locale.test";

// A Currency's precision 0 is whole units — a budget in whole reais on a
// BRL site — not "unset", which falls back to the site's 2 places (#159).
describe("a Currency field's own precision", () => {
  it("keeps 0 as whole units, and falls back to the site only when unset", () => {
    useLocale("pt-BR", "BRL");
    expect(fieldCurrencyPrecision({ fieldtype: "Currency", precision: 0 })).toBe(0);
    expect(fieldCurrencyPrecision({ fieldtype: "Currency", precision: 4 })).toBe(4);
    expect(fieldCurrencyPrecision({ fieldtype: "Currency" })).toBe(2);
  });

  it("shows a precision 0 field without cents", () => {
    useLocale("pt-BR", "BRL");
    const shown = formatValue(6000.376, { fieldtype: "Currency", precision: 0 });
    expect(shown).toBe(formatCurrency(6000, 0));
    expect(shown).not.toContain(",");
    // unset still shows the site's places
    expect(formatValue(6000.376, { fieldtype: "Currency" })).toContain(",38");
  });

  it("shows a Float with precision 0 as a whole number", () => {
    useLocale("pt-BR", "BRL");
    expect(formatValue(12.6, { fieldtype: "Float", precision: 0 })).toBe("13");
  });
});

// A form script may show a number field in another scale (thousands): the value stored does not change.
describe("a field's display scale", () => {
  const thousands: any = { fieldtype: "Currency", precision: 0, displayScale: 1000, displayPrecision: 1 };

  it("is null without a scale, and falls back to the field's precision for its places", () => {
    expect(displayScale({ fieldtype: "Currency" })).toBeNull();
    expect(displayScale({ fieldtype: "Currency", displayScale: 1 } as any)).toBeNull();
    expect(displayScale({ fieldtype: "Currency", displayScale: 0 } as any)).toBeNull();
    expect(displayScale(thousands)).toEqual({ scale: 1000, precision: 1 });
    expect(displayScale({ fieldtype: "Float", precision: 3, displayScale: 100 } as any)).toEqual({ scale: 100, precision: 3 });
  });

  it("shows value ÷ scale, a Currency with its symbol", () => {
    useLocale("pt-BR", "BRL");
    expect(formatValue(41828, thousands)).toBe(formatCurrency(41.8, 1));
    expect(formatValue(1608, { ...thousands, fieldtype: "Float" })).toBe(formatNumber(1.6, 1));
    expect(formatValue(null, thousands)).toBe("");
    // a Percent has no scale
    expect(formatValue(12.5, { fieldtype: "Percent", displayScale: 1000 } as any)).toBe(formatValue(12.5, { fieldtype: "Percent" }));
  });

  it("stores what was typed × scale, rounded to the places shown first", () => {
    useLocale("pt-BR", "BRL");
    expect(unscale(41.8, thousands)).toBe(41800);
    expect(unscale(41.84, thousands)).toBe(41800);
    expect(unscale(0.1, thousands)).toBe(100);
    expect(unscale(7, { fieldtype: "Float" })).toBe(7);
  });
});
