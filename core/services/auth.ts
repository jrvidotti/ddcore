/**
 * Housekeeping for the auth tables, as a scheduler target.
 *
 * It is hygiene and never correctness. The scheduler only runs when the site
 * sets `"scheduler": true`, so nothing may depend on this having run: every
 * read path filters on `expires > now()` itself. If this never ran, no expired
 * session would come back to life — the tables would only grow.
 */
export function sweep() {
  const n = (ddcore as any).__authSweep();
  if (n.sessions || n.tokens || n.attempts) {
    ddcore.log.info(
      `auth sweep: ${n.sessions} session(s), ${n.tokens} token(s), ${n.attempts} attempt(s)`,
    );
  }
  return n;
}
