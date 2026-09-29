import { defineMailTemplate, _ } from "@ddcore/sdk";

/**
 * The invitation of a site that provisions its identity provider (PocketID).
 *
 * The account was made at the provider; `link` is its one-time login link,
 * where the person registers a passkey. There is no password to choose here.
 * `sensitive` for the same reason as core.invite: the link is a way into the
 * account for whoever holds it.
 */
const site = () => _(ddcore.siteName());

export default defineMailTemplate<{ link: string; provider: string; loginUrl: string; hours: number }>({
  name: "core.invite_sso",
  sensitive: true,
  subject: () => _("You have been invited to {0}", [site()]),
  body: (d, b) => [
    b.p(_("An account was created for you on {0}. You sign in through {1} with a passkey — your fingerprint, face or device PIN — and no password.", [site(), d.provider])),
    b.p(_("First, set up your passkey on {0}:", [d.provider])),
    b.button(_("Set up my passkey"), d.link),
    b.p(_("This link can only be used once and expires in {0} hours.", [String(d.hours)])),
    b.p(_("Then sign in at {0} with the {1} button.", [d.loginUrl, d.provider])),
  ],
});
