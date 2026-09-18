# Design: outgoing webhooks (OPS-06)

Record of the design built on 2026-09-13. The contract for app authors is
[`docs/agent/webhooks.md`](../../agent/webhooks.md); this document keeps **why**
each piece is the way it is.

## Starting point

An app had `ddcore.http`, synchronous, and `ddcore.enqueue`. Every integration
rebuilt the same things badly: no signature, no event identity, no record of
what was sent, and a retry that was either missing or blind. The roadmap asked
for delivery after commit, signatures, stable identity, timeout, retries and
authorized replay, tested against rollback, receiver failure and repetition.

OPS-02 had already paid for the shape: a record and a job written on the
caller's transaction, outcomes written on the pool.

## Decisions

**1. A subscription is a DocType, not an app file.** URLs differ between staging
and production and the key is a secret; neither can be versioned with code.
Mail templates are files because their *text* is code (it goes through `_()`);
a webhook has no text. Chosen with the user over a `*.webhook.ts` file.

**2. Standard Webhooks, not a home-grown header.** `webhook-id`,
`webhook-timestamp`, `webhook-signature: v1,<base64>` over `id.timestamp.body`.
Receivers get verification libraries in every language for free.
`TestOPS06_SignatureMatchesTheStandardWebhooksVector` pins the spec's published
vector, so a refactor cannot silently break every receiver.

**3. The key is a Vault field.** Encrypted at rest, redacted on read, kept out of
Version and export by the existing Vault machinery. Delivery reads it through
`vaultRead`, which skips `Vault Audit Log`: a row per retry would bury the reads a
person made under the ones a worker made.

**4. Enqueue in Go, at the three lifecycle exits.** `Insert`, `Save` and `Delete`
call `queueDocWebhooks` after their hooks, with the reloaded document. A hook that
throws takes the event down with it. `dbSet` and rename bypass the lifecycle on
purpose and emit nothing.

**5. Subscriptions are cached in memory.** The lookup sits on every write and the
table is usually empty. The cache is read on the pool — committed rows only — and
dropped in an `AfterCommit` of any `Webhook` write, so a concurrent writer cannot
re-read stale rows into it after the change. Single process per tenant makes this
sufficient; multiple replicas (PRD-08) would need invalidation over events.

**6. The body is frozen at the event.** `payload` stores the envelope, so a retry
an hour later describes the document as it was. It is a `JSON` column, so the
bytes sent are Postgres's canonical rendering of it — identical on every attempt,
which is what the signature needs, though not byte-identical to what Go first
marshalled. The document is a JSON copy passed through `RedactDoc`, the same
function the API border uses, so a Password or Vault value cannot reach a
receiver by a path the API already closed.

**7. The delivery name is the event id.** One record per (event, subscription),
retries and replay reuse it. That is the "stable event identity" the roadmap
asks for, and the thing a receiver deduplicates on.

**8. A timeout is retried — unlike mail's `Uncertain`.** Mail cannot deduplicate:
the same message twice lands in an inbox twice. A webhook carries its id, and the
Standard Webhooks contract asks receivers to use it. Retrying an ambiguous outcome
is therefore the right default here and the wrong one there, and both documents
say so. 4xx other than 408/429 and any redirect are final; redirects are not
followed because a signed body re-posted elsewhere goes to an unconfigured
address.

**9. Backoff belongs to the job queue.** A fixed 30s retry would exhaust six
attempts in three minutes against a receiver that is down for twenty. Rather than
a webhook-private scheduler, `ddcore_job` gained a `backoff` column
(`fixed`|`exponential`, 30s doubling, capped at an hour), shared by the failure
branch and the stale-lease sweep through one SQL expression, and copied by
`RetryJob`. Any app job can use it.

**10. Replay is audited, including refusals.** A replay sends data to a third
party on a person's say-so. The roadmap's Stage 1 asks for "the minimum protected
audit-event facility needed by the first service": `Audit Event` is a DocType
readable only by System Manager, with no `create`/`write` for anyone. An allowed
replay writes on the caller's transaction (a rolled-back replay did not happen);
a refused one writes on the pool, because the refusal is what an investigation
looks for. `Vault Audit Log` is left alone: folding it in is PRD-06's job.

**11. A replay of an in-flight delivery is refused.** `Queued` or `Retrying` means
a job still owns it; a second job would double the send. The row is locked
`FOR UPDATE` so two admins pressing the button race on the lock, not on
the status.

**12. `DDCORE_WEBHOOKS=off` queues nothing.** For rehearsals and restored copies.
Pausing delivery instead would accumulate a backlog of stale events that fires on
the day someone turns it back on. Environment, not `ddcore.json`: it is the
deployment that knows it is a rehearsal.

**13. Retention.** A payload is a copy of a document; keeping every one forever
keeps data that an erasure elsewhere meant to remove. `ops.webhookRetentionDays`
(default 30) with a daily sweep of finished deliveries only.

## Left out

Per-webhook conditions and field selection, custom headers, dual-key secret
rotation, private-network (SSRF) restrictions, inbound webhooks, and a delivery
dashboard. `apps/testapp` gains nothing: the end-to-end coverage lives in
`internal/engine/webhooks_test.go`, where the worker and an `httptest` receiver
are reachable.
