import { describe, expect, it } from "vitest";
import { attachmentsQuery, hasLooseAttachments, isAudioFile, looseAttachments } from "./doc-sidebar-attachments";

describe("isAudioFile", () => {
  it("knows the voice note formats", () => {
    for (const n of ["voice-note.webm", "a.OGG", "b.oga", "c.m4a", "/private/files/x.mp3?v=1"]) expect(isAudioFile(n)).toBe(true);
    for (const n of ["shot.png", "notes.txt", "webm", "", null]) expect(isAudioFile(n)).toBe(false);
  });
});

describe("looseAttachments", () => {
  it("leaves out the files behind an Attach field", () => {
    const rows = [
      { id: "1", file_url: "/files/a.png", attached_to_field: "" },
      { id: "2", file_url: "/files/b.png", attached_to_field: "photo" },
      { id: "3", file_url: "/files/c.webm", attached_to_field: null },
    ];
    expect(looseAttachments(rows).map((r) => r.id)).toEqual(["1", "3"]);
    expect(looseAttachments(null)).toEqual([]);
  });
});

describe("attachmentsQuery", () => {
  it("asks for the document's files", () => {
    expect(attachmentsQuery("Feedback", "FB-1").filters).toEqual([["attached_to_doctype", "=", "Feedback"], ["attached_to_id", "=", "FB-1"]]);
  });
});

describe("hasLooseAttachments", () => {
  it("is a Feedback's list only, so other forms make no File request", () => {
    expect(hasLooseAttachments("Feedback")).toBe(true);
    expect(hasLooseAttachments("Sales Order")).toBe(false);
  });
});
