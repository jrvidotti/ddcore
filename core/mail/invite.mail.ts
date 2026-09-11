import { defineMailTemplate, _ } from "@ddcore/sdk";

/**
 * The message an invited person receives.
 *
 * `sensitive` because `link` carries a single-use token: anyone holding it can
 * set the password on that account. The delivery record keeps who it went to
 * and whether it arrived, and not one character of the link.
 *
 * The site's title is a key like any other label, and it is going inside a
 * translated sentence — the reader gets "Projetos", not "Projects", in a
 * message that already speaks their language. The extractor reports it as
 * `dynamic`, the way it reports a Select's value.
 */
const site = () => _(ddcore.siteName());

export default defineMailTemplate<{ link: string }>({
  name: "core.invite",
  sensitive: true,
  subject: () => _("You have been invited to {0}", [site()]),
  body: (d, b) => [
    b.p(_("An account was created for you on {0}. Choose a password to get in.", [site()])),
    b.button(_("Set my password"), d.link),
    b.p(_("This link can only be used once.")),
  ],
});
