import "@ddcore/sdk/test";
import type { Invoice, Project } from "../../.ddcore/types";
import { balanceAfter, schedule } from "../../services/billing";

const u = () => ddcore.utils;

function makeProject() {
  return ddcore.newDoc<Project>("Project", {
    code: "P-" + u().randomString(6),
    title: "Billing",
    assignee: "Administrator",
    start_date: u().today(),
  }).insert();
}

function makeInvoice(values: Partial<Invoice> = {}) {
  return ddcore.newDoc<Invoice>("Invoice", {
    code: "F-" + u().randomString(6),
    project: makeProject().name,
    issue_date: "2026-01-31",
    total: 100,
    ...values,
  }).insert();
}

describe("Invoice", () => {
  it("stores a Currency at the site's precision, not at the one typed", () => {
    const inv = makeInvoice({ total: 10.005 });
    // the form showed 10.01; what matters is that the column agrees
    const stored = ddcore.db.getValue("Invoice", inv.name, "total");
    expect(stored).toBe(10.01);
  });

  it("splits a total into installments that add back up to it", () => {
    const inv = makeInvoice({ total: 100 });
    schedule(inv.name, 3);

    const rows = ddcore.db.getList("Invoice Installment", {
      filters: { parent: inv.name },
      fields: ["number", "amount", "due_date"],
      orderBy: "number asc",
    });
    expect(rows).toHaveLength(3);
    expect(rows.map((r: any) => r.amount)).toEqual([33.34, 33.33, 33.33]);

    // the whole point: naive thirds would each round to 33.33 and leave 99.99
    const sum = rows.reduce((a: number, r: any) => a + r.amount, 0);
    expect(u().roundCurrency(sum)).toBe(100);
  });

  it("refuses a schedule that does not add up to its invoice", () => {
    // three naive thirds: each rounds to 33.33 and the schedule is a cent short
    expect(() => makeInvoice({
      total: 100,
      installments: [
        { number: 1, due_date: "2026-03-02", amount: 33.33 },
        { number: 2, due_date: "2026-04-02", amount: 33.33 },
        { number: 3, due_date: "2026-05-02", amount: 33.33 },
      ] as Invoice["installments"],
    })).toThrow("add up");
  });

  it("compounds interest a month at a time, as a statement does", () => {
    const inv = makeInvoice({ total: 1000, monthly_interest: 1 });
    // rounded every month, which is not the same as 1000 * 1.01^12
    expect(balanceAfter(inv.name, 1)).toBe(1010);
    expect(balanceAfter(inv.name, 3)).toBe(1030.3);
    expect(balanceAfter(inv.name, 12)).toBe(1126.84);
  });

  it("keeps a Percent as the rate it is", () => {
    const inv = makeInvoice({ monthly_interest: 1.755 });
    expect(ddcore.db.getValue("Invoice", inv.name, "monthly_interest")).toBe(1.755);
  });

  it("keeps a civil date on the day it was written", () => {
    // the end of a month, and a day Brazil's local midnight did not exist
    for (const day of ["2026-01-31", "2018-11-04"]) {
      const inv = makeInvoice({ issue_date: day });
      expect(ddcore.db.getValue("Invoice", inv.name, "issue_date")).toBe(day);
    }
  });

  it("does not drift a due date when adding months to a month end", () => {
    const inv = makeInvoice({ issue_date: "2026-01-31", total: 300 });
    schedule(inv.name, 3);
    const rows = ddcore.db.getList("Invoice Installment", {
      filters: { parent: inv.name },
      fields: ["due_date"],
      orderBy: "number asc",
    });
    expect(rows.map((r: any) => r.due_date)).toEqual(["2026-02-28", "2026-03-31", "2026-04-30"]);
  });
});
