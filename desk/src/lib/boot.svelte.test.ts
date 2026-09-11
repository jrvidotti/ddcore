import { describe, expect, it, beforeEach } from "vitest";
import { boot, siteLogo, siteName } from "./boot.svelte";

const withBoot = (site: string, apps: { desk: any }[] = []) => {
  boot.data = { site: { name: site }, apps } as any;
};

describe("the site's name and mark", () => {
  beforeEach(() => { boot.data = null; });

  it("falls back to ddcore before the boot arrives", () => {
    expect(siteName()).toBe("ddcore");
    expect(siteLogo()).toBe("d");
  });

  it("derives the mark from the site's name", () => {
    withBoot("Projects");
    expect(siteName()).toBe("Projects");
    expect(siteLogo()).toBe("P");
  });

  it("uppercases the initial, as the user avatar does", () => {
    withBoot("demo");
    expect(siteLogo()).toBe("D");
  });

  it("keeps a whole emoji, never half a surrogate pair", () => {
    withBoot("🧪 Lab");
    expect(siteLogo()).toBe("🧪");
  });

  it("prefers an app's desk.logo over the initial", () => {
    withBoot("Projects", [{ desk: null }, { desk: { logo: "📁" } }]);
    expect(siteLogo()).toBe("📁");
  });

  it("lets the first app that declares one win, as desk.home does", () => {
    withBoot("Projects", [{ desk: { logo: "A" } }, { desk: { logo: "B" } }]);
    expect(siteLogo()).toBe("A");
  });

  it("ignores a name that is only whitespace", () => {
    withBoot("   ");
    expect(siteLogo()).toBe("d");
  });
});
