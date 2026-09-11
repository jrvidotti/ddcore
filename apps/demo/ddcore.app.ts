import { defineApp } from "@ddcore/sdk";

export default defineApp({
  name: "demo",
  title: "Demo: Projects",
  version: "0.1.0",
  roles: ["Project Manager", "Project Contributor"],
  scheduler: {
    daily: ["demo.services.tasks.markOverdue"],
  },
  desk: {
    home: "Projects",
    include: ["client/lists.ts"],
  },
});
