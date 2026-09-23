import { describe, it, expect } from "vitest";
import { coalesce } from "./coalesce";

function deferredTask() {
  const resolvers: (() => void)[] = [];
  let calls = 0;
  const task = () => { calls++; return new Promise<void>((r) => resolvers.push(r)); };
  return { task, calls: () => calls, finish: async () => { resolvers.shift()?.(); await new Promise((r) => setTimeout(r)); } };
}

describe("coalesce", () => {
  it("runs a burst at most twice, one at a time", async () => {
    const d = deferredTask();
    const run = coalesce(d.task);
    for (let i = 0; i < 100; i++) run();
    expect(d.calls()).toBe(1);
    await d.finish();
    expect(d.calls()).toBe(2);
    await d.finish();
    expect(d.calls()).toBe(2);
  });

  it("starts again after going idle", async () => {
    const d = deferredTask();
    const run = coalesce(d.task);
    const first = run();
    await d.finish();
    await first;
    run();
    expect(d.calls()).toBe(2);
  });

  it("keeps going after a failed run", async () => {
    let calls = 0;
    const run = coalesce(async () => { calls++; throw new Error("boom"); });
    await run();
    await run();
    expect(calls).toBe(2);
  });
});
