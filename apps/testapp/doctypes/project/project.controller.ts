import { defineController, _ } from "@ddcore/sdk";
import type { Project } from "../../.ddcore/types";

export default defineController<Project>("Project", {
  // the computed fields: how many tasks are still open, and which milestones
  // are late; filled on every load, never stored
  onLoad(doc) {
    doc.open_tasks = doc.isNew() ? 0 : ddcore.db.count("Task", { project: doc.id, status: ["!=", "Completed"] });
    const today = ddcore.utils.today();
    for (const m of doc.milestones || []) m.overdue = !m.completed && !!m.due_date && m.due_date < today;
  },

  validate(doc) {
    if (doc.end_date && doc.start_date && doc.end_date < doc.start_date) {
      ddcore.throw(_("The end date cannot be earlier than the start date."), { title: _("Invalid dates") });
    }

    for (const milestone of doc.milestones || []) {
      // a reopened milestone keeps no completion date; the other way round
      // (completed with no date) the core already blocks, through the field's
      // mandatoryDependsOn
      if (!milestone.completed) milestone.completed_on = null;
      if (milestone.completed_on && milestone.data_prevista && milestone.completed_on < doc.start_date!) {
        ddcore.throw(_("Milestone {0} cannot be completed before the project starts.", [milestone.title]), {
          title: _("Invalid milestone"),
        });
      }
    }

    // a new project starts with no tasks; from then on the derived fields are
    // maintained by services/projects.ts, called from the Task hooks
    if (doc.isNew()) {
      doc.progress = 0;
      doc.status = "Planned";
    }
  },
});
