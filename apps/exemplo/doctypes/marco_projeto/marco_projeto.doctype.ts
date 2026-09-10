import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Marco Projeto",
  module: "Projetos",
  label: "Marco Projeto",
  isChild: true,
  fields: [
    { fieldname: "titulo", fieldtype: "Data", label: "Título", reqd: true, inListView: true },
    { fieldname: "data_prevista", fieldtype: "Date", label: "Data prevista", reqd: true, inListView: true },
    { fieldname: "concluido", fieldtype: "Check", label: "Concluído", default: false, inListView: true },
    { fieldname: "concluido_em", fieldtype: "Date", label: "Concluído em", mandatoryDependsOn: "doc.concluido" },
  ],
});
