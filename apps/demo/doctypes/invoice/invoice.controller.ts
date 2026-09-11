import { defineController, _ } from "@ddcore/sdk";
import type { Invoice } from "../../.ddcore/types";

const u = () => ddcore.utils;

export default defineController<Invoice>("Invoice", {
  validate(doc) {
    if (!doc.installments?.length) return;
    const sum = doc.installments.reduce((a, i) => a + u().flt(i.amount), 0);
    // roundCurrency, not roundTo(x, 2): the site says how many decimals a
    // currency has, and a JPY site says none
    if (u().roundCurrency(sum) !== u().roundCurrency(doc.total)) {
      ddcore.throw(_("The installments add up to {0}, not to the invoice total {1}.", [
        u().formatCurrency(sum),
        u().formatCurrency(doc.total),
      ]));
    }
  },
});
