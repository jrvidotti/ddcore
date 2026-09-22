import { describe, it, expect, vi, afterEach } from "vitest";
import { api, DDCoreError } from "./api";

// The id is the one string a user can read off a red toast and an operator can
// grep for. It has to survive the trip into the exception the desk throws, from
// either place the server puts it.
function respondWith(body: unknown, init: { status: number; headers?: Record<string, string> }) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(body), { status: init.status, headers: init.headers })),
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("DDCoreError.requestId", () => {
  it("takes the id from the error body", async () => {
    respondWith({ error: { type: "InternalError", message: "boom", requestId: "abc123def456" } }, { status: 500 });
    const err = await api.get("/api/resource/Thing").catch((e) => e);
    expect(err).toBeInstanceOf(DDCoreError);
    expect(err.requestId).toBe("abc123def456");
  });

  it("falls back to the response header when the body carries none", async () => {
    respondWith(
      { error: { type: "InternalError", message: "boom" } },
      { status: 500, headers: { "X-Request-Id": "fromheader01" } },
    );
    const err = await api.get("/api/resource/Thing").catch((e) => e);
    expect(err.requestId).toBe("fromheader01");
  });

  it("is undefined when the server said nothing", async () => {
    respondWith({ error: { type: "ValidationError", message: "check the form" } }, { status: 417 });
    const err = await api.get("/api/resource/Thing").catch((e) => e);
    expect(err.requestId).toBeUndefined();
  });
});

describe("own-user notification API", () => {
  it("preserves pagination metadata and sends unread false without dropping it", async () => {
    const page = { data: [{ id: "notice-1", read: false }], total: 21 };
    respondWith({ data: page }, { status: 200 });
    expect(await api.notifications.list({ limit: 20, offset: 20, read: false })).toEqual(page);
    expect(fetch).toHaveBeenCalledWith("/api/notifications?limit=20&offset=20&read=false", expect.objectContaining({ method: "GET" }));
  });
  it("uses authenticated count and encodes an individual notification identity for updates", async () => {
    respondWith({ data: 2 }, { status: 200 });
    expect(await api.notifications.count()).toBe(2);
    expect(fetch).toHaveBeenLastCalledWith("/api/notifications/count", expect.anything());
    respondWith({ data: { id: "a/b", read: true } }, { status: 200 });
    expect(await api.notifications.setRead("a/b", true)).toEqual({ id: "a/b", read: true });
    expect(fetch).toHaveBeenLastCalledWith("/api/notifications/a%2Fb", expect.objectContaining({ method: "PATCH", body: '{"read":true}' }));
  });
});
