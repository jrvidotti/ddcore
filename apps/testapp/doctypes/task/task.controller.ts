import { defineController, _ } from "@ddcore/sdk";
import type { Task } from "../../.ddcore/types";
import { recalculateProgress } from "../../services/projects";

/** The status the due date implies: used on create and on reopen. */
function openStatusFor(doc: Task): "Open" | "Overdue" {
  return doc.due_date && doc.due_date < ddcore.utils.today() ? "Overdue" : "Open";
}

function result(doc: Task) {
  return { status: doc.status, completed_at: doc.completed_at };
}

export default defineController<Task>("Task", {
  beforeInsert(doc) {
    doc.status = "Open";
    doc.completed_at = null;
  },

  validate(doc) {
    const projectStart = ddcore.db.getValue<string>("Project", doc.project!, "start_date");
    if (projectStart && doc.due_date && doc.due_date < projectStart) {
      ddcore.throw(_("The due date cannot be earlier than the project start ({0}).", [projectStart]), {
        title: _("Invalid due date"),
      });
    }
  },

  afterInsert(doc) {
    recalculateProgress(doc.project!);
  },

  onUpdate(doc) {
    // moving a task changes the progress of the project it came from too
    const previous = doc.getDocBeforeSave();
    if (previous && previous.project && previous.project !== doc.project) recalculateProgress(previous.project);
    recalculateProgress(doc.project!);
  },

  afterDelete(doc) {
    recalculateProgress(doc.project!);
  },

  methods: {
    // the three transitions are idempotent and write through the normal lifecycle
    start(doc) {
      if (doc.status !== "In progress") {
        doc.status = "In progress";
        doc.completed_at = null;
        doc.save();
      }
      return result(doc);
    },

    complete(doc) {
      if (doc.status !== "Completed") {
        doc.status = "Completed";
        doc.completed_at = ddcore.utils.now();
        doc.save();
      }
      return result(doc);
    },

    reopen(doc) {
      const target = openStatusFor(doc);
      if (doc.status !== target || doc.completed_at) {
        doc.status = target;
        doc.completed_at = null;
        doc.save();
      }
      return result(doc);
    },
  },
});
