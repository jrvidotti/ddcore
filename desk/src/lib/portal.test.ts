import { describe, expect, it } from "vitest";
import { editableValues, isPortalPath, landing, portalHref, portalRedirect } from "./portal";

describe("portal landing", () => {
  it("keeps a Website User inside the portal", () => {
    expect(landing("/portal", null)).toBe("/portal");
    expect(landing("/portal", "/app/Task")).toBe("/portal");
    expect(landing("/portal", "/portal/members/claims")).toBe("/portal/members/claims");
  });
  it("sends anyone else where they asked, or home", () => {
    expect(landing("/app", "/app/Task")).toBe("/app/Task");
    expect(landing("/app", null)).toBe("/app");
    expect(landing(undefined, null)).toBe("/app");
  });
  it("moves a Website User off the desk", () => {
    expect(portalRedirect(true, "/app")).toBe("/portal");
    expect(portalRedirect(true, "/app/profile")).toBe("/portal");
    expect(portalRedirect(true, "/portal/x/y")).toBeNull();
    expect(portalRedirect(true, "/login/reset")).toBeNull();
    expect(portalRedirect(false, "/app")).toBeNull();
  });
  it("recognises portal paths", () => {
    expect(isPortalPath("/portal")).toBe(true);
    expect(isPortalPath("/portals")).toBe(false);
  });
});

describe("portal forms", () => {
  it("sends only the editable fields", () => {
    expect(editableValues({ a: 1, b: 2, id: "x" }, ["a", "c"])).toEqual({ a: 1 });
    expect(editableValues({ a: 1 }, null)).toEqual({});
  });
  it("builds page paths", () => {
    expect(portalHref("members", "claims")).toBe("/portal/members/claims");
    expect(portalHref("members", "claims", "new")).toBe("/portal/members/claims/new");
    expect(portalHref("members", "claims", "a b")).toBe("/portal/members/claims/a%20b");
  });
});
