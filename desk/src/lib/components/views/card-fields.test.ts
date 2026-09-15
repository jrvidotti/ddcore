import { describe, expect, it } from "vitest";
import type { DocTypeMeta, Field } from "../../meta";
import { resolveCardFields } from "./card-fields";

const meta = (fields: Field[]): DocTypeMeta => ({ name: "Task", app: "testapp", label: "Task", naming: {}, titleField: "subject", fields });
const bodyFields: Field[] = ["subject", "priority", "project", "assignee", "notes"].map((fieldname) => ({ fieldname, fieldtype: "Data", inListView: true }));

describe("card field resolution", () => {
  it.each(["Date", "Datetime"])("fetches the first suitable %s for the footer even outside the body fields", (fieldtype) => {
    const resolved = resolveCardFields(meta([
      ...bodyFields,
      { fieldtype: "Date" },
      { fieldname: "internal_date", fieldtype: "Date", hidden: true },
      { fieldname: "due", fieldtype },
      { fieldname: "later", fieldtype: "Date" },
    ]));
    expect(resolved.dateField).toBe("due");
    expect(resolved.keyFields.map((f) => f.fieldname)).toEqual(["priority", "project", "assignee"]);
    expect(resolved.fetchFields).toEqual(["subject", "due", "priority", "project", "assignee"]);
  });

  it("removes the fallback date from the body so another key field can be shown", () => {
    const resolved = resolveCardFields(meta([{ fieldname: "due", fieldtype: "Date", inListView: true }, ...bodyFields]));
    expect(resolved.keyFields.map((f) => f.fieldname)).toEqual(["priority", "project", "assignee"]);
    expect(resolved.fetchFields.filter((name) => name === "due")).toHaveLength(1);
  });

  it("honors explicit title, subtitle and date ahead of metadata fallbacks", () => {
    const resolved = resolveCardFields(meta([{ fieldname: "due", fieldtype: "Date" }, ...bodyFields]), { title: "name", subtitle: "project", dateField: "modified" });
    expect(resolved).toEqual({
      title: "name", subtitle: "project", dateField: "modified",
      keyFields: [bodyFields[0], bodyFields[1], bodyFields[3]],
      fetchFields: ["name", "project", "modified", "subject", "priority", "assignee"],
    });
  });

  it("omits the footer date when metadata has no suitable field", () => {
    const resolved = resolveCardFields(meta(bodyFields));
    expect(resolved.dateField).toBeUndefined();
    expect(resolved.fetchFields).toEqual(["subject", "priority", "project", "assignee"]);
  });
});
