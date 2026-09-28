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
| `PORT` / `DDCORE_PORT` | HTTP port (default `8080`); `DDCORE_PORT` wins over `PORT` | `8080` |
| `DDCORE_URL` | The public address recovery and invitation links are built from | `https://erp.example.com` |
| `DDCORE_TRUST_PROXY` | Believe `X-Forwarded-For` — only behind a proxy you control | `true` |
| `DDCORE_ADMIN_PASSWORD` | Admin's first password, set by the first migration instead of a generated one; never replaces a password Admin has | a strong password |
| `DDCORE_AUTO_MIGRATE` | `ddcore start` migrates before it serves; `0` turns that off for a deployment that migrates in a step of its own | `0` |
| `DDCORE_DATA_DIR` | Uploads and local backups (the official image sets `/data`) | `/data` |
| `DDCORE_SECRET_KEY` | Master key for encrypted Vault fields and webhook signing | `openssl rand -hex 32` |
| `DDCORE_STORAGE` | Where uploaded file bytes live: `local` (`<dataDir>/files`, the default) or `s3`, with the `DDCORE_S3_*` variables. See [storage](../agent/storage.md) | `s3` |
| `DDCORE_SECRET_<NAME>` | A secret an app reads with `ddcore.secret("<name>")` | |
| `DDCORE_LOGIN_NOTICE` | Plain-text notice above the sign-in form (`\n` breaks the line) | `Public demo — data resets every 6 hours.` |
| `DDCORE_LOGIN_DEMO_USER` / `DDCORE_LOGIN_DEMO_PASSWORD` | A demo account offered on the sign-in screen, with a button that fills the form. Public to every visitor: never a real password | `visitor@example.com` / `demo-visitor` |

`.env.example`, written by `ddcore init`, lists every variable. Development mode is never set
here: it is `ddcore dev`, or `DDCORE_DEV=1` for another command on a developer's machine.

---

## 3. Option A: Docker (and any platform that runs an image)

ddcore publishes an official image for every release, `ghcr.io/jrvidotti/ddcore:<version>`
(`linux/amd64` and `linux/arm64`): the binary on Alpine, with `pg_dump`/`pg_restore` 17 for
backups and the timezone data. `ddcore init` writes a site's `Dockerfile` on it, pinned to the
release that created the site; `ddcore deploy docker` writes it into an existing site:

```dockerfile
FROM ghcr.io/jrvidotti/ddcore:0.21.0

COPY ddcore.json ./
COPY apps/ ./apps/
```

The image does the rest: `WORKDIR /app`, uploads in `/data`, `$PORT` honoured, and
`CMD ["ddcore", "start"]`, which applies pending migrations and patches before it serves the
desk, the API, the job workers and the scheduler from one process.

**The tag is the pin.** It is the ddcore release the site runs: keep it inside the `ddcore`
range of `ddcore.json` and change both in the same commit. `:0.21` follows the minor's patches
and `:latest` the newest release — neither belongs in production.

**Persistent data.** The container's filesystem does not survive a deploy: mount a volume at
`/data`, or set `DDCORE_STORAGE=s3` so uploads live in a bucket.

**Several replicas** are safe against one database: each one's boot migration waits for the
others' (an advisory lock), and every scheduler fires but a cron entry is enqueued once per
minute however many replicas there are. Local storage on a volume ties the site to one replica;
use S3 to run more.

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
    volumes:
      - files:/data

volumes:
  pgdata:
  files:
```

---

## 4. Option B: Railway

`ddcore deploy railway` writes the `Dockerfile` and `.dockerignore` (when missing) and a
`railway.json` that builds the Dockerfile, gates each deploy on `/api/ready` and restarts on
failure, then lists the variables to set. On Railway:

1. Create a project from the site's repository, and add a **PostgreSQL** service.
2. On the site's service, set the variables:
   - `DATABASE_URL=${{Postgres.DATABASE_URL}}`
   - `DDCORE_URL=https://<the service's domain>` and `DDCORE_TRUST_PROXY=true`
   - `DDCORE_ADMIN_PASSWORD` (Admin's first password) and `DDCORE_SECRET_KEY`
   - each `DDCORE_SECRET_*` the apps read, and SMTP (`DDCORE_MAIL_*`, `DDCORE_SMTP_*`) if the
     site sends mail
3. Attach a **volume** at `/data`, or set `DDCORE_STORAGE=s3` and `DDCORE_S3_*`.
4. Generate a domain. Every push deploys; a failed migration fails the health check and the
   previous deployment keeps serving.

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
