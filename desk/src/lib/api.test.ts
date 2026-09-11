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
