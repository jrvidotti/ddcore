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
          │ (HTTP :8090)
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
- **Disk:** Sufficient storage for PostgreSQL data and uploaded user files.
- **Dependencies:** PostgreSQL 14 or higher. No Node.js runtime or Python environment is required on the production host.

---

## 2. Environment Configuration

`ddcore` reads configuration from `ddcore.json` and overrides parameters using environment variables:

| Environment Variable | Description | Example |
| :--- | :--- | :--- |
| `DDCORE_DSN` | PostgreSQL connection string | `postgres://user:pass@db:5432/app_prod?sslmode=require` |
| `DDCORE_PORT` | HTTP port for the server to listen on (default `8090`) | `8090` |
| `DDCORE_SECRET_KEY` | 32-byte secret key used for session cookies and vault encryption | `openssl rand -hex 32` |
| `DDCORE_ENV` | Environment identifier (`production`, `staging`, `development`) | `production` |

---

## 3. Option A: Systemd Service Deployment

A systemd service unit provides automated restarts, logging via `journald`, and process isolation.

### Step 1: Create Application Directory & User
```bash
sudo useradd -r -s /bin/false -d /opt/ddcore ddcore
sudo mkdir -p /opt/ddcore/app
```

### Step 2: Install Binary and Application Code
Place the `ddcore` binary in `/usr/local/bin/ddcore` and copy your application files (containing `ddcore.json` and your app modules) into `/opt/ddcore/app`.

```bash
sudo chown -R ddcore:ddcore /opt/ddcore
```

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
ExecStart=/usr/local/bin/ddcore run
Restart=always
RestartSec=5s

# Security sandboxing
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true

# Environment variables
Environment="DDCORE_ENV=production"
Environment="DDCORE_PORT=8090"
EnvironmentFile=/opt/ddcore/.env

[Install]
WantedBy=multi-user.target
```

### Step 4: Run Migrations and Start Service
```bash
# Apply any pending database migrations before starting the service
sudo -u ddcore -E /usr/local/bin/ddcore migrate

sudo systemctl daemon-reload
sudo systemctl enable --now ddcore
sudo systemctl status ddcore
```

---

## 4. Option B: Docker Deployment

You can containerize your application using Docker:

### Example `Dockerfile`:
```dockerfile
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

# Install latest ddcore binary
RUN curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh

WORKDIR /app
COPY ddcore.json ./
COPY apps/ ./apps/

EXPOSE 8090
CMD ["ddcore", "run"]
```

### Example `docker-compose.yml`:
```yaml
version: '3.8'

services:
  db:
    image: postgres:16-alpine
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
      - "127.0.0.1:8090:8090"
    environment:
      DDCORE_ENV: production
      DDCORE_DSN: postgres://ddcore:${DB_PASSWORD}@db:5432/ddcore_prod?sslmode=disable
      DDCORE_SECRET_KEY: ${SECRET_KEY}

volumes:
  pgdata:
```

---

## 5. Reverse Proxy Configuration (TLS & HTTPS)

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
        proxy_pass http://127.0.0.1:8090;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 6. Health Checks & Observability

`ddcore` exposes diagnostic and monitoring capabilities:

### Health Check Endpoint
- `GET /api/v1/health`: Returns HTTP 200 `{"status": "ok"}` when the application and database connection pool are healthy.
- Use this endpoint for Kubernetes readiness/liveness probes or load balancer health checks.

### Diagnostic Command (`doctor`)
Run `ddcore doctor` to verify system health, database connectivity, and pending migrations:

```bash
ddcore doctor
```

For more details on logging, queue telemetry, and request correlation IDs, see the [Ops & Observability Reference](/agent/ops).

---

## 7. Backup & Disaster Recovery

Because all application state and metadata reside in PostgreSQL:

### Database Backup
```bash
pg_dump -Fc -U ddcore -h localhost ddcore_prod > ddcore_backup_$(date +%Y%m%d_%H%M%S).dump
```

### Database Restore
```bash
pg_restore -c -U ddcore -h localhost -d ddcore_prod ddcore_backup_20260916_120000.dump
```

Store backups in off-site encrypted object storage (e.g. AWS S3 or Cloudflare R2) and test restoration periodically.
