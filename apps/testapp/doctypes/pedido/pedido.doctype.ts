import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Pedido",
  module: "Projects",
  label: "Order",
  naming: { series: "PED-.####" },
  titleField: "client",
  submittable: true,
  trackChanges: true,
  fields: [
    { fieldname: "client", fieldtype: "Data", label: "Client", reqd: true, inListView: true },
    { fieldname: "total", fieldtype: "Currency", label: "Total", inListView: true },
    { fieldname: "workflow_state", fieldtype: "Data", label: "Workflow State", readOnly: true, inListView: true },
    { fieldname: "status", fieldtype: "Data", label: "Status", readOnly: true, inListView: true },
  ],
  permissions: [
    { role: "Project Manager", read: true, write: true, create: true, submit: true, cancel: true, report: true, export: true },
    { role: "Project Contributor", read: true, write: true, create: true },
  ],
});
