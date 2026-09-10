import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Projeto",
  module: "Projetos",
  label: "Projeto",
  naming: { field: "codigo" },
  titleField: "titulo",
  trackChanges: true,
  fields: [
    { fieldname: "codigo", fieldtype: "Data", label: "Código", reqd: true, unique: true, inListView: true },
    { fieldname: "titulo", fieldtype: "Data", label: "Título", reqd: true, inListView: true },
    { fieldname: "descricao", fieldtype: "Text", label: "Descrição" },
    { fieldname: "responsavel", fieldtype: "Link", label: "Responsável", options: "User", reqd: true, inStandardFilter: true },
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["Planejado", "Em andamento", "Concluído"], readOnly: true },
    { fieldname: "data_inicio", fieldtype: "Date", label: "Data de início", reqd: true },
    { fieldname: "data_final", fieldtype: "Date", label: "Data final" },
    { fieldname: "progresso", fieldtype: "Percent", label: "Progresso", readOnly: true },
    { fieldname: "marcos", fieldtype: "Table", label: "Marcos", options: "Marco Projeto", gridEditMode: "inline" },
  ],
  permissions: [
    { role: "Gestor de Projetos", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "Colaborador de Projetos", read: true },
  ],
});
