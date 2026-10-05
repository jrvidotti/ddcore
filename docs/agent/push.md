# Web Push

`ddcore.push.send` delivers a [Web Push](https://www.rfc-editor.org/rfc/rfc8030) message from server
code to a browser or an installed PWA: a notification for a customer, a reminder from a job. The
framework encrypts the payload for the subscription (RFC 8291, `aes128gcm`) and signs the request
with the site's VAPID key (RFC 8292, ES256). The app keeps the subscriptions, in a DocType of its
own, and decides what to send and to whom.

```ts
const res = ddcore.push.send(
  { endpoint: sub.endpoint, keys: { p256dh: sub.p256dh, auth: sub.auth } },
  { title: "Installment due tomorrow", body: "R$ 137.24", url: "/customer/installments" },
  { ttl: 86400, urgency: "normal" },
);
// res.status 201: accepted. 404 or 410: the subscription is gone, delete it.
```

Like everything else on the server, the call is synchronous: no `await`.

## Keys

A site has one VAPID pair. Generate it once:

```bash
ddcore push keys --subject mailto:ops@example.com
```

It prints three `.env` lines; put them in `.env` in development and in the platform's variables in
production, like any other secret (see `auth`):

| Variable | Meaning |
|---|---|
| `DDCORE_SECRET_VAPID_PUBLIC_KEY` | the 65-byte uncompressed P-256 public key, base64url |
| `DDCORE_SECRET_VAPID_PRIVATE_KEY` | the 32-byte private key, base64url |
| `DDCORE_SECRET_VAPID_SUBJECT` | a `mailto:` or `https:` URL a push service can reach the operator at; anything else is refused |

The format is the one the `web-push` tooling uses, so a pair made there works unchanged. `send`
checks that the public key is the private key's half. A missing variable is a `ValidationError`
naming it, never a value: `Secret DDCORE_SECRET_VAPID_SUBJECT is not configured on this site`.

Browsers subscribe with the public key, so **replacing the pair leaves every existing subscription
unusable**: each browser has to subscribe again. Generate the pair once per site and keep it.

## Giving the page the public key

`ddcore.push.publicKey()` returns the public key, or `null` while the site has none. The page needs
it before it can subscribe, so expose it with a method; it is public by design, so `allowGuest` is
fine:

```ts
// services/push.ts
import { whitelisted } from "@ddcore/sdk";

export const publicKey = whitelisted(() => ({ key: ddcore.push.publicKey() }), {
  allowGuest: true,
  methods: ["GET"],
});

// The push services browsers use; an endpoint anywhere else is refused.
const PUSH_HOSTS = [/^fcm\.googleapis\.com$/, /\.push\.services\.mozilla\.com$/, /\.push\.apple\.com$/, /\.notify\.windows\.com$/];

// Stores the browser's subscription for the signed-in user.
export const subscribe = whitelisted((args: { endpoint: string; keys: { p256dh: string; auth: string } }) => {
  const m = /^https:\/\/([^/:?#@]+)[/:]/.exec(String(args.endpoint));
  if (!m || !PUSH_HOSTS.some((re) => re.test(m[1].toLowerCase()))) ddcore.throw("Invalid subscription");
  const existing = ddcore.db.exists("Push Subscription", { endpoint: args.endpoint });
  if (existing) return { id: existing };
  const doc = ddcore.newDoc("Push Subscription", {
    user: ddcore.session.user,
    endpoint: args.endpoint,
    p256dh: args.keys.p256dh,
    auth: args.keys.auth,
  });
  doc.insert({ ignorePermissions: true });
  return { id: doc.id };
});
```

`Push Subscription` is the app's DocType; the framework has none. An endpoint is a capability URL:
whoever has it and the keys can send to that browser, so keep the DocType readable only by the
roles that send.

The endpoint is whatever the browser sent, and `send` makes a request to it from the server. So
`subscribe` above **stores only an `https:` endpoint on a known push service**:
`fcm.googleapis.com` (Chrome), `*.push.services.mozilla.com` (Firefox),
`*.push.apple.com` (Safari) and `*.notify.windows.com` (Edge on Windows). Add a host to the list
only for a push service you know of. `send` itself refuses a plain `http:` endpoint outside
development, this machine included, and never follows a redirect: a `3xx` comes back as the
answer. It does **not** refuse an `https:` endpoint on a private address (`https://10.0.0.5/`,
`https://localhost/`): only the host check in `subscribe` keeps a signed-in user from pointing
the server at the site's own network.

## Subscribing in the browser

In the page (a `www` site, see `www`), with a service worker registered:

```ts
const reg = await navigator.serviceWorker.register("/r/sw.js");
const { data } = await (await fetch("/api/method/portal.services.push.publicKey")).json();
if (data.key && (await Notification.requestPermission()) === "granted") {
  const sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: data.key });
  await fetch("/api/method/portal.services.push.subscribe", {
    method: "POST",
    // a cookie-authenticated POST without it is refused with 403
    headers: { "Content-Type": "application/json", "X-DDCore-CSRF": "1" },
    body: JSON.stringify(sub.toJSON()),   // { endpoint, keys: { p256dh, auth } }
  });
}
```

And in the service worker, show what arrives:

```js
// sw.js
self.addEventListener("push", (event) => {
  const msg = event.data ? event.data.json() : {};
  event.waitUntil(self.registration.showNotification(msg.title ?? "", { body: msg.body, data: msg }));
});
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  if (event.notification.data?.url) event.waitUntil(clients.openWindow(event.notification.data.url));
});
```

## Sending, and deleting the subscriptions that are gone

`send(subscription, payload, opts?)` takes the subscription as the browser's `toJSON()` gives it.
A string payload goes as is; any other value goes as JSON. The payload is at most **3993 bytes**,
what fits in the single 4096-byte record a push service is required to accept; a longer one is a
`ValidationError`. Send an id and let the page fetch the rest when there is more to say.

| Option | Default | Meaning |
|---|---|---|
| `ttl` | `86400` | seconds the push service keeps the message for a browser that is offline; `0` delivers now or never |
| `urgency` | (none) | `very-low`, `low`, `normal` or `high`, sent as the `Urgency` header; without it the push service treats the message as `normal` |
| `topic` | (none) | up to 32 base64url characters: a newer message with the same topic replaces one still waiting |

It returns the push service's answer, `{ status, body, headers }`, **whatever the status**: only a
request that got no answer (a timeout of 15 seconds, a refused connection), a missing key or a bad
argument throws. That leaves the decision to the app:

- `201` — accepted for delivery.
- `404` or `410` — the subscription expired or the user revoked it. Delete it; it will never work
  again.
- `413` — the payload was too large for this service. `429` — slow down (`headers["Retry-After"]`).
  `400` or `403` — the request or the VAPID key was refused; `body` says why.

```ts
// services/reminders.ts — run from a job
export function notifyUser(args: { user: string; title: string; body: string; url: string }) {
  const subs = ddcore.db.getAll("Push Subscription", { filters: { user: args.user }, fields: ["id", "endpoint", "p256dh", "auth"] });
  for (const s of subs) {
    const res = ddcore.push.send(
      { endpoint: s.endpoint, keys: { p256dh: s.p256dh, auth: s.auth } },
      { title: args.title, body: args.body, url: args.url },
    );
    if (res.status === 404 || res.status === 410) {
      ddcore.deleteDoc("Push Subscription", s.id, { ignorePermissions: true });
    } else if (res.status >= 400) {
      ddcore.log.warn("push refused", String(res.status));
    }
  }
}
```

A send is an HTTP call made at once, inside the request or job that makes it, and it is not
undone if the transaction rolls back. Sending to many subscriptions belongs in a job
(`ddcore.enqueue`), not in a desk request.

## Out of scope

- Storing subscriptions: the app's DocType, as above.
- Retries and a delivery record, as outgoing webhooks have: a push message is either taken now or
  not, and the app decides what a failure means.
- More than one VAPID pair per site.
