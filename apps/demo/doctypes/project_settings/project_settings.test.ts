import "@ddcore/sdk/test";
import type { ProjectSettings } from "../../.ddcore/types";

describe("Project Settings", () => {
  it("saves and reloads settings with child rows", () => {
    const settings = ddcore.getDoc<ProjectSettings>("Project Settings");
    settings.planning_enabled = false;
    settings.append("notes", { note: "Planning guidance" });
    settings.save();
    settings.reload();
    expect(settings.name).toBe("singleton");
    expect(settings.planning_enabled).toBe(false);
    expect(settings.notes.some((row) => row.note === "Planning guidance")).toBe(true);
  });
});
