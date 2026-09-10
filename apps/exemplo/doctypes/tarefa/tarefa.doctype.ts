import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Tarefa",
  module: "Projetos",
  label: "Tarefa",
  naming: { field: "codigo" },
  titleField: "titulo",
  trackChanges: true,
  fields: [
    { fieldname: "codigo", fieldtype: "Data", label: "Código", reqd: true, unique: true, inListView: true },
    { fieldname: "projeto", fieldtype: "Link", label: "Projeto", options: "Projeto", reqd: true, inListView: true, inStandardFilter: true },
    { fieldname: "titulo", fieldtype: "Data", label: "Título", reqd: true, inListView: true },
    { fieldname: "descricao", fieldtype: "Text", label: "Descrição" },
    { fieldname: "responsavel", fieldtype: "Link", label: "Responsável", options: "User", reqd: true, inStandardFilter: true },
    { fieldname: "prioridade", fieldtype: "Select", label: "Prioridade", options: ["Baixa", "Média", "Alta"], default: "Média", inListView: true },
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["Aberta", "Em andamento", "Atrasada", "Concluída"], readOnly: true, inListView: true },
    { fieldname: "data_limite", fieldtype: "Date", label: "Data limite", reqd: true, inListView: true },
    { fieldname: "concluida_em", fieldtype: "Datetime", label: "Concluída em", readOnly: true },
  ],
  permissions: [
    { role: "Gestor de Projetos", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "Colaborador de Projetos", read: true, write: true, create: true, report: true },
  ],
});
