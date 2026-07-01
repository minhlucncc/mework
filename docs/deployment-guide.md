# Deployment Guide

> Audience: operators deploying **`mework-server`** (PostgreSQL) in production,
> and users running the **offline/standalone worker** for local development.

## Server mode (production)

Requires PostgreSQL 13+ and optionally Redis for the Mezon turbo engine.

### Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | **yes** | — | PostgreSQL DSN |
| `SERVER_KEY` | **yes** | — | HMAC key for rt_token (≥16 chars) |
| `MEWORK_SECRET_KEY` | **yes** | — | AES-256-GCM key for credential sealing (≥16 chars) |
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address |
| `CHANNEL_ROUTING_ENABLED` | no | `false` | Experimental channel routing |
| `REDIS_URL` | no | — | Redis for turbo engine (Mezon multi-bot) |
| `MELLO_BASE_URL` | no | — | Mello API base URL |

### Database setup

Migrations run automatically on startup. Create an empty database:

```bash
docker run --name mework-pg -e POSTGRES_PASSWORD=pass -e POSTGRES_DB=mework -p 5432:5432 -d postgres:16-alpine
```

### Build

```bash
make build          # all binaries
make build-server   # server only
```

### Docker Compose

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: strongpassword123
      POSTGRES_DB: mework
    ports: ["5432:5432"]
    volumes: [mework_db_data:/var/lib/postgresql/data]

  server:
    build: .
    environment:
      DATABASE_URL: postgres://postgres:strongpassword123@postgres:5432/mework?sslmode=disable
      SERVER_KEY: your-hmac-key-min-16-chars
      MEWORK_SECRET_KEY: your-aes-key-min-16-chars
    ports: ["8080:8080"]
    depends_on: [postgres]

volumes:
  mework_db_data:
```

### Health checks

| Path | Behavior |
|------|----------|
| `GET /livez` | Always 200 (liveness, no DB check) |
| `GET /readyz` | DB ping → 200 or 503 |
| `GET /healthz` | Same as /readyz |

---

## Offline mode (single user, zero deps)

The offline mode has two flavors:

- **Offline stack (recommended)** — `mework daemon start --offline --with-mezon`.
  The daemon supervises an embedded `mework-server` (on SQLite) and a
  `mework-mezon-worker` end to end. Requires no PostgreSQL, no Redis, no
  separate server process. See [SQLite path (offline only)](#sqlite-path-offline-only)
  below and the orchestrator diagram in [runtime-and-sandbox.md](runtime-and-sandbox.md#offline-stack-orchestrator).
- **Standalone worker** — `mework-mezon-worker` invoked directly. Requires
  no PostgreSQL or Redis, but **does require a remote `mework-server`**
  (Postgres-backed). State is in-memory (embedded miniredis) and ephemeral.

The remainder of this section describes the standalone-worker flavor; for
the offline stack, see the SQLite path subsection below.

### Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `MEZON_APP_ID` | **yes** | — | Mezon application ID |
| `MEZON_API_KEY` | **yes** | — | Mezon API key |
| `MEZON_CONFIG` | no | — | JSON file with multiple bot configs |
| `REDIS_URL` | no | — | When set, uses real Redis (persistent). When empty, uses embedded miniredis (ephemeral, no install) |

When `REDIS_URL` is empty, the worker logs:
```
WARNING: REDIS_URL not set — using embedded in-memory Redis (state lost on restart)
For production, set REDIS_URL=redis://... for persistent state
```

### Setup

```bash
# 1. Build
make build

# 2. Initialize workspace
mework init --workspace . --agent orchestrator

# 3. Configure Mezon bot (json file or env vars)
cat > bots.json << EOF
{"bots":[{"app_id":"your_app_id","api_key":"your_api_key","plan":"pro"}]}
EOF

# 4. Start worker (no server needed)
MEZON_CONFIG=./bots.json bin/mework-mezon-worker
```

The worker auto-initializes the orchestrator workspace with CLAUDE.md,
MCP config, skills, and commands.

### Workspace scaffolding

```bash
mework init --workspace . --agent orchestrator   # orchestrator agent
mework init --workspace . --agent worker          # worker agent
```

Creates `mework.yml`, `CLAUDE.md`, `.claude/settings.json`, `.claude/skills/`,
and `.claude/commands/`.

---

## Backup & recovery

PostgreSQL backup for server mode:

```bash
pg_dump -U postgres -h localhost -d mework > mework_backup_$(date +%Y%m%d).sql
```

The AES-256-GCM-sealed credentials in the dump can only be unsealed with the
same `MEWORK_SECRET_KEY`. Back it up separately from the database.

Offline mode has no durable state — miniredis is in-memory only.

---

## SQLite path (offline only)

The offline-stack orchestrator (`mework daemon start --offline --with-mezon`,
see [runtime-and-sandbox.md](runtime-and-sandbox.md#offline-stack-orchestrator))
spawns an embedded `mework-server` with `DATABASE_URL=sqlite://…`. The
storage driver lives in `libs/server/platform/store/sqlite/` and is
dispatched by the existing `store.NewStore(ctx, dsn)` factory when the URL
scheme is `sqlite://`, `:memory:`, or `file:…`.

> **SQLite is offline-only — production deployments still require Postgres.**
> SQLite has no replication, no built-in HA, and a single-writer model. The
> Postgres path is untouched by this change and remains the supported
> production database.

### Auto-minted secrets

When the offline stack boots, the orchestrator auto-mints the two secrets
the server normally requires:

| Env var | Source | Notes |
|---|---|---|
| `SERVER_KEY` | 32-byte random hex (per boot) | HMAC key for `rt_token` lookup hashing |
| `MEWORK_SECRET_KEY` | 32-byte random hex (per boot) | AES-256-GCM key for credential sealing |

Both are written to `~/.mework/runtime/keys.json` (mode `0600`) **before**
the server subprocess is spawned, then injected as env vars into the child.
The keys are regenerated on every fresh stack boot — they only need to
survive the lifetime of one stack run, since the server is not restarted
across boots within a single `daemon start` invocation.

For any deployment beyond the local workstation (e.g. shared server, CI
runner, containerized stack), supply your own `SERVER_KEY` and
`MEWORK_SECRET_KEY` via env. Auto-minted keys are intended for the
single-user workstation case only.

### `~/.mework/runtime/` layout

The offline orchestrator writes its child-process state under
`~/.mework/runtime/` (mode `0700` on the directory; `0600` on every file
inside, per the auth-and-secrets invariants):

```
~/.mework/runtime/
├── keys.json              # auto-minted SERVER_KEY + MEWORK_SECRET_KEY (0600)
├── runner.token           # rt_token from runner enrollment (0600)
├── offline-pids.json      # {workspace, started, children:[{role,pid,port,log}]} (0600)
├── server.log             # mework-server stdout/stderr
└── worker.log             # mework-mezon-worker stdout/stderr
```

`mework daemon stop` reads `offline-pids.json` and signals the children in
reverse spawn order (worker first, then server), waits 5s for graceful
exit, and escalates to `SIGKILL` if needed. The pidfile is removed on
clean shutdown; a stale pidfile after a crash is detected by liveness
signal-0 in the same way `daemon.pid` is, so a fresh `daemon start` does
not race with a dead prior stack.

### Backup (SQLite)

The SQLite database lives at `<workspace>/.mework/data.db` (with WAL
sidecars `-wal` and `-shm`). To back it up while the stack is running,
use the SQLite backup API (e.g. `sqlite3 .mework/data.db ".backup
<dest>"`); simply copying the `.db` file is **not** safe while a writer
is active. The auto-minted `keys.json` should be backed up alongside
the database dump — sealed credentials in the database can only be
unsealed with the same `MEWORK_SECRET_KEY`.
