import { defineForm, ddcore } from "@ddcore/desk-sdk";

// Saving a User only writes the row: nobody is told. The invitation — an
// account with no password and a link to set one — goes out from these
// buttons, which call the same core.services.users methods as
// `ddcore user invite`.
//
// When the site is not really delivering mail (the log transport, the
// default), the method hands the link back and the dialog shows it, so the
// operator can pass it on by hand.
//
// On a site that provisions an identity provider (PocketID), disabling,
// enabling or deleting a User asks whether to do the same there: the account
// at the provider may be used for other applications too, so it is the
// operator's call and never automatic.
type Seen = { provider?: { id: string; label: string }; enabled?: boolean };
const seen = new WeakMap<object, Seen>();

defineForm("User", {
  async refresh(frm) {
    if (frm.isNew) {
      frm.addButton(__("Save and invite"), () => invite(frm));
      return;
    }
    const state: Seen = { enabled: Boolean(frm.doc.enabled) };
    seen.set(frm, state);
    if (frm.doc.id === "Admin") return;
    let hasPassword = true;
    try {
      let provider;
      ({ hasPassword, provider } = await ddcore.call("core.services.users.accountStatus", { user: frm.doc.id }));
      state.provider = provider;
    } catch (e) {
      ddcore.ui.showError(e);
      return;
    }
    if (!frm.doc.enabled) return;
    if (hasPassword) {
      frm.addButton(__("Send password reset"), () =>
        send(frm, "core.services.users.sendPasswordReset", __("Send {0} a link to choose a new password?", [frm.doc.email]), __("Password reset sent")),
      );
    } else {
      frm.addButton(__("Resend invitation"), () =>
        send(frm, "core.services.users.resendInvite", __("Send {0} a new invitation?", [frm.doc.email]), __("Invitation sent")),
      );
    }
  },
  async afterSave(frm) {
    const state = seen.get(frm);
    const enabled = Boolean(frm.doc.enabled);
    if (!state?.provider || state.enabled === undefined || state.enabled === enabled) return;
    state.enabled = enabled;
    const label = state.provider.label;
    const question = enabled
      ? __("{0} was enabled here. Enable their account in {1} too?", [frm.doc.email, label])
      : __("{0} was disabled here. Disable their account in {1} too? They will not be able to sign in there to anything else either.", [frm.doc.email, label]);
    await followAtProvider(question, label, { user: frm.doc.id, email: frm.doc.email, disabled: !enabled }, !enabled);
  },
  async afterDelete(frm) {
    const state = seen.get(frm);
    if (!state?.provider) return;
    const label = state.provider.label;
    await followAtProvider(
      __("{0} was deleted here. Disable their account in {1} too? They will not be able to sign in there to anything else either.", [frm.doc.email, label]),
      label, { email: frm.doc.email, disabled: true }, true,
    );
  },
});

async function followAtProvider(question: string, label: string, args: { user?: string; email: string; disabled: boolean }, destructive: boolean) {
  if (!(await ddcore.ui.confirm(question, label, { destructive }))) return;
  try {
    const res = await ddcore.call("core.services.users.setProviderDisabled", args);
    if (!res.found) {
      ddcore.ui.toast(__("{0} has no account in {1}", [args.email, label]), { indicator: "orange" });
    } else {
      ddcore.ui.toast(args.disabled ? __("Disabled in {0}", [label]) : __("Enabled in {0}", [label]));
    }
  } catch (e) {
    ddcore.ui.showError(e);
  }
}

async function invite(frm: any) {
  const d = frm.doc;
  if (!d.email || !d.full_name) {
    ddcore.ui.toast(__("Fill in the email and the full name first"), { indicator: "orange" });
    return;
  }
  if (d.new_password) {
    ddcore.ui.toast(__("An invited person chooses their own password: clear the New password field"), { indicator: "orange" });
    return;
  }
  try {
    const res = await ddcore.call("core.services.users.invite", {
      email: d.email,
      fullName: d.full_name,
      roles: (d.roles || []).map((r: any) => r.role).filter(Boolean),
      userType: d.user_type || "System User",
    });
    // The draft of the new form would otherwise ask to be kept on the way out.
    await frm.discardChanges();
    await ddcore.setRoute("User", res.user);
    done(res, __("Invitation sent"));
  } catch (e) {
    ddcore.ui.showError(e);
  }
}

async function send(frm: any, method: string, question: string, sent: string) {
  if (!(await ddcore.ui.confirm(question))) return;
  try {
    done(await ddcore.call(method, { user: frm.doc.id }), sent);
  } catch (e) {
    ddcore.ui.showError(e);
  }
}

function done(res: { link?: string }, sent: string) {
  if (!res.link) {
    ddcore.ui.toast(sent);
    return;
  }
  // message is rendered as HTML, and the only button is the secondary one
  ddcore.ui.Dialog({
    title: __("Mail is not being delivered"),
    message: esc(__("This site sends no mail yet, so the message only went to the log. Pass this link on by hand.")),
    fields: [{ fieldname: "link", fieldtype: "Data", label: __("Link"), readOnly: true }],
    values: { link: res.link },
    secondaryLabel: __("Close"),
  }).show();
}

function esc(s: string) {
  return s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]!);
}
