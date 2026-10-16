// Minimal test API available inside `ddcore test`.
import type {} from "./index";

declare module "./index" {
  interface DDCoreAPI {
    /** Helpers that exist only inside `ddcore test`. */
    test: {
      /**
       * Runs `fn` as `user` — their roles, User Permission scopes, shares and
       * user type — inside the test's transaction, then switches back. Every
       * `ddcore.*` call in `fn` checks permissions as that user, and
       * `ddcore.session.user` names them. Users and User Permissions the test
       * inserted are seen, and roll back with it. Returns what `fn` returns.
       */
      asUser<T>(user: string, fn: () => T): T;
      /**
       * Runs `fn` in the platform space, then returns to where the test was.
       * On a site with tenancy, the tests of an app with `tests.space:
       * "tenant"` run inside the run's scratch tenant; this is how one of them
       * writes a shared DocType, creates a Site Tenant or tests a scheduled
       * fan-out. Without tenancy it just runs `fn`. Returns what `fn` returns.
       */
      inPlatform<T>(fn: () => T): T;
      /**
       * Answers `ddcore.ai` for the rest of the test; without it, a call
       * throws. The request is built and the answer read by the same code as
       * a real call, so `respond` receives the exact body the provider would
       * (`model` defaulted, `store: false`, `text.format`…) and returns the
       * provider's JSON, or `{ status, body }` for a raw HTTP answer (an
       * error, say). `transcribe` returns the text, or `{ status, body }`.
       * Handlers left out keep an earlier fakeAi's; `restore()` puts them back.
       */
      fakeAi(handlers: {
        respond?(call: { url: string; body: Record<string, any>; timeout: number }): Record<string, any> | { status: number; body: string };
        transcribe?(call: {
          url: string; model: string; filename: string; contentType: string; base64: string;
          language: string; prompt: string; timeout: number;
        }): string | { status: number; body: string };
      }): { respondCalls: any[]; transcribeCalls: any[]; restore(): void };
    };
  }
}
export interface Expect<T> {
  toBe(v: T): void;
  toEqual(v: any): void;
  toBeTruthy(): void;
  toBeFalsy(): void;
  toBeNull(): void;
  toBeUndefined(): void;
  toBeDefined(): void;
  toContain(v: any): void;
  toBeGreaterThan(n: number): void;
  toBeGreaterThanOrEqual(n: number): void;
  toBeLessThan(n: number): void;
  toBeLessThanOrEqual(n: number): void;
  toBeCloseTo(n: number, digits?: number): void;
  toHaveLength(n: number): void;
  toMatch(re: RegExp | string): void;
  toThrow(match?: RegExp | string): void;
  not: Expect<T>;
}

declare global {
  function describe(name: string, fn: () => void): void;
  function it(name: string, fn: () => void): void;
  function test(name: string, fn: () => void): void;
  function beforeEach(fn: () => void): void;
  function afterEach(fn: () => void): void;
  function beforeAll(fn: () => void): void;
  function expect<T>(v: T): Expect<T>;
}

export {};
