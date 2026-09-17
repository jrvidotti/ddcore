import { describe, it, expect } from "vitest";
import { maintenance, setMaintenance } from "./maintenance.svelte";

describe("maintenance state", () => {
  it("follows the flag and drops a stale reason when the site reopens", () => {
    setMaintenance({ enabled: true, reason: "Upgrade" });
    expect(maintenance).toEqual({ enabled: true, reason: "Upgrade" });
    setMaintenance({ enabled: false, reason: "Upgrade" });
    expect(maintenance).toEqual({ enabled: false, reason: "" });
    setMaintenance(null);
    expect(maintenance.enabled).toBe(false);
  });
});
