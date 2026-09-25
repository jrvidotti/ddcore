import { describe, expect, it } from "vitest";
import { splitVirtualId, virtualRedirect, virtualSourceLabel, virtualSubtitle } from "./virtual";

const ctx = {
  workspaces: [
    { name: "people", app: "base", sidebar: [{ doctype: "Person" }] },
    { name: "buying", app: "base", sidebar: [{ doctype: "Party" }] },
  ] as any[],
  doctypes: { Person: { app: "base", label: "Person" }, Organization: { app: "base", label: "Company" }, Party: { app: "base", label: "Party" } },
  remembered: "buying",
};
const sources = ["Person", "Organization"];

describe("splitVirtualId", () => {
  it("splits at the first separator", () => {
    expect(splitVirtualId("Person:abc:def")).toEqual({ doctype: "Person", id: "abc:def" });
  });
  it("refuses what is not a virtual id", () => {
    for (const v of ["", "abc", ":abc", "Person:", null, 42]) expect(splitVirtualId(v)).toBeNull();
  });
});

describe("virtualRedirect", () => {
  it("sends a record to its source, in the source's workspace", () => {
    expect(virtualRedirect("Party", "Person:111", sources, ctx)).toBe("/app/people/Person/111");
  });
  it("encodes the source id", () => {
    expect(virtualRedirect("Party", "Organization:a/b c", sources, ctx)).toBe("/app/people/Organization/a%2Fb%20c");
  });
  it("sends new back to the list: there is nothing to create", () => {
    expect(virtualRedirect("Party", "new", sources, ctx)).toBe("/app/buying/Party");
  });
  it("leaves an id naming no source to the form's own not-found", () => {
    expect(virtualRedirect("Party", "City:Recife", sources, ctx)).toBeNull();
    expect(virtualRedirect("Party", "111", sources, ctx)).toBeNull();
  });
});

describe("virtualSourceLabel", () => {
  it("is the source DocType's label", () => {
    expect(virtualSourceLabel("Organization:999", ctx.doctypes)).toBe("Company");
    expect(virtualSourceLabel("Unknown:1", ctx.doctypes)).toBe("Unknown");
    expect(virtualSourceLabel("plain", ctx.doctypes)).toBe("");
  });
});

describe("virtualSubtitle", () => {
  it("leads with the source and shows the source's own id", () => {
    expect(virtualSubtitle("Person:111", "Person:111 · Curitiba", ctx.doctypes)).toBe("Person · 111 · Curitiba");
    expect(virtualSubtitle("Organization:999", "", ctx.doctypes)).toBe("Company");
  });
});
