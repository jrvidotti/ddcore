import { whitelisted } from "@ddcore/sdk";

/**
 * Outgoing webhooks: the job target, the replay action and the sweep.
 *
 * Everything that matters happens in Go (internal/engine/webhooks.go): signing,
 * the HTTP request, deciding whether a failure is worth retrying, and writing
 * the outcome where a rollback cannot erase it. This file only gives those a
 * dotted path the job worker, the desk and the scheduler can name.
 */

/**
 * One delivery attempt, as a job target. Not whitelisted: nobody calls it over
 * HTTP. Throwing asks the job to run again with its exponential backoff; the
 * delivery record already says why.
 */
export function send(args: { delivery: string }) {
  (ddcore as any).__webhooks.deliver(args.delivery);
}

/**
 * Sends a finished delivery again, with the same `webhook-id` and body, so a
 * receiver that already has it can tell. Leaves an `Audit Event` either way.
 */
export const replay = whitelisted((args: { delivery: string }) => {
  return (ddcore as any).__webhooks.replay(args.delivery);
}, { roles: ["System Manager"] });

/**
 * Removes finished deliveries older than `ops.webhookRetentionDays`, as a
 * scheduler target. Hygiene, never correctness.
 */
export function sweep() {
  const n = (ddcore as any).__webhooks.sweep();
  if (n.deliveries) ddcore.log.info(`webhook sweep: ${n.deliveries} deliveries`);
  return n;
}
