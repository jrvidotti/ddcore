# Operations: probes, correlation and `doctor`

Three things an operator needs and an app never declares: whether the process is
alive, whether it can do work, and which line in the log belongs to the
complaint someone just filed.

## Liveness is not readiness

| Route | Question | Touches the database | Answer |
|---|---|---|---|
| `GET /healthz`, `GET /api/health` | is this process still serving? | no | always `200` `{"ok":true,"status":"alive"}` |
| `GET /api/ready`, `GET /readyz` | can it do useful work now? | yes | `200` `{"ok":true,"status":"ready"}` or `503` `{"ok":false,"status":"unready","reason":"database"}` |

They are separate because the answers lead to opposite actions. A failed
liveness probe means *restart this process*; a failed readiness probe means
*stop sending it traffic and leave it alone*. Restarting every application
server because Postgres went down does not bring Postgres back.

Both are reachable without credentials, and an invalid `Authorization: token …`
is ignored on them — on any other route it answers `401` before the handler
runs, which would otherwise let a monitoring agent's expired key report a
healthy process as dead.

A trailing slash is accepted on all four (`/readyz/` works). That is not
tidiness: an unmatched path falls through to the desk, which answers `200` with
`index.html`, so a probe configured with a stray slash would otherwise pass
forever — including while the database is down.

**The readiness body is a boolean on purpose.** No latency, no version, no queue
depth, no database error text. Latency is a feedback channel for anyone
measuring their own effect on the site; queue depth leaks business volume and
says when the site is under load; the version says which advisories apply; a
Postgres error string carries host, port, user and database name. An
orchestrator reads the status code and nothing else, so the disclosure would buy
nobody anything.

Readiness is memoised for one second. A probe answered every few seconds by
every replica would otherwise be a database round trip anyone can trigger at
will. The cost is that a database that has just died is reported ready for up to
a second.

### Probing from a container

There is deliberately no `HEALTHCHECK` in the published image: the entrypoint is
the binary itself, and the same image runs `ddcore migrate` and `ddcore doctor`
as one-shot containers that a baked healthcheck would mark unhealthy.

```yaml
# docker compose
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:8090/readyz"]
  interval: 10s
  timeout: 3s
  retries: 3
```

```yaml
# kubernetes
livenessProbe:  { httpGet: { path: /healthz, port: 8090 }, periodSeconds: 10 }
readinessProbe: { httpGet: { path: /readyz,  port: 8090 }, periodSeconds: 5 }
```

## The health report

`GET /api/health/report` is the same picture with the numbers in it, and it
requires the **System Manager** role. It answers `{ data: … }` like any other
endpoint:

```json
{ "status": "warn",
  "version": "0.4.1",
  "database": { "ok": true, "latencyMs": 2.1, "conns": 3, "idle": 2, "maxConns": 10 },
  "queue": { "queued": 12, "runnable": 11, "running": 2, "stalled": 0,
             "failedInWindow": 1, "doneInWindow": 340,
             "oldestQueuedSeconds": 252, "longestRunningSeconds": 3, "windowMinutes": 15 },
  "errors": { "inWindow": 3, "windowMinutes": 15,
              "latest": [{ "name": "…", "method": "job:demo.tasks.sweep", "requestId": "job:412" }] },
  "scheduler": { "enabled": true, "entries": 4 },
  "warnings": ["queue backlog: 214 runnable jobs (limit 100)"] }
```

`status` is `ok`, `warn` or `down`. Only `down` — the database not answering —
changes an HTTP status code anywhere; thresholds produce `warn` and never a
`503`, because a readiness probe that failed on a deep queue would have the
orchestrator restart the very workers draining it.

Two numbers in `queue` are easy to confuse. `queued` is every job waiting;
`runnable` is the subset whose `runAfter` has arrived. Only `runnable` means
*behind* — a job enqueued to run tomorrow is not a backlog, and it does not age
the queue either. `stalled` is running jobs whose lease expired: the worker
holding them is gone, and it is the same set a live worker would put back.

## Request correlation

Every request is given an identifier, and the same string appears in four
places:

- the `X-Request-Id` response header;
- the `requestId` field of any error body;
- the access log line for that request;
- the `request_id` column of any `Error Log` row it produced.

An inbound `X-Request-Id` is honoured when it is 8–64 characters of
`A–Z a–z 0–9 - _ .` — which admits UUID, ULID and a W3C `traceparent`. Anything
else is replaced with a generated id rather than escaped: the value reaches a
log line and a text column, so a newline in it would forge a log entry and a
megabyte of it would flood the column.

Server-side app code reads it from the session:

```ts
export const sync = whitelisted(() => {
  ddcore.log.info(`syncing for ${ddcore.session.requestId}`);
});
```

`ddcore.session.requestId` is empty in a job, a migration or a test — there is
no request behind that work, and the emptiness is information. A failed job run
still gets a handle of its own (`job:<id>`) on the `Error Log` row it writes.

In the desk, a `DDCoreError` carries `requestId`, and the toast for a `500`
shows it in brackets so a user can quote it.

## The access log

One line per request, at most:

```
level=INFO msg=request id=9f1c… method=GET path=/api/resource/Task status=200 ms=14 user=ana@x.com ip=10.0.0.4
```

Only the path is logged, never the query string: a list filter carries personal
data and a recovery link carries a token.

The level is chosen so the log stays readable. A single desk page is around
thirty requests through the same chain — the SPA fallback, every asset, every
file — so a `200` on a static asset is `debug`, an `/api/…` request is `info`, a
`4xx` is `warn`, a `5xx` is `error`, and anything slower than `ops.slowRequestMs`
is `warn` wherever it came from. `DDCORE_DEBUG` logs everything.

The server writes the log to **stdout**, never to stderr: a platform that
captures both streams — Railway, Cloud Run, Kubernetes — reads a line on stderr
as an error by definition, so an `INFO` access line would arrive painted red and
the level policy above would stop meaning anything. Every other command keeps
its log on stderr, because there stdout is the command's own output — an
exported NDJSON, a printed key, `doctor --json`, the MCP JSON-RPC stream.

`DDCORE_LOG_FORMAT` picks the shape: `json` for objects, `text` for lines. With
nothing set the process asks the log's destination — a terminal gets text,
anything else (a pipe, a file, a container's log stream) gets JSON, since a
collector has to parse a text line and grouping by request id, most of what the
id is for, stops working. Deployed, that means JSON without configuring
anything, and a collector reads the level from the `level` field of each object
instead of guessing it from the stream.

A panic is caught, logged with its stack, written to `Error Log`, and answered
with the ordinary JSON error envelope carrying the id. The panic text stays on
the server; the caller gets the id, which is what joins the two.

## Thresholds: the `ops` block

These live in `ddcore.json`, next to `auth`, because they are a decision the
site makes once and keeps everywhere — staging has to call a backlog a backlog
exactly like production, or the rehearsal is not a rehearsal.

```json
{
  "ops": {
    "windowMinutes": 15,
    "queueBacklog": 100,
    "queueAgeSeconds": 300,
    "jobFailures": 5,
    "errorLogEntries": 20,
    "readyTimeoutMs": 2000,
    "slowRequestMs": 2000
  }
}
```

Every field may be omitted; what is left out keeps the value above. A field
written as zero or negative is refused at boot rather than quietly replaced —
a zero threshold is not "no threshold", it is an alarm that fires on the first
job.

## `ddcore doctor`

```
ddcore doctor [--json] [--strict] [--window N]
```

It reports the version, the site, the database (with the DSN's password
redacted), the apps and doctypes, pending DDL, undeclared structures, pending
patches, applied renames, the queue, the Error Log, the scheduler, the workers,
mail, the public URL, the session policy, the thresholds in force, and the
**names** of the configured secrets — never their values, because this report
gets pasted into issues and chat windows.

It works with the database down. The probe runs before, and independently of,
the engine, so an unreachable database produces a report that says so plus every
section that needs no database — not a bare `error:` line. That is the situation
the command exists for.

Exit `1` means a **critical** finding: the database is unreachable, the apps
could not be loaded, a migration is refused, or an undeclared structure still
holds data. Warnings — pending DDL, a backlog, recent failures — exit `0`
unless `--strict`, because this is mostly run by a person reading it rather than
a script failing on it.

`--json` prints the same report as a JSON object, for a cron that wants to feed
it somewhere.

## What this does not do

There is no `/metrics` endpoint and no OpenTelemetry. The authenticated report
and `doctor --json` are both scrapeable today, and a metrics endpoint with no
scraper deployed is a product nobody uses.

Nothing records when the scheduler last ran, so `scheduler.entries` is what this
build would install and not proof that a cron is alive. Retrying, cancelling and
retaining jobs are not here either — this reports queue health and writes to no
job row.
