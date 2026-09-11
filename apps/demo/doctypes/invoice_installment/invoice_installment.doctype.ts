import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Invoice Installment",
  module: "Projects",
  label: "Invoice Installment",
  isChild: true,
  fields: [
    { fieldname: "number", fieldtype: "Int", label: "Number", reqd: true, inListView: true },
    { fieldname: "due_date", fieldtype: "Date", label: "Due date", reqd: true, inListView: true },
    { fieldname: "amount", fieldtype: "Currency", label: "Amount", reqd: true, inListView: true },
  ],
});
