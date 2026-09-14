import { defineApp } from "@ddcore/sdk";

export default defineApp({
  name: "core",
  title: "DDCore",
  description: "The framework's own DocTypes: users, roles, files, comments and versions.",
  roles: ["System Manager", "All", "Guest"],
  // Hygiene, never correctness: this only runs where the site enables the
  // scheduler, so every read path filters on expiry itself.
  scheduler: {
    hourly: ["core.services.auth.sweep"],
    daily: ["core.services.jobs.sweep", "core.services.webhooks.sweep", "core.services.audit.sweep"],
  },
  afterInstall(ctx) {
    for (const role of ["System Manager", "All", "Guest"]) {
      if (!ddcore.db.exists("Role", role)) ddcore.newDoc("Role", { role_name: role }).insert({ ignorePermissions: true });
    }
    for (const [name, full] of [["Administrator", "Administrator"], ["Guest", "Guest"]]) {
      if (!ddcore.db.exists("User", name)) {
        const u = ddcore.newDoc("User", { email: name, full_name: full, enabled: true, user_type: name === "Guest" ? "Website User" : "System User" });
        if (name === "Administrator") u.append("roles", { role: "System Manager" });
        u.insert({ ignorePermissions: true });
      }
    }
  },
});
