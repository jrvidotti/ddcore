import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import { notifications, refreshNotifications, startNotifications, stopNotifications } from "./notifications.svelte";
import { connectEvents, disconnectEvents } from "./events";

class FakeSource {
  static instances: FakeSource[] = [];
  listeners = new Map<string, (event: any) => void>();
  onerror: (() => void) | null = null;
  close = vi.fn();
  constructor(public url: string) { FakeSource.instances.push(this); }
  addEventListener(name: string, handler: (event: any) => void) { this.listeners.set(name, handler); }
  emit(name: string) { this.listeners.get(name)?.({ data: "{}" }); }
}

afterEach(() => {
  stopNotifications(); disconnectEvents();
  vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers(); FakeSource.instances = [];
});

function session() {
  const document = { visibilityState: "visible", addEventListener: vi.fn(), removeEventListener: vi.fn() };
  vi.stubGlobal("document", document); vi.stubGlobal("EventSource", FakeSource);
  startNotifications(); connectEvents();
  return document;
}

describe("notification session recovery", () => {
  it("reloads unread counts on opening a session, SSE reconnect, change and returning to the tab", async () => {
    const count = vi.spyOn(api.notifications, "count").mockResolvedValue(3);
    const document = session();
    await Promise.resolve(); expect(notifications.unread).toBe(3);
    const source = FakeSource.instances[0];
    source.emit("hello"); source.emit("notifications_changed");
    document.addEventListener.mock.calls[0][1]();
    expect(count).toHaveBeenCalledTimes(4);
    document.visibilityState = "hidden";
    document.addEventListener.mock.calls[0][1]();
    expect(count).toHaveBeenCalledTimes(4);
    stopNotifications();
    source.emit("notifications_changed");
    expect(count).toHaveBeenCalledTimes(4);
    expect(document.removeEventListener).toHaveBeenCalled();
  });

  it("discards a late response after logout and resets persisted client state", async () => {
    let resolve!: (count: number) => void;
    vi.spyOn(api.notifications, "count").mockReturnValue(new Promise<number>(r => { resolve = r; }));
    const pending = refreshNotifications();
    stopNotifications(); resolve(9); await pending;
    expect(notifications.unread).toBe(0);
  });

  it("keeps newer counts when responses arrive out of order", async () => {
    let resolve!: (count: number) => void;
    vi.spyOn(api.notifications, "count").mockReturnValueOnce(new Promise<number>(r => { resolve = r; })).mockResolvedValueOnce(1);
    const old = refreshNotifications(); await refreshNotifications(); resolve(9); await old;
    expect(notifications.unread).toBe(1);
  });

  it("cancels scheduled SSE reconnect when the session ends", () => {
    vi.useFakeTimers(); vi.stubGlobal("EventSource", FakeSource);
    connectEvents(); FakeSource.instances[0].onerror?.();
    disconnectEvents(); vi.advanceTimersByTime(4000);
    expect(FakeSource.instances).toHaveLength(1);
  });
});
