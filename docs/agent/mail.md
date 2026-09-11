# Sending mail

One call queues one message from a declared template, records what became of
it, and hands it to a worker. There is no outbox to poll and no send function to
call: the framework's own recovery and invitation messages travel this same
road, which is what keeps it honest.

## A template

A message lives in `mail/<name>.mail.ts`, next to `doctypes/` and `services/`.
Nothing registers it — the file is enough.

```ts
import { defineMailTemplate, _ } from "@ddcore/sdk";

export default defineMailTemplate<{ order: string; customer: string; url: string }>({
  name: "shop.order_confirmed",
  subject: (d) => _("Your order {0} is confirmed", [d.order]),
  body: (d, b) => [
    b.p(_("Hi {0},", [d.customer])),
    b.p(_("We received your order.")),
    b.table([_("Item"), _("Qty")], d.lines.map((l) => [l.item, l.qty])),
    b.button(_("View order"), d.url),
  ],
});
```

`name` is unique across every installed app, like a DocType name, so prefix it
with the app's own. Declaring the same name twice is a load error naming both
files.

**Both functions run at delivery, in the reader's language**, so every string in
them has to be a literal `_("…")` call. The extractor collects `_(…)` by syntax:
a subject assembled through a helper is reported as `dynamic`, `make check` sees
nothing missing, and the message goes out in English on a translated site. This
is the same trap a Select's values have, for the same reason — see
[i18n](i18n.md).

## Blocks

An app never writes HTML. `body` returns blocks, and the core renders both the
plain-text and the HTML part from the same list, escaping as it goes:

| Block | Text part | HTML part |
| --- | --- | --- |
| `b.p(text)` | a paragraph | `<p>` |
| `b.h(text)` | underlined with `=` | `<h2>` |
| `b.button(text, url)` | `text: url` | a styled link |
| `b.table(head, rows)` | columns aligned by rune | `<table>` |
| `b.rule()` | a line of dashes | `<hr>` |

The plain-text part always spells a button's address out, which is also what
lets `ddcore user invite` work with no transport configured.

A `button` whose URL carries a scheme other than `http`, `https` or `mailto`
renders as text rather than a link. A block type the core does not know renders
as nothing — a newer app against an older core loses that block instead of
leaking a struct into somebody's inbox.

## Sending

```ts
const { delivery } = ddcore.sendMail({
  template: "shop.order_confirmed",
  to: customer.email,
  args: { order: doc.name, customer: doc.customer_name, url: link, lines },
  attach: [doc.invoice],                          // File names, or a file_url
  reference: { doctype: "Sales Order", name: doc.name },
  key: `order-confirmed:${doc.name}`,             // optional
});
```

Synchronous, like everything else on the server, but delivery is not. The record
and the job are both written on **your** transaction:

- A request that rolls back has sent nothing and left nothing behind.
- A template nobody declared, an address that is not an address, and an
  attachment you may not read all fail **here**, in your own transaction, rather
  than alone in a worker half an hour later.

`lang` overrides the language; by default it is the recipient's, if they are a
user of this site, and the site's otherwise. The language of whoever pressed the
button never decides what a reader receives.

`key` makes a send idempotent: the column is unique, so a second call with the
same key is refused by the database rather than delivered twice. Without a key,
two identical messages are two messages — resending an invitation is a thing
people do on purpose.

## Attachments

`attach` takes `File` document names, or the `file_url` an `Attach` field
stores. Permission is decided once, when the message is queued, in your context:
read permission on the document a file hangs from is read permission on the
file, and a detached file belongs to its owner. It is the same rule
`/private/files` applies, because it is the same function.

`DDCORE_MAIL_MAX_ATTACHMENT` caps the total bytes on one message (10 MiB by
default), summed from `File.file_size` before anything is queued.

## The record

Every message becomes an `Email Delivery`, at `/app/email-delivery` for a System
Manager. It holds the recipient, the subject, the template and its arguments,
the language, the attachment names, the reference, and the outcome:

| Status | Meaning |
| --- | --- |
| `Queued` | written, not yet attempted |
| `Sent` | the transport accepted it |
| `Failed` | it did not go; the job retries up to three times |
| `Uncertain` | the connection failed *after* the message was handed over |

`Uncertain` is not retried. The relay may already have it, and trying again is
precisely how the same message arrives twice. SMTP does not promise
exactly-once delivery, and this is the framework declining to pretend otherwise.

**The rendered body is never stored.** A message is re-rendered from its
template and arguments whenever it is needed, which keeps the table small and
keeps the body's contents out of a table administrators can read.

Deleting the document a message refers to does **not** delete the record: an
order can be deleted, an invoice that already reached a customer cannot be
un-sent. Renaming the document does follow.

## Messages that carry a credential

A template whose arguments are a secret — a recovery link, a one-time token —
must say so:

```ts
export default defineMailTemplate<{ link: string }>({
  name: "core.reset",
  sensitive: true,
  subject: () => _("Reset your password on {0}", [site()]),
  body: (d, b) => [b.button(_("Choose a new password"), d.link)],
});
```

A sensitive message keeps only its metadata: who it went to, its subject,
whether it arrived. Its arguments travel in the job payload and nowhere else,
and it cannot be re-rendered after the fact.

**Leaving `sensitive` off a template that mails a credential writes that
credential into `tab_email_delivery`, where every System Manager can read it.**
Nothing detects this for you. The framework's own two messages —
`core/mail/invite.mail.ts` and `core/mail/reset.mail.ts` — are declared this
way, and are the example to copy.

Note what this does *not* fix: the job payload is a column too. A live token
sits in `ddcore_job` until the retention sweep removes the finished row — seven
days by default, thirty if the send failed (`jobRetentionDays` and
`jobRetentionFailedDays` in `ops`; see [operations](ops.md)). A recovery link
expires long before that, so the window is bounded by the token's own lifetime
rather than by the sweep. It is much less exposure than storing the body, and it
is not zero.

## Where a message goes

Transport is deployment, not site configuration, so all of it is environment —
see [authentication](auth.md) and `.env.example`:

- `log` (the default) writes the message, link and all, to the log, and hands
  the link back to whoever asked. This is what makes development and
  `ddcore user invite` work with nothing configured.
- `smtp` talks to a relay. It refuses to send credentials in the clear to
  anything but loopback.
- `method` hands the composed message to an app function named by
  `DDCORE_MAIL_METHOD` — an HTTP mail API, usually. It receives
  `{ to, subject, text, html, attachments }`, with each attachment's bytes
  base64-encoded, and is expected to throw if it could not send.

`ddcore doctor` reports which transport is live.

## What this is not

There is no inbound mail, no IMAP, no bounce handling, no CC or BCC, no
Reply-To, no per-message From, and no resend button. An app that generates a PDF
writes a `File` first and attaches it by name; there is no way to attach raw
bytes, because bytes with no owner have no retention policy and no permission.
