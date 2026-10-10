# Production Deployment Guide

This guide covers deploying and operating `ddcore` applications in production environments.

---

## 1. System Requirements & Architecture

A production `ddcore` deployment consists of three primary components:

```
[ Clients / Browsers ]
          │ (HTTPS :443)
          ▼
┌─────────────────────────┐
│ Reverse Proxy (TLS)     │  Nginx, Caddy, or Traefik
└─────────┬───────────────┘
          │ (HTTP :8080)
          ▼
┌─────────────────────────┐
│ ddcore Service          │  Single compiled Go binary running your app
└─────────┬───────────────┘
          │ (PostgreSQL wire protocol)
          ▼
┌─────────────────────────┐
│ PostgreSQL 14+ Database │  Managed service or self-hosted cluster
└─────────────────────────┘
```

### Minimum Specifications:
- **Operating System:** Linux (Ubuntu 22.04+, Debian 12+, RHEL 9+, Alpine Linux) on `x86_64` (amd64) or `aarch64` (arm64).
- **RAM:** 512 MB minimum (1 GB+ recommended).
- **Disk:** Sufficient storage for PostgreSQL data and, unless `DDCORE_STORAGE=s3` keeps them in a bucket, uploaded user files.
- **Dependencies:** PostgreSQL 14 or higher. No Node.js runtime or Python environment is required on the production host.

---

## 2. Environment Configuration

`ddcore.json` holds what the site decided (apps, language, currency, the `ddcore` range) and is
committed. Everything that varies per deployment, and every secret, comes from the environment,
which overrides the file:

| Environment Variable | Description | Example |
| :--- | :--- | :--- |
| `DATABASE_URL` / `DDCORE_DSN` | PostgreSQL connection string. `DATABASE_URL` is what platforms such as Railway inject; `DDCORE_DSN` wins over it | `postgres://user:pass@db:5432/app?sslmode=require` |
| `DDCORE_POOL_MAX_CONNS` | Size of the database pool requests run on (`poolMaxConns` in `ddcore.json`). Unset, the DSN's `pool_max_conns` or pgx's default (the larger of 4 and the number of CPUs) applies. A request holds its connection while it waits on a slow outbound call, so a site that makes them may need more | `20` |
| `DDCORE_SHUTDOWN_GRACE_SECONDS` | How long a process told to stop lets its running jobs finish before it interrupts them and puts them back in the queue (`shutdownGraceSeconds` in `ddcore.json`, default `30`). The platform's stop timeout must be longer — see "Stopping" below | `60` |
| `PORT` / `DDCORE_PORT` | HTTP port (default `8080`); `DDCORE_PORT` wins over `PORT` | `8080` |
| `DDCORE_URL` | The public address recovery and invitation links are built from | `https://erp.example.com` |
| `DDCORE_TRUST_PROXY` | Believe `X-Forwarded-For` — only behind a proxy you control | `true` |
| `DDCORE_CORS_ORIGINS` | The other origins whose pages may call the methods whitelisted with `cors: true`, comma-separated; replaces `cors.origins` in `ddcore.json`. `scheme://host[:port]`, `scheme://*.domain` for its subdomains, or `*`. Never with credentials | `https://shop.example.com,https://*.partner.example` |
| `DDCORE_ADMIN_PASSWORD` | Admin's first password, set by the first migration instead of a generated one; never replaces a password Admin has | a strong password |
| `DDCORE_AUTO_MIGRATE` | `ddcore start` migrates before it serves; `0` turns that off for a deployment that migrates in a step of its own | `0` |
| `DDCORE_DATA_DIR` | Uploads and local backups (the official image sets `/data`) | `/data` |
| `DDCORE_SECRET_KEY` | Master key for encrypted Vault fields and webhook signing | `openssl rand -hex 32` |
| `DDCORE_STORAGE` | Where uploaded file bytes live: `local` (`<dataDir>/files`, the default) or `s3`, with the `DDCORE_S3_*` variables. See [storage](../agent/storage.md) | `s3` |
| `DDCORE_SECRET_<NAME>` | A secret an app reads with `ddcore.secret("<name>")` | |
| `DDCORE_APP_<NAME>` | A setting an app reads with `ddcore.env("<name>")` that is not a secret — `ddcore doctor` prints its value | |
| `DDCORE_LOGIN_NOTICE` | Plain-text notice above the sign-in form (`\n` breaks the line) | `Public demo — data resets every 6 hours.` |
| `DDCORE_LOGIN_DEMO_USER` / `DDCORE_LOGIN_DEMO_PASSWORD` | A demo account offered on the sign-in screen, with a button that fills the form. Public to every visitor: never a real password | `visitor@example.com` / `demo-visitor` |
| `DDCORE_MAP_TILE_URL` | Where a Geolocation field's map draws its tiles from, a Leaflet URL template with `{z}`, `{x}`, `{y}`. The default is OpenStreetMap's own server, whose [tile usage policy](https://operations.osmfoundation.org/policies/tiles/) rules out heavy use: set a provider of your own in production | `https://tiles.example.com/{z}/{x}/{y}.png?key=…` |
| `DDCORE_MAP_ATTRIBUTION` | The credit that provider requires, shown on the map (may hold a link; sanitized). Defaults to "© OpenStreetMap contributors" only while the URL is the default | `© Example Maps` |

`.env.example`, written by `ddcore init`, lists every variable. Development mode is never set
here: it is `ddcore dev`, or `DDCORE_DEV=1` for another command on a developer's machine.

---

## 3. Option A: Docker (and any platform that runs an image)

ddcore publishes an official image for every release, `ghcr.io/jrvidotti/ddcore:<version>`
(`linux/amd64` and `linux/arm64`): the binary on Alpine, with `pg_dump`/`pg_restore` 17 for
backups and the timezone data. `ddcore init` writes a site's `Dockerfile` on it, on the minor
series of the release that created the site; `ddcore deploy docker` writes it into an existing
site:

```dockerfile
FROM ghcr.io/jrvidotti/ddcore:0.21

COPY ddcore.json ./
COPY apps/ ./apps/
```

The image does the rest: `WORKDIR /app`, uploads in `/data`, `$PORT` honoured, and
`CMD ["ddcore", "start"]`, which applies pending migrations and patches before it serves the
desk, the API, the job workers and the scheduler from one process.

**The tag is the pin.** Each release publishes `:0.21.3`, moves `:0.21` to it, and moves
`:latest`. The generated `:0.21` follows its minor series, so every deploy takes the newest
patch — fixes and backwards-compatible additions — and never the next minor, which may break
while ddcore is `0.x`. Keep the tag inside the `ddcore` range of `ddcore.json` and move both in
the same commit when you take a new minor. Pin an exact release (`:0.21.3`) when a redeploy or
rollback of an old commit must run the very binary it ran then. `:latest` does not belong in
production.

**Persistent data.** The container's filesystem does not survive a deploy: mount a volume at
`/data`, or set `DDCORE_STORAGE=s3` so uploads live in a bucket.

**Several replicas** are safe against one database: each one's boot migration waits for the
others' (an advisory lock), and every scheduler fires but a cron entry is enqueued once per
minute however many replicas there are. Local storage on a volume ties the site to one replica;
use S3 to run more.

**Slow jobs.** A queue of long jobs can be given workers of its own, so it never holds up mail
and everything else: `"workers": { "default": 2, "bot": 2 }` in `ddcore.json`, and
`ddcore.enqueue(..., { queue: "bot" })`. A separate container running
`ddcore jobs work --queue bot` serves only that queue. See `ops` → "Worker pools".

**Stopping.** On SIGTERM the process stops claiming jobs, finishes the HTTP requests in flight,
and gives the jobs it is running `shutdownGraceSeconds` (30 by default) to finish; the ones still
running then go back to the queue with their attempt given back, and the process exits. Give the
container longer than that before it is killed — Docker's default is 10 seconds:
`stop_grace_period: 45s` in Compose, `docker stop -t 45`, or `terminationGracePeriodSeconds: 45`
on Kubernetes. A process killed first leaves its jobs `running` until their lease expires two
minutes later, when they are failed like the jobs of a worker that crashed.

### Example `docker-compose.yml`

The `docker-compose.yml` that `ddcore init` writes runs only a development database, with the
password in the file. For production, use a file like this one, with secrets taken from the
environment:

```yaml
services:
  db:
    image: postgres:17-alpine
    restart: always
    environment:
      POSTGRES_USER: ddcore
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: ddcore_prod
    volumes:
      - pgdata:/var/lib/postgresql/data

  app:
    build: .
    restart: always
    depends_on:
      - db
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      DATABASE_URL: postgres://ddcore:${DB_PASSWORD}@db:5432/ddcore_prod?sslmode=disable
      DDCORE_URL: https://erp.example.com
      DDCORE_TRUST_PROXY: "true"
      DDCORE_ADMIN_PASSWORD: ${ADMIN_PASSWORD}
      DDCORE_SECRET_KEY: ${SECRET_KEY}
    # longer than shutdownGraceSeconds (30), so running jobs can finish or be put back
    stop_grace_period: 45s
    volumes:
      - files:/data

volumes:
  pgdata:
  files:
```

---

## 4. Option B: Railway

The Railway project is described as code, in `.railway/railway.ts` — Railway's
Infrastructure as Code, applied with `railway config plan` and `railway config apply`.
(`railway.json`, the older Config as Code, is read only until 2026-12-01; `ddcore` no longer
writes it.) `ddcore deploy railway` writes the `Dockerfile` and `.dockerignore` when missing,
and `.railway/railway.ts` with its `package.json`:

```ts
import { defineRailway, github, postgres, preserve, project, service } from "railway/iac";

export default defineRailway(() => {
  const db = postgres("Postgres");
  const site = service("erp", {
    source: github("acme/erp", { branch: "main" }), // from the checkout's origin
    healthcheck: "/api/ready",
    healthcheckTimeout: 300,
    env: {
      DATABASE_URL: db.env.DATABASE_URL,
      DDCORE_TRUST_PROXY: "true",
      DDCORE_URL: preserve(),
      DDCORE_ADMIN_PASSWORD: preserve(),
      DDCORE_SECRET_KEY: preserve(),
    },
  });
  return project("erp", { resources: [db, site] });
});
```

Secrets never enter the file: `preserve()` keeps the value set in Railway. Then:

1. `npm install --prefix .railway` — the SDK the file imports.
2. `railway link` to the project (`railway init` creates one).
3. In Railway's Variables, set `DDCORE_URL`, `DDCORE_ADMIN_PASSWORD`, `DDCORE_SECRET_KEY`, each
   `DDCORE_SECRET_*` the apps read (declare them in the file as `preserve()` too) and SMTP if the
   site sends mail.
4. Uploads: a volume at `/data` (`volumeMounts` in the file), which keeps the site at one
   replica, or `DDCORE_STORAGE=s3` with `DDCORE_S3_*`, which lets it run several.
5. `railway config plan`, review it, `railway config apply`. Every push to the branch deploys;
   a failed migration fails the health check and the previous deployment keeps serving.

Declare every property the service already has — its `source` above all: a plan against a
file without one would disconnect the repository. `railway config pull --json` shows the
project as Railway has it. A site still on `railway.json` runs
`railway config migrate --apply --delete-files` first, then merges the output into the file.

Railway's log viewer reads the JSON log the server writes when stdout is not a terminal. For
backups, add a cron service on the same image running `ddcore backup --to s3 --keep 14`.

---

## 5. Option C: Systemd Service

A systemd service unit provides automated restarts, logging via `journald`, and process isolation.

### Step 1: Create Application Directory & User
```bash
sudo useradd -r -s /bin/false -d /opt/ddcore ddcore
sudo mkdir -p /opt/ddcore/app
```

### Step 2: Install Binary and Application Code
Install the release the site pins, and copy the site (`ddcore.json` and `apps/`) into
`/opt/ddcore/app`:

```bash
curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh \
  | sudo env VERSION=0.21.0 BIN_DIR=/usr/local/bin sh
sudo chown -R ddcore:ddcore /opt/ddcore
```

`install.sh` verifies the archive against the release's `SHA256SUMS`.

### Step 3: Create Systemd Unit (`/etc/systemd/system/ddcore.service`)
```ini
[Unit]
Description=ddcore Application Service
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=ddcore
Group=ddcore
WorkingDirectory=/opt/ddcore/app
ExecStart=/usr/local/bin/ddcore start
Restart=always
RestartSec=5s

# Security sandboxing
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true

EnvironmentFile=/opt/ddcore/.env

[Install]
WantedBy=multi-user.target
```

### Step 4: Start the Service
`ddcore start` migrates before it serves. To migrate in a step of your own instead, set
`DDCORE_AUTO_MIGRATE=0` and run `ddcore migrate` first.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ddcore
sudo systemctl status ddcore
```

---

## 6. Reverse Proxy Configuration (TLS & HTTPS)

`ddcore` should be run behind a TLS-terminating reverse proxy.

### Nginx Configuration:
```nginx
server {
    listen 443 ssl http2;
    server_name app.example.com;

    ssl_certificate /etc/letsencrypt/live/app.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/app.example.com/privkey.pem;

    client_max_body_size 50M;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 7. Health Checks & Observability

`ddcore` exposes diagnostic and monitoring capabilities:

### Health Check Endpoint
- `GET /api/health` (also `/healthz`): liveness — 200 while the process answers, whatever the
  database's state. Use it to decide whether to restart the process.
- `GET /api/ready` (also `/readyz`): readiness — 200 when the database answers, 503 when it
  does not. Use it for a platform's deploy health check and a load balancer.

### Diagnostic Command (`doctor`)
Run `ddcore doctor` to verify system health, database connectivity, and pending migrations:

```bash
ddcore doctor
```

For more details on logging, queue telemetry, and request correlation IDs, see the [Ops & Observability Reference](/agent/ops).

---

## 8. Backup & Disaster Recovery

`ddcore backup` writes one checksummed archive holding the database (`pg_dump`), every
stored file (local or S3), the configuration and the core/app versions. Secrets are
never included: provision `DDCORE_SECRET_KEY`, `DDCORE_SECRET_*`, SMTP and S3
credentials on the restore target separately.

```bash
# nightly, from cron or a systemd timer; --to s3 copies it off the machine
ddcore backup --to s3 --keep 14

# a controlled cutover: pause writes and jobs for a consistent archive
ddcore backup --maintenance --to s3
```

Restore into an isolated, empty database and data directory, verify, and time it:

```bash
DDCORE_DSN=postgres://ddcore:…@localhost/ddcore_drill DDCORE_DATA_DIR=/srv/drill \
  ddcore restore s3:ddcore-20260917-020000.tar --smoke
```

The restored site stays in maintenance mode until `ddcore maintenance off`. The
host or image needs `pg_dump`/`pg_restore` at least as new as the server. See the
[backup, restore and maintenance reference](/agent/backup) for the archive format,
rollback with `--allow-older-binary`, and the cutover runbook.
