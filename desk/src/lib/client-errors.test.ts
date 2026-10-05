import { afterEach, describe, expect, it } from "vitest";
import {
  CLIENT_ERROR_LIMIT, clearClientErrors, installClientErrorListeners, recentClientErrors,
  recordClientError, recordThrown, requestPath, resetClientErrorListeners,
} from "./client-errors";

afterEach(() => { clearClientErrors(); resetClientErrorListeners(); });

describe("client errors", () => {
  it("keeps the last ten, oldest first", () => {
    for (let i = 1; i <= CLIENT_ERROR_LIMIT + 3; i++) recordClientError({ kind: "error", message: `e${i}` });
    const list = recentClientErrors();
    expect(list).toHaveLength(CLIENT_ERROR_LIMIT);
    expect(list[0].message).toBe("e4");
    expect(list[list.length - 1].message).toBe(`e${CLIENT_ERROR_LIMIT + 3}`);
  });

  it("stamps the time, drops undefined fields and cuts long messages", () => {
    recordClientError({ kind: "request", message: "x".repeat(2000), method: "GET", path: "/api/x", status: undefined });
    const [e] = recentClientErrors();
    expect(e.at).toMatch(/^\d{4}-\d{2}-\d{2}T/);
    expect(e.message).toHaveLength(500);
    expect("status" in e).toBe(false);
    expect(e).toMatchObject({ kind: "request", method: "GET", path: "/api/x" });
  });

  it("hands out copies", () => {
    recordClientError({ kind: "error", message: "a" });
    recentClientErrors()[0].message = "changed";
    expect(recentClientErrors()[0].message).toBe("a");
  });

  it("strips the query string from a request path", () => {
    expect(requestPath("/api/resource/Lead?filters=%5B%5D&token=s")).toBe("/api/resource/Lead");
    expect(requestPath("http://h/api/x?y=1")).toBe("/api/x");
  });

  it("records window errors and rejections once, skipping errors already recorded", () => {
    const handlers: Record<string, (e: any) => void> = {};
    const win = { addEventListener: (t: string, fn: any) => { handlers[t] = fn; } } as any;
    installClientErrorListeners(win);
    installClientErrorListeners(win); // a second install adds nothing
    handlers.error({ message: "x is not a function", error: new TypeError("x is not a function") });
    handlers.error({}); // a resource that failed to load
    handlers.unhandledrejection({ reason: new Error("nope") });
    handlers.unhandledrejection({ reason: "plain" });
    const seen = new Error("request failed");
    recordThrown(seen, { kind: "request", message: "ValidationError: bad", status: 417 });
    handlers.unhandledrejection({ reason: seen });
    expect(recentClientErrors().map((e) => [e.kind, e.message])).toEqual([
      ["error", "x is not a function"],
      ["rejection", "nope"],
      ["rejection", "plain"],
      ["request", "ValidationError: bad"],
    ]);
  });
});
