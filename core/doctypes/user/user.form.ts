import { defineForm, ddcore } from "@ddcore/desk-sdk";

// Saving a User only writes the row: nobody is told. The invitation — an
// account with no password and a link to set one — goes out from these
// buttons, which call the same core.services.users methods as
// `ddcore user invite`.
//
// When the site is not really delivering mail (the log transport, the
// default), the method hands the link back and the dialog shows it, so the
// operator can pass it on by hand.
defineForm("User", {
  async refresh(frm) {
    if (frm.isNew) {
      frm.addButton(__("Save and invite"), () => invite(frm));
      return;
    }
    if (!frm.doc.enabled || frm.doc.id === "Admin") return;
    let hasPassword = true;
    try {
      ({ hasPassword } = await ddcore.call("core.services.users.accountStatus", { user: frm.doc.id }));
    } catch (e) {
      ddcore.ui.showError(e);
      return;
    }
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
});

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
