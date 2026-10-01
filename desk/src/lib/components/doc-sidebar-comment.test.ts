import { describe, expect, it } from "vitest";
import { canDeleteComment, canEditComment, isCommentEdited } from "./doc-sidebar-comment";

const mine = { owner: "ana@x.com", comment_type: "Comment" };
const trail = { owner: "ana@x.com", comment_type: "Workflow" };

describe("canEditComment", () => {
  it("lets the author edit their own comment", () => {
    expect(canEditComment(mine, "ana@x.com")).toBe(true);
  });
  it("refuses anybody else", () => {
    expect(canEditComment(mine, "bia@x.com")).toBe(false);
  });
  it("refuses a timeline entry, even to its owner", () => {
    expect(canEditComment(trail, "ana@x.com")).toBe(false);
  });
  it("reads a row without a type as a comment", () => {
    expect(canEditComment({ owner: "ana@x.com" }, "ana@x.com")).toBe(true);
  });
});

describe("canDeleteComment", () => {
  it("lets the author delete their own comment", () => {
    expect(canDeleteComment(mine, "ana@x.com", false)).toBe(true);
  });
  it("lets a System Manager delete anybody's comment", () => {
    expect(canDeleteComment(mine, "root@x.com", true)).toBe(true);
  });
  it("refuses another user", () => {
    expect(canDeleteComment(mine, "bia@x.com", false)).toBe(false);
  });
  it("offers nobody the removal of a timeline entry", () => {
    expect(canDeleteComment(trail, "ana@x.com", false)).toBe(false);
    expect(canDeleteComment(trail, "root@x.com", true)).toBe(false);
  });
});

describe("isCommentEdited", () => {
  it("is false while modified still equals creation", () => {
    expect(isCommentEdited({ creation: "2026-10-01T21:42:22.460819Z", modified: "2026-10-01T21:42:22.460819Z" })).toBe(false);
  });
  it("is true once modified follows creation", () => {
    expect(isCommentEdited({ creation: "2026-10-01T21:42:22.460819Z", modified: "2026-10-01T21:43:00.000001Z" })).toBe(true);
  });
  it("is false when the row carries no modified", () => {
    expect(isCommentEdited({ creation: "2026-10-01T21:42:22.460819Z" })).toBe(false);
  });
});
