/**
 * The parts of the profile page that are decisions rather than markup.
 *
 * They live here for the reason ListView's state does: a component cannot be
 * unit-tested in this repo — there is no testing-library — and these are
 * exactly the bits worth testing.
 */

export interface SessionRow {
  id: string;
  current: boolean;
  ip?: string | null;
  userAgent?: string | null;
  created?: string | null;
  lastSeen?: string | null;
  expires?: string | null;
}

/**
 * Client-side password checks.
 *
 * These are a courtesy, not a control: the server applies the real policy on
 * every path that sets a password, and this only saves a round trip and says
 * the obvious thing about the confirmation field. `minLength` comes from the
 * server so the two never disagree about the number.
 */
export function passwordProblem(
  password: string,
  confirm: string,
  minLength: number,
  t: (s: string, args?: any[]) => string,
): string {
  if (!password) return t("Choose a password");
  if (password.length < minLength) return t("The password must have at least {0} characters", [minLength]);
  if (confirm !== password) return t("The two passwords do not match");
  return "";
}

/**
 * A short, honest description of a device from its user agent.
 *
 * Deliberately crude. The point is to let someone recognise their own
 * sessions, and a browser and platform do that; anything more precise is
 * fingerprinting written into our own UI.
 */
export function describeDevice(ua: string | null | undefined, unknown: string): string {
  if (!ua) return unknown;
  const browser =
    /Edg\//.test(ua) ? "Edge" :
    /OPR\/|Opera/.test(ua) ? "Opera" :
    /Firefox\//.test(ua) ? "Firefox" :
    /Chrome\//.test(ua) ? "Chrome" :
    /Safari\//.test(ua) ? "Safari" :
    "";
  const os =
    /iPhone|iPad/.test(ua) ? "iOS" :
    /Android/.test(ua) ? "Android" :
    /Mac OS X|Macintosh/.test(ua) ? "macOS" :
    /Windows/.test(ua) ? "Windows" :
    /Linux/.test(ua) ? "Linux" :
    "";
  const parts = [browser, os].filter(Boolean);
  return parts.length ? parts.join(" · ") : unknown;
}

/**
 * The current session first, then the most recently seen.
 *
 * Putting "this device" at the top is what makes the list safe to act on: the
 * one row a person must not revoke by accident is the one they can always
 * find.
 */
export function sortSessions(rows: SessionRow[]): SessionRow[] {
  return [...rows].sort((a, b) => {
    if (a.current !== b.current) return a.current ? -1 : 1;
    return String(b.lastSeen ?? "").localeCompare(String(a.lastSeen ?? ""));
  });
}

/** True when a key is past its expiry — the list greys those out. */
export function isExpired(expires: string | null | undefined, now = new Date()): boolean {
  if (!expires) return false;
  const t = new Date(expires).getTime();
  return Number.isFinite(t) && t < now.getTime();
}
