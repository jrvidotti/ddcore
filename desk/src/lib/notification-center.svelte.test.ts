import { afterEach, describe, expect, it, vi } from "vitest";
import { goto } from "$app/navigation";
import { api, type DeskNotification, type NotificationPage } from "./api";
import { NotificationCenter } from "./notification-center.svelte";
import { notifications, stopNotifications } from "./notifications.svelte";
import { confirm } from "./ui.svelte";

vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("./ui.svelte", () => ({ confirm: vi.fn() }));

const notice = { id: "n1", title: "Review", message: "New task", read: false, creation: "2026-01-01T00:00:00Z", reference_doctype: "Task", reference_id: "task1" } satisfies DeskNotification;
afterEach(() => { vi.restoreAllMocks(); vi.mocked(goto).mockReset(); vi.mocked(confirm).mockReset(); stopNotifications(); });

describe("notification center", () => {
  it("loads persisted offline notifications with page and read filters", async () => {
    const list = vi.spyOn(api.notifications, "list").mockResolvedValue({ data: [notice], total: 21 });
    const center = new NotificationCenter();
    await center.load(20, "unread");
    expect(list).toHaveBeenCalledWith({ limit: 20, offset: 20, read: false });
    expect(center.rows).toEqual([notice]); expect(center.total).toBe(21); expect(center.loading).toBe(false);
    await center.load(0, "read");
    expect(list).toHaveBeenLastCalledWith({ limit: 20, offset: 0, read: true });
  });
  it("waits for persisted read state and requests refreshed server data", async () => {
    const setRead = vi.spyOn(api.notifications, "setRead").mockResolvedValue({ ...notice, read: true });
    vi.spyOn(api.notifications, "count").mockResolvedValue(0);
    const center = new NotificationCenter();
    const revision = notifications.revision;
    await center.toggle(notice);
    expect(setRead).toHaveBeenCalledWith("n1", true);
    expect(notifications.revision).toBeGreaterThan(revision); expect(center.pending).toBe("");
    await center.toggle({ ...notice, read: true });
    expect(setRead).toHaveBeenLastCalledWith("n1", false);
  });
  it("returns to the last page when access changes remove the current page", async () => {
    vi.spyOn(api.notifications, "list").mockResolvedValue({ data: [], total: 20 });
    const center = new NotificationCenter(); center.offset = 20;
    await center.load(20, "all"); expect(center.offset).toBe(0);
  });
  it("ignores stale pages and responses after navigation away", async () => {
    let resolve!: (page: NotificationPage) => void;
    vi.spyOn(api.notifications, "list").mockReturnValueOnce(new Promise(r => { resolve = r; })).mockResolvedValueOnce({ data: [], total: 0 });
    const center = new NotificationCenter();
    const old = center.load(0, "all"); await center.load(0, "read");
    resolve({ data: [notice], total: 1 }); await old;
    expect(center.rows).toEqual([]); expect(center.total).toBe(0);
    vi.mocked(api.notifications.list).mockReturnValueOnce(new Promise(r => { resolve = r; }));
    const late = center.load(0, "all"); center.destroy();
    resolve({ data: [notice], total: 1 }); await late; expect(center.rows).toEqual([]);
  });
  it("clears previously visible document content while revalidating access", async () => {
    vi.spyOn(api.notifications, "list").mockRejectedValue(new Error("Access denied"));
    const center = new NotificationCenter(); center.rows = [notice];
    const load = center.load(0, "all"); expect(center.rows).toEqual([]);
    await load; expect(center.error).toBe("Access denied"); expect(center.loading).toBe(false);
  });
  it("asks to mark an unread notification as read before opening its document", async () => {
    const setRead = vi.spyOn(api.notifications, "setRead").mockResolvedValue({ ...notice, read: true });
    vi.spyOn(api.notifications, "count").mockResolvedValue(0);
    vi.mocked(confirm).mockResolvedValueOnce(true);
    const center = new NotificationCenter();
    await center.open(notice);
    expect(confirm).toHaveBeenCalledOnce(); expect(setRead).toHaveBeenCalledWith("n1", true);
    expect(goto).toHaveBeenCalledWith("/app/Task/task1");
  });
  it("opens without marking when the answer is no, and without asking when already read", async () => {
    const setRead = vi.spyOn(api.notifications, "setRead");
    vi.mocked(confirm).mockResolvedValueOnce(false);
    const center = new NotificationCenter();
    await center.open(notice);
    expect(setRead).not.toHaveBeenCalled(); expect(goto).toHaveBeenCalledWith("/app/Task/task1");
    await center.open({ ...notice, read: true });
    expect(confirm).toHaveBeenCalledOnce(); expect(goto).toHaveBeenCalledTimes(2);
  });
  it("stays on the page when marking as read fails", async () => {
    vi.spyOn(api.notifications, "setRead").mockRejectedValue(new Error("Access denied"));
    vi.mocked(confirm).mockResolvedValueOnce(true);
    const center = new NotificationCenter();
    await center.open(notice);
    expect(center.error).toBe("Access denied"); expect(goto).not.toHaveBeenCalled();
  });
});
