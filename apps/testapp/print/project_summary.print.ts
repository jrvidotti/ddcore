import { definePrintTemplate, _ } from "@ddcore/sdk";

export default definePrintTemplate({
  name: "testapp.project_summary",
  doctype: "Project",
  label: "Project Summary",
  body: (doc, b, ctx) => {
    const milestones = (doc.milestones || []).map((m: any) => [
      m.title || "",
      m.due_date ? ctx.formatDate(m.due_date) : "-",
      m.completed ? _("Completed") : _("Planned"),
    ]);

    return [
      b.header(doc.title || doc.name, {
        subtitle: _("Project Code: {0}", [doc.code]),
      }),
      b.p(doc.description || _("No project description provided.")),
      b.keyValues([
        [_("Assignee"), doc.assignee || "-"],
        [_("Status"), doc.status || "-"],
        [_("Start date"), doc.start_date ? ctx.formatDate(doc.start_date) : "-"],
        [_("End date"), doc.end_date ? ctx.formatDate(doc.end_date) : "-"],
        [_("Progress"), `${doc.progress || 0}%`],
      ]),
      b.divider(),
      b.h2(_("Milestones")),
      b.table([_("Title"), _("Due Date"), _("Status")], milestones),
    ];
  },
});
