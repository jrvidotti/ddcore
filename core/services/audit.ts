/**
 * Housekeeping for tab_audit_event, as a scheduler target.
 *
 * It is hygiene and never correctness. The scheduler only runs when the site
 * sets `"scheduler": true`. How long an event survives is the site's decision
 * in `ops.auditRetentionDays`. Zero keeps forever.
 */
export function sweep() {
  const n = (ddcore as any).__auditSweep();
  if (n && n.events) {
    ddcore.log.info(`audit sweep: ${n.events} events purged`);
  }
  return n;
}
