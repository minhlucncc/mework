# mework

An **AI agent runtime** that runs in two modes:

- **Server mode** — multi-tenant hub with Postgres + Redis, enrolled runners,
  webhook pipelines, and provider integrations (Mello, Mezon).
- **Offline mode** — single-user, zero external dependencies. A standalone
  worker with embedded miniredis, an orchestrator agent with MCP tools, and
  Mezon chat integration.

Agents run **on your machine inside sandboxes** — source code and credentials
never leave the device.

## Quick comparison

| | Server mode | Offline mode |
|---|---|---|
| **Database** | Postgres (required) | None (miniredis embedded) |
| **Message broker** | Postgres LISTEN/NOTIFY or in-memory | In-memory (miniredis lists) |
| **Multi-tenant** | Yes (accounts, tenants, PAT auth) | Single user |
| **Provider integrations** | Webhooks (Mello), Mezon worker | Mezon bot (standalone) |
| **State persistence** | Durable (Postgres) | Ephemeral (lost on restart) |
| **Setup** | `docker compose up` + enroll runner | `mework init` + start worker |
| **Best for** | Teams, production, multi-provider | Dev, testing, single user |

## Architecture

### Server mode

```
Provider ─webhook→  mework-server (Agent Hub)
                      ├── Postgres (jobs, sessions, runtimes)
                      ├── Redis (turbo engine state, dedup)
                      ├── Agent catalog + session manager
                      └── SSE push → Runner → Sandbox → result
```

### Offline mode

```
Mezon ──→ mework-mezon-worker (turbo engine)
             ├── miniredis (in-memory state)
             ├── Orchestrator agent (Claude with MCP tools)
             ├── .claude/skills/ (session-manager, communicator, planner)
             └── .claude/commands/ (/sessions, /spawn, /status, /stop)
```

The offline worker is a single binary that:
1. Connects to Mezon via WebSocket (turbo SDK, multi-bot)
2. Receives messages and pushes them to an **inbox queue** (Redis list)
3. An **orchestrator goroutine** pops the inbox, runs Claude with MCP tools
4. Results go to an **outbox queue**, then sent back to Mezon via the WebSocket

## Install

### Quick install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/minhlucncc/mework/main/install.sh | sh
```

Installs `mework`, `mework-server`, `mework-mezon-worker`, and `mework-mcp`
to `/usr/local/bin` (or `~/.local/bin`).

### From source

```bash
git clone https://github.com/minhlucncc/mework.git
cd mework && make build    # → bin/mework, bin/mework-server, bin/mework-mezon-worker, bin/mework-mcp
```

Requires **Go 1.26**. Server mode also needs **PostgreSQL** (Docker:
`make test-db`). Offline mode needs **only the binary** — no databases to install.

## Quick start: Offline with Mezon

The fastest way to try mework — one binary, zero infrastructure.

```bash
# 1. Install
curl -fsSL https://raw.githubusercontent.com/minhlucncc/mework/main/install.sh | sh
# Or build from source: git clone + make build

# 2. Create a workspace with the orchestrator template
mkdir ~/my-bot && cd ~/my-bot
mework init --agent orchestrator

# 3. Configure your Mezon bot credentials
#    Get app_id and api_key at https://mezon.ai/developers/dashboard
mework provider mezon set --app-id YOUR_APP_ID --api-key YOUR_API_KEY

# 4. Start the worker (miniredis embedded, no install needed)
mework mezon-worker start

# 5. Add the bot to a Mezon clan, then chat
#    @your-bot hello
#    @your-bot sessions
#    @your-bot spawn explore this repo
```

No Postgres, no Redis, no server. The worker has everything built-in.
You can also test from the CLI:

```bash
mework agent send orchestrator "explore the workspace"
mework agent send orchestrator "list sessions" --wait
```

## Quick start: Server mode

### Server mode (multi-tenant)

```bash
# 1. Start Postgres and Redis
docker run -d --name mework-pg -p 5432:5432 postgres:16-alpine
docker run -d --name mework-redis -p 6379:6379 redis:7-alpine

# 2. Start the server
DATABASE_URL=postgres://postgres:postgres@localhost:5432/mework \
  SERVER_KEY=your-key-min-16-chars \
  MEWORK_SECRET_KEY=your-key-min-16-chars \
  REDIS_URL=redis://localhost:6379/0 \
  bin/mework-server

# 3. Enroll a runner
mework runner enroll --url http://localhost:8080 --token <registration-token>
mework daemon start

# 4. Create an agent profile and start a sandbox
mework profile create --name default --backend claude
mework sandbox start -w .
```

### Offline mode (single user, zero deps)

```bash
# 1. Initialize a workspace
mework init --workspace . --agent orchestrator

# 2. Start the Mezon bot worker with miniredis
MEZON_CONFIG=./bots.json \
  bin/mework-mezon-worker

# 3. Chat with the orchestrator via Mezon (@your-bot)
#    or via CLI
mework agent send orchestrator "hello"
```

The offline worker auto-initializes the orchestrator workspace with:
- `CLAUDE.md` — orchestrator persona, commands, session management
- `.claude/settings.json` — MCP server config (mework-mcp tools)
- `.claude/skills/` — session-manager, communicator, planner
- `.claude/commands/` — `/sessions`, `/spawn`, `/status`, `/stop`

## CLI Commands

```                   
Core:     workspace, board, ticket, comment, search
Runtime:  daemon, agent, profile, runner, runtime, sandbox, session, server
Worker:   mezon-worker (start/stop/status/logs)
Setup:    init, login, provider, config, auth
```

| Command | Purpose |
|---------|---------|
| `mework init --agent orchestrator` | Scaffold a workspace with CLAUDE.md + MCP + skills |
| `mework daemon start` | Start the local agent daemon (server mode) |
| `mework mezon-worker start` | Start the standalone Mezon bot worker |
| `mework agent send <name> <msg>` | Send a message to a local or hub agent |
| `mework provider mezon bot register` | Register a Mezon bot on the server |
| `mework session create/attach/send` | Interactive session lifecycle |
| `mework sandbox start -w .` | Turn a workspace into a runnable sandbox |

Full reference: **[docs/cli-and-usage.md](docs/cli-and-usage.md)**.

## HTTP API

| Auth | Routes |
|------|--------|
| **Open** | `GET /healthz` · `GET /livez` · `GET /readyz` · `POST /webhooks/{provider}` |
| **Runtime (`rt_`)** | `/api/v1/jobs/*` · `/api/v1/runners/sessions/*` · `/api/v1/agents/*/pull` |
| **PAT** | `/api/v1/{runtimes,connections,profiles,agents,sessions,channels,mezon/bots}` |

Details: **[docs/api-reference.md](docs/api-reference.md)**.

## Orchestrator agent

The orchestrator is a Claude Code agent with:

- **MCP tools**: `spawn_sandbox`, `list_child_sandboxes`, `get_sandbox_status`,
  `destroy_sandbox`, `notify_human`, `ask_human`, `get_session_context`, `write_artifact`
- **Skills**: session management, task planning, Mezon communication
- **Commands**: `/sessions`, `/spawn <task>`, `/status <id>`, `/stop <id>`

The orchestrator manages work by spawning child sandboxes — each sandbox is an
independent Claude Code agent working on a specific task. The user chats with the
orchestrator to coordinate everything.

## Templates

```                   
templates/workspace/
├── orchestrator/     ← mework init --agent orchestrator
└── worker/           ← mework init --agent worker
```

Each template includes `mework.yml`, `CLAUDE.md`, `.claude/settings.json`,
`.claude/skills/`, and `.claude/commands/`.

## Spec-driven development

All non-trivial changes start as specs using OpenSpec:

```
/opsx:explore → /opsx:propose → /opsx:spec → /opsx:apply → /opsx:ship → /opsx:archive
```

Shipped changes live in `openspec/changes/archive/`. Details:
[docs/openspec-workflow.md](docs/openspec-workflow.md).

## Development

```bash
make build    # all binaries
make vet      # go vet
make test     # go test -p 1 (DB tests need TEST_DATABASE_URL)
make test-db  # Docker Postgres
```

## Docs

- [docs/product-overview.md](docs/product-overview.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/api-reference.md](docs/api-reference.md)
- [docs/cli-and-usage.md](docs/cli-and-usage.md)
- [docs/deployment-guide.md](docs/deployment-guide.md)
- [docs/auth-and-secrets.md](docs/auth-and-secrets.md)
- [docs/runtime-and-sandbox.md](docs/runtime-and-sandbox.md)
- [docs/openspec-workflow.md](docs/openspec-workflow.md)
- [docs/engineering-skills.md](docs/engineering-skills.md)
- [examples/remote-claude/](examples/remote-claude/)
