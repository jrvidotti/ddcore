import { defineApp } from "@ddcore/sdk";

export default defineApp({
  name: "exemplo",
  title: "Exemplo: Projetos",
  version: "0.1.0",
  roles: ["Gestor de Projetos", "Colaborador de Projetos"],
  scheduler: {
    daily: ["exemplo.services.tarefas.marcarAtrasadas"],
  },
  desk: {
    home: "Projetos",
    include: ["client/listas.ts"],
  },
});
