import { beforeEach, describe, expect, it, vi } from "vitest";

const calls: string[] = [];
vi.mock("$lib/api", () => ({ api: { logout: vi.fn(async () => { calls.push("logout"); }) } }));
vi.mock("$lib/notifications.svelte", () => ({ stopNotifications: vi.fn(() => calls.push("notifications")) }));
vi.mock("$lib/assignments.svelte", () => ({ stopPendingTasks: vi.fn(() => calls.push("pending tasks")) }));
vi.mock("$lib/events", () => ({ disconnectEvents: vi.fn(() => calls.push("events")) }));

import { signOut } from "./sign-out";

describe("signOut", () => {
  beforeEach(() => {
    calls.length = 0;
    vi.stubGlobal("location", { href: "/app" });
  });

  it("ends the session, then stops both counters and the events connection, and goes to the sign-in page", async () => {
    await signOut();
    expect(calls).toEqual(["logout", "notifications", "pending tasks", "events"]);
    expect(location.href).toBe("/login");
  });
});
