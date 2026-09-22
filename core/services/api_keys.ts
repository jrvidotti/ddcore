import { whitelisted, _ } from "@ddcore/sdk";

/**
 * A person's own API keys.
 *
 * `API Key` is a DocType, but its permissions are System Manager only — and
 * they should stay that way, because read on the doctype would be read on
 * everyone's keys. So these go through `ddcore.db.*`, which skips the
 * permission check, and filter by `ddcore.session.user` here instead.
 */

function me(): string {
  const user = ddcore.session.user;
  if (user === "Guest") ddcore.throw(_("Sign in to continue"));
  return user;
}

export const listMyAPIKeys = whitelisted(() => ({
  // Not `ddcore.db.getList`: that one *does* check the doctype's permissions,
  // and API Key is System Manager only. The host op reads the rows of this one
  // user directly, which is the narrower thing.
  keys: (ddcore as any).__auth.apiKeys(me()),
}));

/**
 * Mints a key and returns the secret **once**.
 *
 * Only the hash is stored, so there is no second chance to read it — which is
 * the same property that makes a leaked key replaceable rather than
 * recoverable. The desk says so where it shows it.
 */
export const createMyAPIKey = whitelisted((args: { label?: string; days?: number }) => {
  const user = me();
  const label = String(args.label ?? "").trim();
  if (!label) ddcore.throw(_("Describe what this key is for"));
  const days = Number(args.days ?? 0);
  if (!Number.isFinite(days) || days < 0) ddcore.throw(_("Invalid number of days"));
  return (ddcore as any).__auth.createAPIKey(user, label, Math.floor(days));
});

export const revokeMyAPIKey = whitelisted((args: { id: string }) => {
  const id = String(args.id ?? "").trim();
  if (!id) ddcore.throw(_("That key is not yours"));
  // The owner is part of the delete's WHERE on the host side, so an id taken
  // from someone else's list removes nothing instead of removing theirs.
  return (ddcore as any).__auth.revokeAPIKey(me(), id);
});
