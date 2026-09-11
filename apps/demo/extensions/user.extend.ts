import { extendDoctype } from "@ddcore/sdk";

// User belongs to core. The demo does not fork it to add a field of its own:
// it extends it, and core knows nothing about projects.
export default extendDoctype("User", {
  fields: [
    {
      fieldname: "default_project",
      fieldtype: "Link",
      label: "Default project",
      options: "Project",
      description: "Used as the project when this user opens a task.",
      insertAfter: "user_type",
    },
  ],
  // a property setter: core decides what `last_login` is, the demo decides
  // that a site run on projects wants it in the list
  set: {
    last_login: { inListView: true },
  },
});
