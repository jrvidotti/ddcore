/**
 * Housekeeping for ddcore_job, as a scheduler target.
 *
 * It is hygiene and never correctness. The scheduler only runs when the site
 * sets `"scheduler": true`, so nothing may depend on this having run: a job is
 * finished whether or not its row is still here, and every read path works the
 * same with the row gone. If this never ran, the table would only grow.
 *
 * How long a row survives is the site's decision, in `ops.jobRetentionDays` and
 * `ops.jobRetentionFailedDays`. Zero there means keep forever. Failures outlive
 * successes because they are the evidence of what went wrong, and they are read
 * long after the fact.
 */
export function sweep() {
  const n = (ddcore as any).__jobSweep();
  if (n.done || n.failed) {
    ddcore.log.info(`job sweep: ${n.done} done, ${n.failed} failed/cancelled`);
  }
  return n;
}
