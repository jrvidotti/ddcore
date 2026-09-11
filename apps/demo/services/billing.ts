import type { Invoice } from "../.ddcore/types";

const u = () => ddcore.utils;

/**
 * Fills an invoice's schedule with `count` monthly instalments.
 *
 * The split is not `total / count` rounded: three thirds of 100.00 each round
 * to 33.33 and the schedule comes out a cent short of the invoice it belongs
 * to. `splitAmount` places the residue deliberately, on the earliest
 * instalments, so the parts always add back up to the whole.
 */
export function schedule(invoice: string, count: number): void {
  const doc = ddcore.getDoc<Invoice>("Invoice", invoice);
  const amounts = u().splitAmount(doc.total, count);
  doc.installments = amounts.map((amount, i) => ({
    number: i + 1,
    due_date: u().addMonths(doc.issue_date, i + 1),
    amount,
  })) as Invoice["installments"];
  doc.save();
}

/**
 * The balance after `months` of simple monthly interest, rounded at the site's
 * precision *each month* — which is what a statement does, and why it does not
 * equal the compounded figure computed in one step.
 */
export function balanceAfter(invoice: string, months: number): number {
  const doc = ddcore.getDoc<Invoice>("Invoice", invoice);
  const rate = u().flt(doc.monthly_interest) / 100;
  let balance = u().roundCurrency(doc.total);
  for (let i = 0; i < months; i++) balance = u().roundCurrency(balance * (1 + rate));
  return balance;
}
