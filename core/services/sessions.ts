import { whitelisted, _ } from "@ddcore/sdk";

/**
 * A person's own sessions: what is signed in, and how to end it.
 *
 * Sessions live in `ddcore_session`, which is not a DocType and which
 * `ddcore.db.sql` cannot write to — it is read-only by design. So every
 * function here crosses the bridge through a host op, and each one is scoped
 * to `ddcore.session.user` on this side before it does.
 *
 * A session is identified by an opaque handle, never by its sid. The sid is a
 * bearer token: one cross-site scripting hole that could read this list would
 * otherwise hand over every device the person is signed in on.
 */

function me(): string {
  const user = ddcore.session.user;
  if (user === "Guest") ddcore.throw(_("Sign in to continue"));
  return user;
}

export const listMySessions = whitelisted(() => ({
  sessions: (ddcore as any).__auth.sessions(me()),
}));

export const revokeMySession = whitelisted((args: { id: string }) => {
  const id = String(args.id ?? "").trim();
  if (!id) ddcore.throw(_("Which session?"));
  // Scoped to this user on the host side too, so a handle copied from
  // somebody else's list reaches nothing.
  return (ddcore as any).__auth.revokeSessions(me(), { id });
});

/** Ends every session but the one asking — "sign out everywhere else". */
export const revokeMyOtherSessions = whitelisted(() => {
  const auth = (ddcore as any).__auth;
  return auth.revokeSessions(me(), { exceptSid: auth.currentSid() });
});
