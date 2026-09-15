import { defineWorkflow } from "@ddcore/sdk";

export default defineWorkflow({
  name: "Pedido Approval",
  doctype: "Pedido",
  stateField: "workflow_state",
  initialState: "Draft",
  states: [
    { state: "Draft", docstatus: 0, allowEdit: "Project Contributor" },
    { state: "Pending Approval", docstatus: 0, allowEdit: "Project Manager" },
    { state: "Approved", docstatus: 1, allowEdit: "Project Manager", updateFields: { status: "Approved" } },
    { state: "Rejected", docstatus: 2, updateFields: { status: "Rejected" } },
  ],
  transitions: [
    {
      state: "Draft",
      action: "Submit for Approval",
      nextState: "Pending Approval",
      allowed: ["Project Contributor", "Project Manager"],
    },
    {
      state: "Pending Approval",
      action: "Approve",
      nextState: "Approved",
      allowed: "Project Manager",
      allowSelfApproval: false,
      condition: (doc) => (doc.total ? doc.total > 0 : true),
    },
    {
      state: "Pending Approval",
      action: "Reject",
      nextState: "Rejected",
      allowed: "Project Manager",
    },
  ],
});
