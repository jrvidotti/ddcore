import { defineMailTemplate, _ } from "@ddcore/sdk";

/**
 * The message a person receives when a tenant adds them while they already
 * sign in to the site from another one (#167). There is nothing to set up:
 * their password, or their single sign-on, is the one they have, so the link
 * is just the tenant's address — and it carries no token.
 *
 * It names the tenant that added them, which they now work in, and nothing
 * about the others.
 */
const site = () => _(ddcore.siteName());

export default defineMailTemplate<{ tenantTitle: string; link: string }>({
  name: "core.access_granted",
  subject: (d) => _("You now have access to {0}", [d.tenantTitle || site()]),
  body: (d, b) => [
    b.p(_("Your account on {0} can now work in {1}. Sign in as usual and choose it from the user menu.", [site(), d.tenantTitle || site()])),
    b.button(_("Open {0}", [d.tenantTitle || site()]), d.link),
    b.p(_("If you cannot sign in, choose “I forgot my password” on the sign-in page.")),
  ],
});
