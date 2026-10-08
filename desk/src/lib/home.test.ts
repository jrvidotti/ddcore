import { describe, it, expect } from "vitest";
import { accountLabel, emptyHome } from "./home";

describe("emptyHome", () => {
  it("is nothing while there is a workspace to open", () => {
    expect(emptyHome({ workspaces: [{ name: "Demo" }] })).toBeNull();
    expect(emptyHome(null)).toBeNull();
  });

  it("tells a site without workspaces from a reader whose roles open none", () => {
    expect(emptyHome({ workspaces: [] })).toBe("none-declared");
    expect(emptyHome({ workspaces: [], workspacesDenied: true })).toBe("no-access");
  });
});

describe("accountLabel", () => {
  it("shows the name with the login beside it", () => {
    expect(accountLabel("ana@x.com", "Ana Souza")).toBe("Ana Souza (ana@x.com)");
  });

  it("shows the login alone when there is no other name", () => {
    expect(accountLabel("ana@x.com", "")).toBe("ana@x.com");
    expect(accountLabel("ana@x.com", "ana@x.com")).toBe("ana@x.com");
  });
});
