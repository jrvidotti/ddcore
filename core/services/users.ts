import { whitelisted, _ } from "@ddcore/sdk";

/**
 * What a System Manager does *to* other accounts: invite one, send someone a
 * recovery link, end their sessions, lift a lockout.
 *
 * These are the counterpart of `profile.ts`, and the difference in tone is
 * deliberate. A public recovery endpoint must never say whether an address
 * exists; an admin pressing "send invitation" is entitled to be told
 * plainly when it does not, because they are already allowed to read the list
 * of users.
 *
 * The role is enforced by the whitelist, on the Go side, before any of this
 * runs.
 */

const ADMIN = { roles: ["System Manager"] };

/**
 * Creates an account with **no password** and mails an invitation.
 *
 * No password is not a special state needing a flag: `CheckPassword` already
 * refuses an empty hash, so an invited person simply cannot sign in until they
 * set one.
 *
 * `link` comes back only when the site is not really delivering mail — in
 * development, or before SMTP is configured — so that an operator can pass it
 * on by hand instead of reading a log file.
 */
export const invite = whitelisted((args: {
  email: string; fullName: string; roles?: string[]; userType?: string;
}) => {
  // ddcore.users.invite validates, creates, mails and audits; the same call
  // an app makes to invite people to its portal
  return ddcore.users.invite({
    email: String(args.email ?? ""),
    fullName: String(args.fullName ?? ""),
    roles: args.roles || [],
    userType: (args.userType as any) || "System User",
  });
}, ADMIN);

export const resendInvite = whitelisted((args: { user: string }) => {
  return ddcore.users.resendInvite(requireUser(args.user));
}, ADMIN);

export const sendPasswordReset = whitelisted((args: { user: string }) => {
  const user = requireUser(args.user);
  const rec = (ddcore as any).__auth.startRecovery(user, "reset");
  ddcore.audit("account.reset_password", "User", user);
  return { user, expires: rec.expires, link: rec.link || undefined };
}, ADMIN);

/** Ends every session of another account — the "they lost the laptop" button. */
export const revokeUserSessions = whitelisted((args: { user: string }) => {
  const user = requireUser(args.user);
  ddcore.audit("account.revoke_sessions", "User", user);
  return (ddcore as any).__auth.revokeSessions(user, {});
}, ADMIN);

/**
 * Lifts a lockout without waiting it out.
 *
 * The counter is keyed on what was typed rather than on the account it
 * resolved to — that is what stops the lockout from answering "does this
 * address exist" — so unlocking clears the key for the name and for the
 * e-mail, which are the two things anyone would have typed.
 */
export const unlockUser = whitelisted((args: { user: string }) => {
  const user = requireUser(args.user);
  const auth = (ddcore as any).__auth;
  const email = ddcore.db.getValue("User", user, "email") as string | null;
  let cleared = auth.clearAttempts("login:" + String(user).toLowerCase());
  if (email && email.toLowerCase() !== String(user).toLowerCase()) {
    cleared += auth.clearAttempts("login:" + email.toLowerCase());
  }
  ddcore.audit("account.unlock", "User", user, { cleared });
  return { user, cleared };
}, ADMIN);

function requireUser(name: string): string {
  const user = String(name ?? "").trim();
  if (!user || !ddcore.db.exists("User", user)) ddcore.throw(_("User {0} does not exist", [user]));
  return user;
}
