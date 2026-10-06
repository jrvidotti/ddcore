# App sites (`www`)

An app can serve a static site to anyone, under a URL prefix of its own: a public payment
page, a status page, a sign-up form. The site is files only — typically the build of a
SPA — and everything it shows comes from the app's guest methods, called on the same
origin. There is no server-side rendering and no route with a TypeScript handler: the
data is guarded by the methods, the files are public.

```ts
// ddcore.app.ts
export default defineApp({
  name: "gateway",
  title: "Gateway",
  www: {
    "/r": "client/checkout/build",                 // short form
    "/status": { dir: "client/status/dist", fallback: null, frame: true },
  },
});
```

| Key | Meaning |
| --- | --- |
| prefix | One lowercase path segment (`/r`, `/pay`, `/status-page`). Not one ddcore answers itself: `/api`, `/app`, `/login`, `/portal`, `/_app`, `/assets`, `/files`, `/private`, `/mcp`, `/healthz`, `/readyz`; and not one another app took. |
| `dir` | The folder served, relative to the app and inside it — not the app's root. It may not exist yet: until the build is there, the prefix answers 404. |
| `fallback` | The file served for a path that names no file, so a client-side route like `/r/ABC123` loads the SPA's shell. Default `"index.html"`; `null` answers 404 instead. |
| `frame` | `true` lets other pages embed the site in a frame. Default `false`. |

A declaration that breaks one of these rules stops the load with a message naming the app
and the prefix, like any other metadata error.

## How it is served

- `GET` and `HEAD` only; anything else is `405`.
- `/r` redirects (308) to `/r/`, keeping the query string.
- A path naming a file serves it, with the content type of its extension. A directory serves
  its `index.html`. Anything else serves `fallback`, or `404`.
- A path that climbs out of the folder (`..`, `%2e%2e`, `..%2f`) is a `404`. Symlinks out of
  the folder are not followed.
- Headers on every file: `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: strict-origin-when-cross-origin`, and `X-Frame-Options: DENY` unless
  `frame: true`. Caching: `.html` is `no-cache`; files under `_app/immutable/` or `immutable/`
  (hashed names) are cached for a year; everything else for five minutes.
- The files are public: no sign-in, and maintenance mode still serves them (reads stay open).
- The folder is read on every request, so a rebuild shows at once. A prefix added or removed
  in `ddcore.app.ts` takes effect on the next reload, as `ddcore dev` does by itself.

## Where the source lives

Put the site's project under the app's **`client/`** folder. ddcore compiles every `.ts`
file of an app into its server bundle, except under `client/`, `node_modules/` and folders
starting with a dot, so a SvelteKit or Vite project anywhere else gets its `src/*.ts` loaded
as server code and the app fails to load. Commit the source; build it in your deploy step
(or commit the build, if the image has no Node).

## A SvelteKit site

`client/checkout/svelte.config.js`:

```js
import adapter from "@sveltejs/adapter-static";

export default {
  kit: {
    adapter: adapter({ pages: "build", assets: "build", fallback: "index.html" }),
    // The site lives at /r: its bundles must be asked for at /r/_app/…, since /_app/…
    // is the desk's.
    paths: { base: "/r" },
  },
};
```

Turn off server rendering and prerendering for the dynamic routes (`export const ssr = false`
in `src/routes/+layout.ts`): there is no Node server behind the site. Links inside the SPA use
`base` from `$app/paths`, so `/r/ABC123` stays under the prefix.

The page reads its data with `fetch`, on the same origin, as Guest:

```ts
const res = await fetch(`/api/method/gateway.services.checkout.status?code=${encodeURIComponent(code)}`);
const { data } = await res.json();
```

## The methods behind it

A site's methods are `allowGuest`, and return only what the page shows — never a whole
document. Guest has no role permission on anything, so they read with
`{ ignorePermissions: true }` and pick the fields themselves.

On a site with tenancy, **Guest works in the platform space** and sees no tenant's rows. The
method finds which tenant the request is about, from something the platform space can read,
and enters it with `ddcore.tenant.run`:

```ts
// services/checkout.ts
import { whitelisted } from "@ddcore/sdk";

export const status = whitelisted((args: { code: string }) => {
  // "Payment Link" is a `shared: true` DocType: platform rows mapping a public code to its tenant
  const link = ddcore.getDoc("Payment Link", String(args.code ?? ""), { ignorePermissions: true });
  return ddcore.tenant.run(link.tenant, () => {
    const charge = ddcore.getDoc("Charge", link.charge, { ignorePermissions: true });
    return { status: charge.status, amount: charge.amount, payee: charge.payee_name };
  });
}, { allowGuest: true, methods: ["GET"] });
```

A code in a public URL is a credential: make it long and random (not a series), and let it
expire. See `tenancy` for what the platform space can do.

### Images

A tenant's logo is a `File`. The method hands the page a link the browser can load without a
session:

- with the **s3** storage backend, `ddcore.files.presign(fileUrl, { ttl: 300, ignorePermissions: true })`
  returns a short-lived URL of the bucket itself (see `storage`);
- with the **local** backend, which cannot presign, store the logo as a public file
  (`isPrivate: false`): its `/files/…` URL answers anyone who has it.

### Rate limiting

A guest method can be called by anyone, as often as they like. ddcore has no public throttle
for app code yet; until it does, keep a counter in `ddcore.cache`, keyed by what the caller
names:

```ts
function limit(key: string, max: number, seconds: number) {
  const n = (ddcore.cache.get(key) ?? 0) + 1;
  if (n > max) ddcore.throw("Too many requests, try again in a minute", { type: "TooManyRequestsError", retryAfter: seconds });
  ddcore.cache.set(key, n, seconds);
}

export const pix = whitelisted((args: { code: string }) => {
  limit("pix:" + args.code, 5, 60);
  // …
}, { allowGuest: true, methods: ["POST"] });
```

The cache lives in each process, so a site served by several processes allows each one the
budget, and every `set` restarts the window. For a hard limit per client address, put it in
the proxy in front of ddcore.

## Calling from another origin

A site served by ddcore calls on its own origin and needs no CORS. A page served from somewhere
else calls methods that opt in with `cors: true`; see `controller-api` → "Calls from another
origin".
