import { describe, expect, it } from "vitest";
import type { DocTypeMeta, Field } from "../../meta";
import { resolveCardFields } from "./card-fields";

const meta = (fields: Field[]): DocTypeMeta => ({ name: "Task", app: "testapp", label: "Task", idGeneration: {}, titleField: "subject", fields });
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
    const resolved = resolveCardFields(meta([{ fieldname: "due", fieldtype: "Date" }, ...bodyFields]), { title: "id", subtitle: "project", dateField: "modified" });
    expect(resolved).toEqual({
      title: "id", subtitle: "project", dateField: "modified", image: undefined, hasImage: false,
      keyFields: [bodyFields[0], bodyFields[1], bodyFields[3]],
      fetchFields: ["id", "project", "modified", "subject", "priority", "assignee"],
    });
  });

  it("omits the footer date when metadata has no suitable field", () => {
    const resolved = resolveCardFields(meta(bodyFields));
    expect(resolved.dateField).toBeUndefined();
    expect(resolved.fetchFields).toEqual(["subject", "priority", "project", "assignee"]);
  });

  it("fetches the card image and keeps its URL out of the key fields", () => {
    const photo: Field = { fieldname: "photo", fieldtype: "Attach Image", inListView: true };
    const resolved = resolveCardFields(meta([photo, ...bodyFields]), { image: "photo" });
    expect(resolved).toMatchObject({ image: "photo", hasImage: true });
    expect(resolved.keyFields.map((f) => f.fieldname)).toEqual(["priority", "project", "assignee"]);
    expect(resolved.fetchFields).toEqual(["subject", "photo", "priority", "project", "assignee"]);
  });

  it("defaults the image to the DocType's imageField, and lets the card override it", () => {
    const doctype = { ...meta([{ fieldname: "photo", fieldtype: "Attach Image" }, { fieldname: "logo", fieldtype: "Attach" }, ...bodyFields]), imageField: "photo" };
    expect(resolveCardFields(doctype).image).toBe("photo");
    expect(resolveCardFields(doctype, { image: "logo" }).image).toBe("logo");
  });

  it("keeps the avatar slot when the image field is above the reader's permlevel", () => {
    // applyFieldLevels removes the field from the meta, and the server drops it from the rows
    const resolved = resolveCardFields({ ...meta(bodyFields), imageField: "photo" });
    expect(resolved).toMatchObject({ image: undefined, hasImage: true });
    expect(resolved.fetchFields).not.toContain("photo");
  });
});
