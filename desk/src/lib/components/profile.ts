/**
 * The parts of the profile page that are decisions rather than markup.
 *
 * They live here for the reason ListView's state does: a component cannot be
 * unit-tested in this repo — there is no testing-library — and these are
 * exactly the bits worth testing.
 */

import type { Field } from "$lib/meta";

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

/**
 * The language picker of the profile screen.
 *
 * The site's languages and their autonyms come from the boot payload, not from
 * `User`'s meta: User is a System Manager DocType, so the one person who always
 * needs this field — its subject — cannot read the DocType that declares it.
 * The label and the description are the same English keys the DocType carries,
 * so a translated screen reads the same either way.
 */
export function languageField(langs: { code: string; label: string }[], t: (s: string) => string): Field {
  return {
    fieldname: "language",
    fieldtype: "Select",
    label: t("Language"),
    description: t("Leave blank to follow the site language."),
    // no empty entry here: the control adds one, and blank is a real choice —
    // it means "follow the site language"
    options: langs.map((l) => l.code),
    optionLabels: langs.map((l) => l.label),
  };
}

/** Which of the framework's own notifications also arrive by email. */
export type EmailNotifications = { assignment: boolean; share: boolean; due: boolean };

/**
 * The switches that moved, and only those: the profile service writes what it
 * is sent, so a switch the person did not touch is not sent at all.
 */
export function changedEmailNotifications(
  before: EmailNotifications, after: EmailNotifications,
): Partial<EmailNotifications> {
  const changed: Partial<EmailNotifications> = {};
  for (const kind of ["assignment", "share", "due"] as const) {
    if (after[kind] !== before[kind]) changed[kind] = after[kind];
  }
  return changed;
}

/** What an app's profile section starts its fields with: each field's default, or empty. */
export function sectionValues(fields: Field[] = []): Record<string, any> {
  const values: Record<string, any> = {};
  for (const f of fields) if (f.fieldname) values[f.fieldname] = f.default ?? null;
  return values;
}

/**
 * The label of the first required field left empty, or "" when there is none —
 * the same test a dialog makes before it runs its primary action.
 */
export function missingRequired(fields: Field[] = [], values: Record<string, any>): string {
  for (const f of fields) {
    if (!f.reqd || !f.fieldname || f.hidden) continue;
    const v = values[f.fieldname];
    if (v === null || v === undefined || v === "") return f.label || f.fieldname;
  }
  return "";
}
