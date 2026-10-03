# Design: App static sites under a prefix (`www`) and per-method CORS (#68)

Record of the design agreed on 2026-10-03. The contract for app authors is
[`docs/agent/www.md`](../../agent/www.md) and the "Calls from another origin" section of
[`docs/agent/controller-api.md`](../../agent/controller-api.md); this document keeps **why** each
piece is the way it is.

## Starting point

#68 is still open. Its `pathTail` part shipped in 0.25.0. The payment gateway built on ddcore
still runs a separate SvelteKit service (`checkout/`) only to:

- serve the public payment page `/r/[code]`;
- forward `pix`, `status` and `simulate` to the `gateway.services.checkout.*` methods, with an
  API token kept on its server;
- proxy the tenant's logo;
- forward the Sicoob webhook.

That is one more deploy, one more credential and one more hop on the path of the money. On top
of that, the merchant's front end has to call some gateway methods from another origin, and
ddcore has no CORS at all.

Decisions taken with the owner of the request:

| Question | Decision |
| --- | --- |
| Pages | The app's static SPA is served under a prefix, with a fallback. No SSR, no routes with a TS handler. The SPA calls `allowGuest` methods on the same origin |
| CORS | A site-wide allowlist of origins plus a per-method opt-in (`cors: true`), always without credentials (no cookie) |
| Already solved without changing ddcore | The Sicoob webhook uses `pathTail` (0.25.0); the logo uses `ddcore.files.presign` |
| Out of scope | Routes with a TS handler, SSR, tenant by hostname, anonymous forms, CORS on `/api/resource`, per-tenant dynamic origins, cross-origin cookies |

Expected outcome: the checkout becomes an `adapter-static` build inside the gateway app, and the
separate service goes away. Migrating the gateway itself happens later, in its own repository.

## 1. `www`: the app's static folder under a prefix

```ts
defineApp({ www: { "/r": { dir: "checkout/build", fallback: "index.html", frame: false } } })
// short form: www: { "/r": "checkout/build" }  (same as fallback "index.html", frame false)
```

### Declaration and validation

- `AppDef.www?: Record<string, string | { dir: string; fallback?: string | null; frame?: boolean }>`.
  It reaches Go through the snapshot's `apps`: the registry already exists (`prelude.js`
  `case "app"`, and `reg.meta()` returns `apps`).
- `Engine.Load`, next to the portal checks and `routeNameClash`, normalises and validates it:
  - the prefix starts with `/`, is a single segment (`^/[a-z0-9][a-z0-9_-]*$`) and clashes
    neither with a reserved path (`api`, `app`, `login`, `portal`, `_app`, `assets`, `files`,
    `private`, `mcp`, `healthz`, `readyz`) nor with another app's prefix;
  - `dir` is relative and, after `filepath.Clean`, stays inside the app's directory.

  The result is a table on the `State` (`prefix → {app, absDir, fallback, frame}`), read through
  `State.WWW(prefix)`.
- An error fails `Load` with a clear message, like the other checks. A directory that does not
  exist yet is **not** an error, because the build may come later; until it exists the prefix
  answers 404.

### How it is served (`internal/api/www.go`)

- The router is built once and hot reload swaps the `State`, so the lookup happens per request:
  `NotFound` becomes a handler that tries the first path segment against the current state's
  `www` table and falls back to the desk. A prefix added or removed under `dev` takes effect at
  once.
- Only `GET` and `HEAD`; any other verb gets 405.
- The path inside the prefix is cleaned and refused if it escapes (`..`):
  - an existing file in the directory is served with `http.ServeContent`, its content type from
    the extension;
  - a directory serves its `index.html`;
  - anything else serves the `fallback` when there is one, and 404 without one.
- `/r`, without the trailing slash, redirects with 308 to `/r/`.
- **Headers:**
  - `.html`: `Cache-Control: no-cache`;
  - paths under `_app/immutable/` or `immutable/`: `Cache-Control: public, max-age=31536000, immutable`;
  - everything else: `Cache-Control: public, max-age=300`;
  - always: `X-Content-Type-Options: nosniff` and `Referrer-Policy: strict-origin-when-cross-origin`;
  - `X-Frame-Options: DENY`, unless `frame: true`.
- **Access:** the files are public (Guest); `requireLogin` does not apply. Maintenance mode
  already lets GET through. The data stays protected by the methods.
- **Desk:** `/_app` stays the desk's. The app's SPA has to be built with its base equal to the
  prefix (SvelteKit `kit.paths.base = "/r"`), so its assets live under `/r/_app/...`.

## 2. Per-method CORS

```jsonc
// ddcore.json
"cors": { "origins": ["https://shop.example.com", "https://*.partner.com"] }
// env: DDCORE_CORS_ORIGINS="https://a.com,https://b.com"  ("*" = any origin)
```
```ts
export const status = whitelisted(fn, { allowGuest: true, cors: true });
```

### Configuration (`internal/config/config.go`)

- `CORS struct { Origins []string }` in `File`.
- The `DDCORE_CORS_ORIGINS` variable, comma-separated, overrides the file.
- The format is validated: `*`, `scheme://host[:port]` or `scheme://*.domain`.

### Middleware (`internal/api/cors.go`)

- Applies only to `/api/method/{path}` and `/api/method/{path}/*`.
- The method name is read as `confineWebsiteUsers` reads it: the first segment, escaped and then
  decoded.
- **Preflight** (`OPTIONS` with `Origin` and `Access-Control-Request-Method`): when the method is
  whitelisted with `cors: true` and the origin matches the allowlist, it answers `204` with:
  - `Access-Control-Allow-Origin: <the origin, echoed>`;
  - `Access-Control-Allow-Methods` from the method's `methods`, or `GET, POST`;
  - `Access-Control-Allow-Headers: Authorization, Content-Type, X-Tenant`;
  - `Access-Control-Max-Age: 600` and `Vary: Origin`.

  Otherwise it answers `403` with no CORS headers. The `OPTIONS` routes have to be registered in
  chi, because today an `OPTIONS` falls through to the desk.
- **Actual request** with an accepted `Origin`: adds `Allow-Origin` (echoed), `Vary: Origin` and
  `Access-Control-Expose-Headers: X-Request-Id`, and the handler runs as usual.
- It **never** sends `Access-Control-Allow-Credentials`. Authentication is `Authorization: token`
  or Guest.
- An `Origin` equal to the site's own, or none at all, is not CORS. A method without `cors`, or an
  origin outside the list, goes on without headers, exactly as today.
- **Origin matching:**
  - `*` matches any origin;
  - `https://*.partner.com` matches any subdomain (not the bare domain), with the exact scheme;
  - everything else is an exact, case-insensitive comparison.
- **SDK:** `cors?: boolean` on `WhitelistOpts`. The option travels through the same generic
  `opts` map `pathTail` already uses, so no Go metadata changes.
