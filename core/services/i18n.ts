import { whitelisted, _ } from "@ddcore/sdk";

/**
 * Lets a user change their own language.
 *
 * This exists because permissions are per document, not per field. A user has no
 * write on their own User record, and granting it is not an option: `roles` is a
 * child table on User, so write would also let anyone grant themselves System
 * Manager.
 *
 * Writing through `ddcore.db.setValue` is the deliberate part — it is the one
 * path that touches the column without a permission check and without loading
 * the document (`ddcore.getDoc` would fail first: User is a System Manager
 * doctype, so the user cannot even *read* their own record). The price is that
 * it skips the hooks too, so the cache invalidation `User.onUpdate` normally
 * does has to happen here by hand.
 */
export const setMyLanguage = whitelisted((args: { language?: string | null }) => {
  const user = ddcore.session.user;
  if (user === "Guest") ddcore.throw(_("Sign in to continue"));

  // blank is a real choice: it means "follow the site language"
  const language = String(args.language ?? "").trim() || null;
  if (language !== null && !ddcore.session.langs.includes(language)) {
    ddcore.throw(_("{0} is not a language this site serves", [language]));
  }

  ddcore.db.setValue("User", user, { language });
  // setValue runs no hooks, so User.onUpdate does not drop this for us; without
  // it the change stays invisible for as long as the cached language lives.
  ddcore.cache.del("lang:" + user);

  return { language };
});
