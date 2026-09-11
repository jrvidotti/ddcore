import type { SendMailArgs } from "@ddcore/sdk";

/**
 * Delivery, as a job target.
 *
 * This is not whitelisted: nobody calls it over HTTP. `ddcore.sendMail` writes
 * an `Email Delivery` record and enqueues this, which is what buys durable
 * retries from `ddcore_job` without the mail service growing a queue of its
 * own — and, because `enqueue` writes on the request's transaction, a message
 * is only queued if the request commits.
 *
 * Rendering happens here rather than in Go because a template is a JavaScript
 * function: `subject` and `body` run in the reader's language and hand back
 * blocks, which Go turns into the two parts of the message.
 *
 * The pluggable transport is dispatched here for the same reason. When the site
 * sets `DDCORE_MAIL_TRANSPORT=method`, `ddcore.__mail.deliver` composes the
 * message and hands it straight back instead of sending it, because only the
 * runtime can call into an app.
 */
export function send(args: { delivery: string; args?: Record<string, any> }) {
  const bridge = (ddcore as any).__mail;
  const d = bridge.load(args.delivery);

  // A sensitive template's arguments were never written to the delivery
  // record — they travel with the job, and nowhere else.
  const payload = args.args || d.args || {};
  const rendered = (globalThis as any).__ddcore.renderMail(d.template, payload, d.lang);

  const out = bridge.deliver(args.delivery, rendered.subject, rendered.blocks);
  if (!out || !out.method) return;

  try {
    ddcore.callMethod(out.method, out.message);
  } catch (e: any) {
    // Go recorded nothing for this transport, so an app that throws would
    // otherwise leave the record sitting at Queued for ever.
    bridge.result(args.delivery, "Failed", String((e && e.message) || e));
    throw e;
  }
  bridge.result(args.delivery, "Sent");
}

/**
 * Queues a message on behalf of Go.
 *
 * `Engine.StartRecovery` has a template to render, and a template is a
 * JavaScript function — so the framework's own two messages take the same road
 * an app's message takes, rather than a private one in Go. Not whitelisted:
 * reachable by dotted path, never over HTTP.
 */
export function queue(args: SendMailArgs) {
  return ddcore.sendMail(args);
}
