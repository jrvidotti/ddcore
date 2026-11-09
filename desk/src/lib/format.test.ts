import { describe, it, expect } from "vitest";
import { fieldCurrencyPrecision, formatValue, formatCurrency } from "./format";
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
