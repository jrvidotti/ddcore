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
