/**
 * Delivery, as a job target.
 *
 * This is not whitelisted: nobody calls it over HTTP. `Engine.SendMail`
 * enqueues it, which is what buys durable retries from `ddcore_job` without
 * this package growing a queue of its own — and, because `enqueue` writes on
 * the request's transaction, a message is only queued if the request commits.
 *
 * The pluggable transport is resolved here rather than in Go. When the site
 * configures `DDCORE_MAIL_TRANSPORT=method`, `DDCORE_MAIL_METHOD` names an app
 * function by dotted path — exactly how a `scheduler` entry and
 * `ddcore.enqueue` already name app code — and `ddcore.callMethod` reaches it.
 * Routing it here means the hook needs no new mechanism at all: `ddcore.app.ts`
 * has no generic hook to hang one on, and inventing a manifest key for one
 * feature would have been the wrong kind of new.
 */
export function send(args: { to: string[]; subject: string; text: string; html?: string }) {
  const method = (ddcore as any).__mailMethod();
  if (method) {
    ddcore.callMethod(method, args);
    return;
  }
  (ddcore as any).__sendMail(args);
}
