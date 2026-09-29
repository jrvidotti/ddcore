/**
 * The identity provider's job target: pushes a User's mapped roles to it as
 * groups. Everything happens in Go (internal/engine/pocketid.go); this gives
 * it a dotted path the job worker can name. Not whitelisted — nobody calls it
 * over HTTP. Throwing asks the job to run again with its backoff.
 */
export function sync(args: { user: string }) {
  (ddcore as any).__idp.sync(args.user);
}
