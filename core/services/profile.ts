import { whitelisted, _ } from "@ddcore/sdk";

/**
 * What a person may do to their own account.
 *
 * The shape of every function here is set by one fact, the same one that
 * explains `core/services/i18n.ts`: a user has **no permission at all on their
 * own User record**, not even read. Granting write is not an option, because
 * `roles` is a child table on User and write would let anyone make themselves a
 * System Manager; and a read grant through `ifOwner` grants nothing either,
 * since the owner of the row is whoever created the account.
 *
 * So reads go through `ddcore.db.getValue` and writes through
 * `ddcore.db.setValue`, both of which skip the permission check by design. The
 * price is that they skip the hooks too, so any cache `User.onUpdate` would
 * have dropped has to be dropped here by hand.
 */

function me(): string {
  const user = ddcore.session.user;
  if (user === "Guest") ddcore.throw(_("Sign in to continue"));
  return user;
}

export const getMyProfile = whitelisted(() => {
  const user = me();
  const d = ddcore.db.getValue("User", user, [
    "name", "email", "full_name", "language", "user_type", "last_login",
  ]) as any;
  return {
    name: d?.name ?? user,
    email: d?.email ?? user,
    fullName: d?.full_name ?? "",
    language: d?.language ?? null,
    userType: d?.user_type ?? "",
    lastLogin: d?.last_login ?? null,
    roles: ddcore.getRoles(user).filter((r) => r !== "All"),
  };
});

/**
 * The two fields a person may change about themselves.
 *
 * They are enumerated one by one, and `args` is never spread into `setValue`.
 * That is the whole security of this function: spreading would let a caller
 * post `roles`, `enabled` or `user_type` and have them written with no
 * permission check at all.
 */
export const updateMyProfile = whitelisted((args: { fullName?: string; language?: string | null }) => {
  const user = me();
  const values: Record<string, any> = {};

  if (args.fullName !== undefined) {
    const fullName = String(args.fullName ?? "").trim();
    if (!fullName) ddcore.throw(_("Full name is required"));
    values.full_name = fullName;
  }

  if (args.language !== undefined) {
    // blank is a real choice: it means "follow the site language"
    const language = String(args.language ?? "").trim() || null;
    if (language !== null && !ddcore.session.langs.includes(language)) {
      ddcore.throw(_("{0} is not a language this site serves", [language]));
    }
    values.language = language;
  }

  if (Object.keys(values).length) {
    ddcore.db.setValue("User", user, values);
    // setValue runs no hooks, so User.onUpdate does not drop these for us.
    ddcore.cache.del("lang:" + user);
    ddcore.cache.del("roles:" + user);
  }
  return getMyProfileValues(user);
});

function getMyProfileValues(user: string) {
  const d = ddcore.db.getValue("User", user, ["full_name", "language"]) as any;
  return { fullName: d?.full_name ?? "", language: d?.language ?? null };
}

/**
 * Changing one's own password requires proving the current one.
 *
 * Without that check, anyone who found an unlocked laptop could lock the owner
 * out of their own account — and the session cookie alone is not proof of
 * anything more than possession of the laptop.
 *
 * The attempt is throttled like a login, because a form that verifies a
 * password is a password oracle with a nicer name.
 */
export const changeMyPassword = whitelisted((args: { current: string; password: string }) => {
  const user = me();
  const auth = (ddcore as any).__auth;

  auth.throttle("pwchange:" + user);

  if (!auth.checkPassword(user, String(args.current ?? ""))) {
    ddcore.throw(_("The current password is not right"));
  }

  // Every other session goes; this one stays, so the person is not thrown out
  // of the tab they just typed the password into.
  auth.setPassword(user, String(args.password ?? ""), auth.currentSid());
  auth.clearAttempts("pwchange:" + user);
  return { ok: true };
});
