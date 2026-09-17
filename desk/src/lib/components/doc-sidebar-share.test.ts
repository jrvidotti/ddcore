import { describe, it, expect } from "vitest";
import { shareRightLabels, canRemoveShare } from "./doc-sidebar-share";

describe("doc-sidebar-share helpers", () => {
  it("lists the rights a share grants", () => {
    expect(shareRightLabels({ read: true, write: false, share: false })).toEqual(["Can Read"]);
    expect(shareRightLabels({ read: true, write: true, share: true })).toEqual(["Can Read", "Can Write", "Can Share"]);
    // read is implied even when the flag is off
    expect(shareRightLabels({ read: false, write: false, share: true })).toEqual(["Can Read", "Can Share"]);
  });

  it("lets a sharer or the recipient remove a share", () => {
    expect(canRemoveShare({ user: "ana@x.com" }, true, "bia@x.com")).toBe(true);
    expect(canRemoveShare({ user: "ana@x.com" }, false, "ana@x.com")).toBe(true);
    expect(canRemoveShare({ user: "ana@x.com" }, false, "bia@x.com")).toBe(false);
  });
});
