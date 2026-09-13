# Outgoing webhooks

A webhook tells somebody else's server that something happened here. The
framework records every event it owes a receiver, sends it after the change
commits, signs it, retries it with backoff, and lets an administrator send it
again. An app does not write an HTTP client or a retry loop for this.

## A subscription

A `Webhook` is a document, at `/app/Webhook` for a System Manager. It is data
and not a file in an app, because every part of it belongs to the deployment:
staging posts to a different receiver than production, and the signing key is a
secret.

| Field | Meaning |
| --- | --- |
| `enabled` | a disabled webhook queues nothing, and a delivery already queued for it fails instead of sending |
| `url` | where the POST goes. `https` is required outside development mode, except for a loopback address |
| `event_type` | `Document` — lifecycle events of one DocType — or `Custom` — an event an app emits |
| `webhook_doctype` + `on_insert`, `on_update`, `on_submit`, `on_cancel`, `on_trash` | which document events |
| `custom_event` | the name an app passes to `ddcore.webhooks.emit` |
| `secret` | the signing key, a `Vault` field: encrypted at rest, never returned by a read. Needs `DDCORE_SECRET_KEY` (see [vault](vault.md)) |
| `timeout` | seconds to wait for an answer, 1–60, default 10 |
| `max_attempts` | how many times to try, 1–10, default 6 |

The framework's own bookkeeping DocTypes cannot be watched — `Webhook`,
`Webhook Delivery`, `Audit Event`, `Version`, `Error Log`, `Email Delivery`,
`Vault Audit Log` — and neither can a child table: a change to a row is an
update of its parent.

Changes to subscriptions take effect for writes that start after the change
commits.

## Document events

| Event type the receiver sees | When |
| --- | --- |
| `doc.on_insert` | a document is inserted (an insert that submits also sends `doc.on_submit`) |
| `doc.on_update` | a draft is saved, or an `allowOnSubmit` field of a submitted document changes |
| `doc.on_submit` | submitted |
| `doc.on_cancel` | cancelled |
| `doc.on_trash` | deleted |

The event is queued after the controller's hooks have run, so a hook that throws
takes the event down with the write. `doc.dbSet` and rename are not events: they
deliberately skip the lifecycle, and a webhook is part of it.

## App events

```ts
ddcore.webhooks.emit("shop.order_paid", { order: doc.name, total: doc.grand_total }, {
  reference: { doctype: "Sales Order", name: doc.name },
  key: `order-paid:${doc.name}`,       // optional
});
// → { deliveries: ["k3j9…"] }
```

The name is a dotted lowercase identifier; `doc.` is reserved for the framework's
own events, so an app cannot forge one. `data` is copied into the payload as it
is at the call. With no subscription for the event, `emit` does nothing and
returns an empty list.

`key` makes an emit idempotent per webhook: it is stored in a unique column, so a
second emit with the same key throws instead of sending twice. Without a key,
two emits are two events.

## What is sent

```
POST <url>
content-type: application/json
webhook-id: k3j9x0f2ab
webhook-timestamp: 1789336574
webhook-signature: v1,AphAXzEKQFNKZZ8tAMa2/zzUw+Fx/F6gUtZcgIGmdaE=

{"data": {"doc": {…}, "name": "SO-0001", "doctype": "Sales Order"}, "type": "doc.on_submit", "timestamp": "2026-09-13T21:56:14.056335Z"}
```

The headers follow [Standard Webhooks](https://www.standardwebhooks.com/), so a
receiver can verify with any of its libraries:

- `webhook-id` is the `Webhook Delivery` name. It is **the same on every attempt
  and on a replay** — that is what a receiver deduplicates on.
- `webhook-signature` is `v1,` + base64 HMAC-SHA256 of `<id>.<timestamp>.<body>`.
  A secret written `whsec_<base64>` is decoded as that spec's key format;
  anything else is used as its raw bytes.
- `webhook-timestamp` is the attempt's time, so a receiver can refuse stale
  requests.

For a document event, `data.doc` is the document with its children, as it was
when the event happened — not as it is when a retry goes out an hour later.
Every `Password` and `Vault` field is removed, exactly as an API read removes it.

## Delivery

Nothing is sent from the request. The delivery record and its job are written on
**your** transaction:

- A request that rolls back has told no receiver anything, and left no record.
- A worker that dies mid-delivery leaves a job whose lease expires; the next
  worker picks it up.

| Status | Meaning |
| --- | --- |
| `Queued` | written, not yet attempted |
| `Retrying` | an attempt failed in a way worth retrying; another is scheduled |
| `Sent` | the receiver answered 2xx |
| `Failed` | out of attempts, refused by the receiver, or the webhook was disabled |

A network error, a timeout, `408`, `429` and any `5xx` are retried, waiting 30
seconds, then 1, 2, 4, 8 minutes, capped at an hour — the job queue's
`backoff: "exponential"` (see [operations](ops.md)). Any other answer, including
a redirect, is final: redirects are not followed, because a signed body re-sent
to wherever a `302` points is sent to an address nobody configured. The record
keeps the response status and the first 512 bytes of the answer.

**A timeout is retried even though the receiver may already have acted.** This
is the opposite of mail's `Uncertain`, and deliberately so: every attempt carries
the same `webhook-id`, and a receiver that stores the ids it has processed turns
"at least once" into "effectively once". A receiver that does not deduplicate
will, sometimes, act twice. Nothing here is exactly-once.

Deleting the document an event refers to does not delete the delivery; renaming
it does follow.

## Replay

A `Sent` or `Failed` delivery can be sent again — the **Replay** button on the
delivery, `ddcore webhooks replay <delivery>`, or
`POST /api/method/core.services.webhooks.replay` with `{ "delivery": "…" }`.
A replay keeps the id and the body and starts a fresh set of attempts.

Only a System Manager may replay, and a delivery still `Queued` or `Retrying` is
refused rather than doubled. **Every replay writes an `Audit Event`** —
`webhook.replay`, the actor, the delivery, the request id and the previous
status — and so does a refused one, even though its transaction rolled back.
The detail never carries the payload or the secret.

## Operations

- `DDCORE_WEBHOOKS=off` stops events from becoming deliveries — for a migration
  rehearsal or a restored copy of production that must not reach real receivers.
  Nothing is queued while it is off, so turning it back on releases no backlog.
  `ddcore doctor` warns while it is off with webhooks enabled.
- `ddcore webhooks list [--status Failed] [--webhook name]` shows recent
  deliveries.
- `ops.webhookRetentionDays` (default 30, zero keeps for ever) is how long a
  `Sent` or `Failed` delivery is kept; `core.services.webhooks.sweep` removes
  older ones daily where the scheduler runs. A payload is a copy of a document,
  so keeping every one for ever keeps data an erasure elsewhere meant to remove.
- Jobs run on the `webhook` queue, so `ddcore jobs stats` separates a receiver
  that is down from everything else.

## What this is not

There is no per-webhook condition or field selection, no custom headers, no
secret rotation with two keys valid at once, and no inbound webhooks. A receiver
URL is not checked against private networks, so a webhook can point at a service
inside the deployment's own network: creating one is a System Manager's power,
and should be treated as such.
