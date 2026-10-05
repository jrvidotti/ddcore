import { defineDoctype } from "@ddcore/sdk";

/**
 * What the site's users report to the developers of its apps: a bug, an
 * improvement, a feature they miss. The desk's user menu sends it through
 * `POST /api/feedback`, which writes it in the platform space whatever space
 * the author works in, so only the platform's System Managers read it. The
 * author follows their own through `GET /api/feedback/mine`, never the list.
 */
export default defineDoctype({
  name: "Feedback",
  module: "Core",
  label: "Feedback",
  icon: "message-square",
  titleField: "title",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "feedback_type", fieldtype: "Select", label: "Type", options: ["Bug", "Improvement", "Feature Request"], optionColors: { Bug: "red", Improvement: "blue", "Feature Request": "purple" }, reqd: true, inListView: true, inStandardFilter: true },
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["New", "In Review", "Planned", "Done", "Won't Do"], optionColors: { New: "blue", "In Review": "orange", Planned: "purple", Done: "green", "Won't Do": "gray" }, default: "New", inListView: true, inStandardFilter: true },
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true, searchIndex: true },
    { fieldname: "description", fieldtype: "Text", label: "Description", reqd: true },

    { fieldname: "bug_section", fieldtype: "Section Break", label: "Bug", dependsOn: "doc.feedback_type == 'Bug'" },
    { fieldname: "severity", fieldtype: "Select", label: "Severity", options: ["Low", "Medium", "High", "Critical"], optionColors: { Low: "gray", Medium: "blue", High: "orange", Critical: "red" }, inStandardFilter: true },
    { fieldname: "steps_to_reproduce", fieldtype: "Text", label: "Steps to reproduce" },
    { fieldname: "expected_result", fieldtype: "Small Text", label: "Expected result" },
    { fieldname: "actual_result", fieldtype: "Small Text", label: "Actual result" },

    { fieldname: "improvement_section", fieldtype: "Section Break", label: "Improvement", dependsOn: "doc.feedback_type == 'Improvement'" },
    { fieldname: "current_behavior", fieldtype: "Text", label: "How it works today" },
    { fieldname: "suggested_improvement", fieldtype: "Text", label: "Suggested improvement" },

    { fieldname: "feature_section", fieldtype: "Section Break", label: "Feature Request", dependsOn: "doc.feedback_type == 'Feature Request'" },
    { fieldname: "problem", fieldtype: "Text", label: "Problem to solve" },
    { fieldname: "expected_benefit", fieldtype: "Text", label: "Expected benefit" },

    { fieldname: "response_section", fieldtype: "Section Break", label: "Response" },
    { fieldname: "response", fieldtype: "Small Text", label: "Response", description: "Shown to the author with the status" },

    { fieldname: "origin_section", fieldtype: "Section Break", label: "Origin", collapsible: true },
    { fieldname: "reported_by", fieldtype: "Data", label: "Reported by", readOnly: true, searchIndex: true },
    { fieldname: "reporter_name", fieldtype: "Data", label: "Reporter name", readOnly: true, inListView: true },
    { fieldname: "reporter_email", fieldtype: "Data", label: "Reporter email", readOnly: true },
    { fieldname: "source_tenant", fieldtype: "Data", label: "Tenant", readOnly: true, inStandardFilter: true },
    { fieldname: "app_version", fieldtype: "Data", label: "Version", readOnly: true },
    { fieldname: "page_url", fieldtype: "Data", label: "Page", readOnly: true },
    { fieldname: "context", fieldtype: "JSON", label: "Context", readOnly: true },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, delete: true, report: true, export: true },
  ],
});
