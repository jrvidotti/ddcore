import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// A stand-in for EventSource: it records the names listened to and lets the
// test dispatch a named event, which is all the SSE client relies on.
class FakeSource {
  static last: FakeSource | null = null;
  listeners = new Map<string, ((e: MessageEvent) => void)[]>();
  onerror: (() => void) | null = null;
  closed = false;
  constructor(public url: string) { FakeSource.last = this; }
  addEventListener(name: string, fn: (e: MessageEvent) => void) {
    this.listeners.set(name, [...(this.listeners.get(name) || []), fn]);
  }
  close() { this.closed = true; }
  emit(name: string, data: any) {
    for (const fn of this.listeners.get(name) || []) fn({ data: JSON.stringify(data) } as MessageEvent);
  }
}

describe("events", () => {
  beforeEach(() => { vi.resetModules(); vi.useFakeTimers(); vi.stubGlobal("EventSource", FakeSource); });
  afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

  it("delivers an app event subscribed before and after connecting, once per handler", async () => {
    const ev = await import("./events");
    const early: any[] = [], late: any[] = [];
    ev.subscribe("my_app.chat", (p) => early.push(p));
    ev.connectEvents();
    const off = ev.subscribe("my_app.chat", (p) => late.push(p));
    ev.subscribe("my_app.chat", () => {}); // a second handler adds no second listener
    FakeSource.last!.emit("my_app.chat", { session: "CS-1" });
    expect(early).toEqual([{ session: "CS-1" }]);
    expect(late).toEqual([{ session: "CS-1" }]);
    expect(FakeSource.last!.listeners.get("my_app.chat")).toHaveLength(1);
    off();
    FakeSource.last!.emit("my_app.chat", { session: "CS-2" });
    expect(late).toHaveLength(1);
    ev.disconnectEvents();
  });

  it("listens again to app events after a reconnect", async () => {
    const ev = await import("./events");
    const got: any[] = [];
    ev.connectEvents();
    ev.subscribe("my_app.chat", (p) => got.push(p));
    const first = FakeSource.last!;
    first.onerror!();
    vi.advanceTimersByTime(3000);
    expect(FakeSource.last).not.toBe(first);
    FakeSource.last!.emit("my_app.chat", 1);
    expect(got).toEqual([1]);
    ev.disconnectEvents();
  });
});
