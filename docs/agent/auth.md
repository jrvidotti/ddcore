# Access: sign-in, recovery, invitation and secrets

What the framework guarantees about getting in, staying in, and getting back in.

## The policy lives in two files

`ddcore.json` holds what the site decided, because staging has to lock an
account out exactly as production does or the rehearsal proves nothing:

```jsonc
"auth": {
  "sessionDays": 30,          // session lifetime; the cookie follows it
  "apiKeyDays": 0,            // 0 = an API key never expires
  "minPasswordLength": 8,
  "maxLoginAttempts": 5,      // failures before the lockout
  "lockoutMinutes": 15,
  "resetMinutes": 60,         // how long a recovery link is good for
  "inviteHours": 72,
  "secureCookie": null,       // null = decide per request
  "selfServiceApiKeys": true,
  "passwordLogin": true       // false = single sign-on only (Admin keeps a password)
}
```

`.env` holds where the site is running — the public URL a link is built from,
whether a proxy is trusted, and everything about mail. See `.env.example`;
`ddcore doctor` reports which transport is live and warns when no public URL is
set.

An invalid value is refused at load. A session that lasts zero days or a
lockout after zero attempts is a policy nobody meant to write.

## Signing in

`POST /api/login` with `{usr, pwd}`. On success a `sid` cookie: `HttpOnly`,
`SameSite=Lax`, `Max-Age` from `sessionDays`, and `Secure` when the request
arrived over TLS or the site URL is https.

Failures are counted in `ddcore_login_attempt` and answered with **429
`TooManyRequestsError`**, a `Retry-After` header and `error.extra.retryAfter`.
Three properties are load-bearing:

- The throttle runs **before** the user lookup and before any hashing. Argon2
  costs 64 MiB a call, so an unthrottled login is a memory-exhaustion vector
  needing no credentials.
- The counter is keyed on **what was typed**, not the account it resolved to,
  and an unknown address is still checked against a decoy hash. Otherwise the
  fast, never-locking answer for an address nobody owns tells an attacker which
  addresses exist — and the lockout becomes the oracle it was meant to close.
- The correct password is **not** a way out of a lockout.

`ddcore user unlock <email>` lifts one early. An account can be locked under
two spellings (its name and its e-mail), and the command clears both.

`trustProxy` decides whether `X-Forwarded-For` is believed. Leave it off unless
a proxy you control sets it: otherwise any client picks the address we hold
responsible, and the address throttle becomes a way to lock out a stranger.

### A notice on the sign-in screen

A deployment can put a notice above the form and offer a demo account, with a
button that fills the form in. Set it in `ddcore.json` or, per deployment, in
the environment, which wins field by field:

```json
{ "login": { "notice": "Public demo — data resets every 6 hours.", "demoUser": "visitor@example.com", "demoPassword": "demo-visitor" } }
```

```env
DDCORE_LOGIN_NOTICE=Public demo — data resets every 6 hours.\nFeel free to break things.
DDCORE_LOGIN_DEMO_USER=visitor@example.com
DDCORE_LOGIN_DEMO_PASSWORD=demo-visitor
```

- All three go out in the public `/api/boot` (`site.login`) to anyone who opens
  the site. They are for a shared demo account or a maintenance note, **never a
  real account's password**. Give the demo user a role that cannot administer
  the site.
- The notice is plain text, shown as written, with its line breaks; `\n` in the
  variable is one. It is the operator's sentence, not a catalogue key, so it is
  not translated.
- The demo account is offered only when both user and password are set.

## Single sign-on (OpenID Connect)

Google, a self-hosted [PocketID](https://pocket-id.org), or any other OpenID
Connect provider. It is one generic client: the authorization code flow with
PKCE, discovery from the issuer, and the `id_token` verified for signature,
issuer, audience, expiry and nonce. Providers differ only in their variables.

```bash
DDCORE_URL=https://erp.example.com             # required: the callback is built from it
DDCORE_OIDC_PROVIDERS=google,pocketid

DDCORE_OIDC_GOOGLE_CLIENT_ID=...               # issuer defaults to https://accounts.google.com
DDCORE_OIDC_GOOGLE_CLIENT_SECRET=...
DDCORE_OIDC_GOOGLE_ALLOWED_DOMAINS=example.com # optional

DDCORE_OIDC_POCKETID_ISSUER=https://id.example.com
DDCORE_OIDC_POCKETID_CLIENT_ID=...
DDCORE_OIDC_POCKETID_CLIENT_SECRET=...
# optional for any provider: _LABEL (the button), _SCOPES (default "openid email profile")
```

A provider id starts with a letter and goes on in lowercase letters, digits and
`_`, up to 32 characters; the list is lowercased as it is read and a repeated id
is refused. Any id other than `google` needs `_ISSUER`, which must be an
`http(s)` URL and loses a trailing `/`. A provider missing its client
credentials, or configured without `DDCORE_URL`, is refused at load. `_SCOPES`
is separated by commas or spaces and always gains `openid`; `_ALLOWED_DOMAINS`
is comma-separated, matched case-insensitively, and a leading `@` is dropped;
`_LABEL` defaults to *Google*, *PocketID* or the id itself.

**Registering the client.** The redirect URI is
`<DDCORE_URL>/api/auth/oidc/<id>/callback`. `ddcore doctor --json` carries it
per provider in `sso.providers[]` together with a five-second discovery probe —
a provider that does not answer is a warning, never critical; the text report
only names the configured ids and whether password sign-in is on. Discovery is
fetched once per provider and kept, with a ten-second timeout on every call out;
a failure is not kept, so a provider that was down is tried again on the next
sign-in rather than until a restart.

- Google: Google Cloud Console → APIs & Services → Credentials → *OAuth client
  ID*, type *Web application*, with that URI under *Authorized redirect URIs*.
- PocketID: *OIDC Clients* → *Add*, with that URI as the callback URL. It is a
  confidential client, so leave *Public client* off; PKCE is always sent.

**Nobody gets an account this way.** The provider proves who someone is, and the
site still decides whether they may use it:

1. The `email` claim must be present and `email_verified` must be true.
2. The address must be in `_ALLOWED_DOMAINS`, when that list is set — checked on
   every sign-in, not only the first, so narrowing the list takes effect at once.
3. The first sign-in links the provider's `sub` to the **enabled** User whose
   e-mail is that address, or failing that whose name is, compared without case.
   `Guest` is never matched; `Admin` is. Later sign-ins resolve by `sub`,
   so an address changed at the provider still lands on the same account; a
   linked User since disabled is refused with `disabled`, and a link left
   pointing at a deleted User falls back to matching the address again. The link
   is kept in `ddcore_user_identity`; deleting or renaming the User carries it
   along.

Invite people first (`ddcore user invite`, or the desk). They can then sign in
through the provider without ever accepting the invitation.

**The flow.**

- `GET /api/auth/oidc/<id>/start?redirect=/app/...` stores a single-use state —
  hashed, with a nonce and a PKCE verifier — in `ddcore_oidc_state` for ten
  minutes, binds it to the browser with an `HttpOnly` `ddcore_oidc_state` cookie
  (path `/api/auth/oidc/`, `SameSite=Lax`, the same ten minutes), and redirects
  to the provider. The route takes no session and writes a row per request; the
  sweep below clears the expired ones.
- `GET /api/auth/oidc/<id>/callback` requires the state in the query string and
  in the cookie to match, spends it, exchanges the code, and sets the same `sid`
  cookie a password sign-in sets.
- Only a path on this site is accepted as `redirect`: `//host`, `/\host` and
  absolute URLs fall back to `/app`.

A failure of the flow redirects to `/login?sso_error=<code>`, where the desk
shows a translated message. The codes are `state`, `provider`,
`unverified_email`, `no_account`, `disabled`, `domain`, `throttled` and
`server`. A cancelled sign-in arrives as `provider`; a failure on our own side —
a database that went away, a bug — is `server`, so the logs do not send anyone
to the identity provider for it. The desk shows the same general sentence for
both.
An id that is not configured is the exception: `start` answers **404
`DoesNotExistError`** as JSON, since nothing is under way to send anywhere.

Callback failures are throttled per **client address**, `maxLoginAttempts × 5`
inside the lockout window — the looser limit, because an office shares one
address. A completed sign-in clears both that account's password-login failures
and the address's, as a password sign-in does.

**Audit.** A callback writes `account.login_sso`, `Allowed` or `Denied`, with
the provider and the reason in the detail. Two paths never reach it: a throttled
callback, and the provider's own `?error=` return, which is answered before the
callback runs. A first link also writes `account.identity_link`.

**Password sign-in off.** Set `"passwordLogin": false` and `POST /api/login`
refuses everyone but `Admin`, and forgot-password sends nothing to
anyone else. The refusal is answered only once the password has verified, for
the reason a disabled account is. The desk hides the form behind an
*Admin sign-in* link.
Admin keeps a password so that an outage at the provider is not also an
outage of the site's administration. Setting `false` with no provider
configured is refused at load.

`/api/boot` exposes `site.login.password` and `site.login.providers`
(`{id, label}` only) to the sign-in screen.

## Sessions

One TTL, read from the policy in both the SQL and the cookie. Sessions record
`ip` and `user_agent` so a person can recognise their own devices.

Every path that sets a password ends the user's **other** sessions — the User
form, `ddcore user passwd`, a recovery, and self-service. A recovery ends *all*
of them, because the reason for a reset may be that the account is no longer
theirs. Self-service spares the caller's own, so nobody is thrown out of the
tab they just typed into.

A session is addressed from app code by an opaque handle, never by its sid. The
sid is a bearer token: one XSS able to read a session list would otherwise hand
over every device.

## Recovery and invitation

Four public routes, all throttled, all exempt from the CSRF header check
because there is no session yet to forge a request from:

| Route | Body | 200 |
|---|---|---|
| `POST /api/auth/forgot-password` | `{usr}` | `{ok:true}` **always** |
| `POST /api/auth/token` | `{token}` | `{kind, user, fullName, expires}` |
| `POST /api/auth/reset-password` | `{token, password}` | `{ok:true}` |
| `POST /api/auth/accept-invite` | `{token, password, fullName?}` | `{ok:true}` |

`forgot-password` answers identically for an unknown address, a real one and a
disabled account — status, body and timing. Timing is free: there is no Argon2
on that path, and the message goes through the job queue rather than out a
socket.

Tokens are 192 bits of `crypto/rand`, stored as their **SHA-256**, single-use,
and spent in one statement so two submissions of the same link cannot both set
a password. A kind is checked, so a pending invitation is not also a way to
reset somebody's password. Completing a recovery does **not** sign the person
in: one way to get a session is enough to reason about.

An invited account is created with **no password**, which needs no flag —
an empty hash already refuses every sign-in until one is set.

## Mail

`DDCORE_MAIL_TRANSPORT` is `log` (the default: the link goes to the log and is
handed back to whoever asked), `smtp`, or `method`. `method` names an app
function by dotted path in `DDCORE_MAIL_METHOD` — the same way `scheduler` and
`ddcore.enqueue` already name app code.

The invitation and the recovery message are two ordinary templates,
`core/mail/invite.mail.ts` and `core/mail/reset.mail.ts`, sent the way an app
sends anything — see [mail](mail.md). Both declare `sensitive`, because their
one argument is a link that can set somebody's password: the `Email Delivery`
record says who it went to and whether it arrived, and holds no part of the
link.

Messages go through `ddcore_job`, so retries are durable, and because the
delivery record and the job are both written on the request's transaction a
message is only queued if the request commits. The SMTP sender refuses to
authenticate in the clear to anything but loopback.

## Self-service (`core/services/`)

A user has **no write on their own User record**, and granting it is not an
option: `roles` is a child table on User, so write would be self-promotion to
System Manager. These services exist to offer the narrow thing instead, and
each checks `ddcore.session.user` before touching anything:

- `profile.getMyProfile` · `profile.updateMyProfile({fullName?, language?})` ·
  `profile.changeMyPassword({current, password})`
- `sessions.listMySessions` · `sessions.revokeMySession({id})` ·
  `sessions.revokeMyOtherSessions`
- `api_keys.listMyAPIKeys` · `api_keys.createMyAPIKey({label, days?})` ·
  `api_keys.revokeMyAPIKey({name})`

`updateMyProfile` enumerates its two fields and never spreads `args` — that
enumeration is the security of the function, since the write underneath skips
the permission check.

System Manager only: `users.invite`, `users.resendInvite`,
`users.sendPasswordReset`, `users.revokeUserSessions`, `users.unlockUser`.

## Passwords

The policy is applied where hashing happens — the `hashPassword` host op and
`Engine.SetPassword` — rather than at each of the five call sites, so app code
nobody has written yet meets it too. Minimum length (in runes), not blank, not
the account name, and a cap so an unauthenticated caller cannot choose how much
Argon2 the server does. No complexity ruleset: those produce `Password1!` and a
sticky note.

## Secrets

An integration credential is read from the environment, never from a column:

```ts
const key = ddcore.secret("stripe_key");   // DDCORE_SECRET_STRIPE_KEY
```

A secret in the database is a secret in every backup, replica, export and
`Version` diff; one in the environment is in none of them because it was never
written down, and rotating it is a redeploy rather than a migration. The
prefix is the boundary — an app reads its own secrets and nothing else the
process was started with.

A `Password` field is for a secret a *person* types. It is still plain text at
rest; what the framework guarantees is that it does not leave — blanked on
every read through the API, absent from `Version`, absent from an export.

## Housekeeping

`core.services.auth.sweep` deletes expired sessions, old tokens, old
attempts and expired single sign-on states, hourly, where the site enables the scheduler. It is hygiene and never
correctness: every read path filters on expiry itself, so if it never ran
nothing would become valid again — the tables would only grow.

## Not here yet

A real CSRF token (the check is header-presence only), MFA, LDAP and automatic
provisioning or role mapping from a provider (SEC-05, in the demand-driven
backlog), provider-initiated (single) logout, a screen to see or remove one's
linked identities (unlinking is a `DELETE` in `ddcore_user_identity`), e-mail
verification on a changed address, and an admin UI for unlocking or listing
another user's sessions.

Sharp edges of the single sign-on that is here: provider configuration is read
at boot, so a change to `DDCORE_OIDC_*` needs a restart and not a reload; with
password sign-in off, an invitation or an admin's reset link still sets
a password nobody but Admin can use; and `email_verified` is taken at
the provider's word, so a provider is trusted for every address it says it
checked.
