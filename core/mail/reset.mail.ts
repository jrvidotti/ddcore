import { defineMailTemplate, _ } from "@ddcore/sdk";

/**
 * The message someone gets after asking to recover their password.
 *
 * `sensitive` for the same reason as the invitation: `link` is a live
 * credential until it is spent. See `core/mail/invite.mail.ts`.
 */
const site = () => _(ddcore.siteName());

export default defineMailTemplate<{ link: string }>({
  name: "core.reset",
  sensitive: true,
  subject: () => _("Reset your password on {0}", [site()]),
  body: (d, b) => [
    b.p(_("Someone asked to reset the password for this account on {0}. If it was not you, ignore this message: nothing has changed.", [site()])),
    b.button(_("Choose a new password"), d.link),
    b.p(_("This link can only be used once.")),
  ],
});
