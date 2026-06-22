# Deployment Guide — MeWork Server

> Audience: operators deploying and running **`mework-server`** (Go HTTP backend +
> PostgreSQL) in production. For tokens and the full env reference, see
> [auth-and-secrets.md](auth-and-secrets.md).

## 1. System requirements

- **OS**: Linux (Ubuntu 22.04 LTS or newer recommended), macOS, or Windows.
- **Go**: `1.25.7` or newer (to build from source).
- **PostgreSQL**: `13` or newer.
- **Docker & Docker Compose** (optional — for container deployment).

## 2. Configuration (environment variables)

`mework-server` is configured entirely through environment variables and **fails fast
at startup** if a required one is missing.

| Variable | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `DATABASE_URL` | string | **yes** | — | PostgreSQL DSN (e.g. `postgres://user:pass@host:port/dbname?sslmode=disable`) |
| `SERVER_KEY` | string | **yes** | — | HMAC-SHA256 key for hashing/verifying `rt_token` lookups |
| `MEWORK_SECRET_KEY` | string | **yes** | — | AES-256-GCM key for sealing provider credentials at rest |
| `LISTEN_ADDR` | string | no | `:8080` | HTTP listen address |
| `WEBHOOK_SECRET` | string | no | — | Loaded but not enforced; per-connection webhook secrets in the DB are used instead |
| `MELLO_BASE_URL` | string | no | `https://mello.mezon.vn/api/v1` | Mello REST base URL |
| `CHANNEL_ROUTING_ENABLED` | bool | no | `false` | Opt-in to the experimental per-resource channel auto-provisioning path. Off by default — a default deployment uses the legacy webhook → job → claim → write-back pipeline. |

> Use long, random, independent values for `SERVER_KEY` and `MEWORK_SECRET_KEY`
> (each **at least 16 characters** — the server fails fast on shorter keys).
> Losing `MEWORK_SECRET_KEY` means stored provider credentials can no longer be
> unsealed (connections must be reconnected); rotating it requires re-sealing.

## 3. Database setup

The server runs goose migrations automatically on startup (auto-migration). You only
need to create an empty database first.

Quick Postgres via Docker (test/staging):
```bash
docker run --name mework-postgres \
  -e POSTGRES_PASSWORD=mysecretpassword -e POSTGRES_DB=mework \
  -p 5432:5432 -d postgres:16-alpine
```

Or on an existing PostgreSQL service:
```sql
CREATE DATABASE mework;
```

## 4. Build

```bash
make build         # builds both CLI (mework) and server (mework-server)
make build-server  # server only
```
The binary is produced at `bin/mework-server`.

## 5. Production deployment

### Option A — systemd service (recommended on a VPS)

1. Copy the binary into place:
   ```bash
   sudo cp bin/mework-server /usr/local/bin/
   ```

2. Create `/etc/systemd/system/mework-server.service`:
   ```ini
   [Unit]
   Description=MeWork Central Server
   After=network.target postgresql.service

   [Service]
   Type=simple
   User=nobody
   Group=nogroup
   Environment="DATABASE_URL=postgres://postgres:mysecretpassword@localhost:5432/mework?sslmode=disable"
   Environment="SERVER_KEY=replace-with-a-long-random-secret"
   Environment="MEWORK_SECRET_KEY=replace-with-a-different-long-random-secret"
   Environment="LISTEN_ADDR=:8080"
   Environment="WEBHOOK_SECRET=mello-webhook-secret"
   ExecStart=/usr/local/bin/mework-server
   Restart=always
   RestartSec=5
   LimitNOFILE=65535

   [Install]
   WantedBy=multi-user.target
   ```

3. Enable and start:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable mework-server
   sudo systemctl start mework-server
   ```

4. Check status and logs:
   ```bash
   sudo systemctl status mework-server
   journalctl -u mework-server.service -f
   ```

### Option B — Docker Compose

Create `docker-compose.yml`:
```yaml
version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    container_name: mework-db
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: strongpassword123
      POSTGRES_DB: mework
    ports:
      - "5432:5432"
    volumes:
      - mework_db_data:/var/lib/postgresql/data
    restart: always

  server:
    build:
      context: .
      dockerfile: Dockerfile   # if you provide one
    # or pull from your registry:
    # image: registry.yourdomain.com/mework-server:latest
    container_name: mework-server
    environment:
      - DATABASE_URL=postgres://postgres:strongpassword123@postgres:5432/mework?sslmode=disable
      - SERVER_KEY=a-long-random-hmac-key
      - MEWORK_SECRET_KEY=a-different-long-random-aes-key
      - LISTEN_ADDR=:8080
      - WEBHOOK_SECRET=mello-webhook-secret
    ports:
      - "8080:8080"
    depends_on:
      - postgres
    restart: always

volumes:
  mework_db_data:
```

Start it:
```bash
docker compose up -d
```

## 6. Health check

The server exposes `/healthz`. It returns `200 OK` when PostgreSQL is reachable, and
`503 Service Unavailable` otherwise.

```bash
curl -i http://localhost:8080/healthz
```

Successful response:
```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok"}
```

## 7. Backup & recovery

All durable state (accounts, runtimes, profiles, job history, sealed connections) lives
in PostgreSQL, so a periodic database dump is sufficient.

Backup:
```bash
pg_dump -U postgres -h localhost -d mework > mework_backup_$(date +%Y%m%d).sql
```

Restore:
```bash
psql -U postgres -h localhost -d mework < mework_backup_xxxx.sql
```

> The AES-256-GCM-sealed provider credentials in the dump can only be unsealed with the
> same `MEWORK_SECRET_KEY` the server runs with. Back up that key securely and
> separately from the database.
