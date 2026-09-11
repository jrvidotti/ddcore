import { defineForm, ddcore } from "@ddcore/desk-sdk";
import type { Project } from "../../.ddcore/types";

defineForm<Project>("Project", {
  refresh(frm) {
    if (frm.isNew) return;
    frm.addIndicator(__("Progress: {0}%", [ddcore.format.number(frm.doc.progress, 2)]), progressColor(frm.doc.progress));
    frm.addButton(__("Tasks"), () => ddcore.route(`/app/Task?project=${encodeURIComponent(frm.doc.name)}`));
  },
});

function progressColor(progress: number | null) {
  if (!progress) return "gray";
  return progress >= 100 ? "green" : "blue";
}
