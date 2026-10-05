import { defineMailTemplate, _ } from "@ddcore/sdk";

/**
 * The copy of a user's feedback mailed to the site's feedback addresses
 * (`"feedback": {"to": [...]}` in ddcore.json). Everything the user typed is
 * here, so the developer can read it without opening the desk; the button
 * opens the document, where the files too large to attach still are.
 */
type FeedbackMail = {
  doctype: string;
  id: string;
  feedback_type: string;
  title: string;
  description: string;
  severity?: string;
  steps_to_reproduce?: string;
  expected_result?: string;
  actual_result?: string;
  current_behavior?: string;
  suggested_improvement?: string;
  problem?: string;
  expected_benefit?: string;
  reporter_name?: string;
  reporter_email?: string;
  source_tenant?: string;
  page_url?: string;
  files: number;
  attached: number;
};

const oneLine = (s: string) => {
  const line = String(s ?? "").replace(/\s+/g, " ").trim();
  return line.length > 150 ? line.slice(0, 149) + "…" : line;
};

export default defineMailTemplate<FeedbackMail>({
  name: "core.feedback",
  subject: (d) => `[${_(d.feedback_type)}] ${oneLine(d.title)}`,
  body: (d, b) => {
    const sections: [string, string | undefined][] = [
      [_("Severity"), d.severity && _(d.severity)],
      [_("Steps to reproduce"), d.steps_to_reproduce],
      [_("Expected result"), d.expected_result],
      [_("Actual result"), d.actual_result],
      [_("How it works today"), d.current_behavior],
      [_("Suggested improvement"), d.suggested_improvement],
      [_("Problem to solve"), d.problem],
      [_("Expected benefit"), d.expected_benefit],
    ];
    const from = [d.reporter_name, d.reporter_email && `<${d.reporter_email}>`].filter(Boolean).join(" ");
    const origin: string[][] = [[_("From"), from]];
    if (d.source_tenant) origin.push([_("Tenant"), d.source_tenant]);
    if (d.page_url) origin.push([_("Page"), d.page_url]);
    return [
      b.h(oneLine(d.title)),
      b.p(d.description),
      ...sections.filter(([, v]) => v).flatMap(([label, v]) => [b.p(`${label}:`), b.p(String(v))]),
      b.rule(),
      b.table([_("Origin"), ""], origin),
      ...(d.files > d.attached ? [b.p(_("{0} of {1} files were too large to attach; open the feedback to see them.", [d.files - d.attached, d.files]))] : []),
      b.button(_("Open feedback"), ddcore.docUrl(d.doctype, d.id)),
    ];
  },
});
