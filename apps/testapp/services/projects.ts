// A Project's progress and status are derived from its Tasks: nobody types
// them. The Task controller calls this service after an insert, an update or
// a delete.

export type ProjectStatus = "Planned" | "In progress" | "Completed";

export function recalculateProgress(project: string): void {
  if (!project || !ddcore.db.exists("Project", project)) return;

  const tasks = ddcore.db.getAll<{ status: string }>("Task", {
    filters: { project },
    fields: ["status"],
    limit: 10000,
  });
  const completedCount = tasks.filter((t) => t.status === "Completed").length;
  const status: ProjectStatus =
    tasks.length === 0 ? "Planned" : completedCount === tasks.length ? "Completed" : "In progress";
  const progress = tasks.length === 0 ? 0 : ddcore.utils.roundTo((completedCount * 100) / tasks.length, 2);

  // dbSet writes the derived columns without re-entering the Project validate
  ddcore.getDoc("Project", project).dbSet({ progress, status });
}
