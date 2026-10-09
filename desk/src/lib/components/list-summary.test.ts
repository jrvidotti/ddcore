import { describe, expect, it } from "vitest";
import { combineSummary, summaryDatatype, summaryExpression, summaryField, summaryFilters } from "./list-summary";

const fields = [
  { fieldname: "amount", fieldtype: "Currency", label: "Amount" },
  { fieldname: "qty", fieldtype: "Int", label: "Qty" },
];

describe("summaryExpression", () => {
  it("counts by default and on count", () => {
    expect(summaryExpression()).toBe("count(*) as value");
    expect(summaryExpression("count")).toBe("count(*) as value");
  });

  it("sums the named field", () => {
    expect(summaryField("sum:amount")).toBe("amount");
    expect(summaryExpression("sum:amount")).toBe("sum(amount) as value");
  });

  it("refuses an aggregate it does not know or a field that is not a fieldname", () => {
    expect(summaryExpression("avg:amount")).toBeNull();
    expect(summaryExpression("sum:amount) as value, (id")).toBeNull();
    expect(summaryExpression("sum:")).toBeNull();
  });
});

describe("summaryFilters", () => {
  const list = [["charge", "=", "c1"]];

  it("keeps the list's filters when the card has none", () => {
    expect(summaryFilters(list, {})).toEqual(list);
  });

  it("appends triples as they are", () => {
    expect(summaryFilters(list, { filters: [["amount", "<", 0]] })).toEqual([["charge", "=", "c1"], ["amount", "<", 0]]);
  });

  it("appends an object as equalities, skipping empty values", () => {
    expect(summaryFilters(list, { filters: { entry_type: "Receipt", subaccount: "" } })).toEqual([["charge", "=", "c1"], ["entry_type", "=", "Receipt"]]);
  });

  it("does not change the list's filters", () => {
    summaryFilters(list, { filters: { entry_type: "Receipt" } });
    expect(list).toEqual([["charge", "=", "c1"]]);
  });
});

describe("summaryDatatype", () => {
  it("takes the card's datatype first", () => {
    expect(summaryDatatype({ label: "N", aggregate: "sum:qty", datatype: "Float" }, fields)).toBe("Float");
  });

  it("formats a count as Int and a sum as its field", () => {
    expect(summaryDatatype({ label: "N" }, fields)).toBe("Int");
    expect(summaryDatatype({ label: "Total", aggregate: "sum:amount" }, fields)).toBe("Currency");
    expect(summaryDatatype({ label: "X", aggregate: "sum:missing" }, fields)).toBeUndefined();
  });
});

describe("combineSummary", () => {
  it("adds the value of each filter set, an empty one as 0", () => {
    expect(combineSummary([[{ value: "10.5" }], [{ value: null }], [], [{ value: 2 }]])).toBe(12.5);
  });
});
