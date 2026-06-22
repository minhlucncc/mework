# mework

An **agent hub** for running AI coding agents from your task board — built around the **DX of
a GitHub Actions self-hosted runner**: install a runner once, then drive everything from the
hub. Agents run **on your machine inside sandboxes**, so source code and credentials never
leave the device.

> **Status.** The agent-hub model described below is **implemented**: install-once runner
> enrollment, SSE-pushed dispatch, the versioned agent catalog, interactive sessions, and
> workspace-bound sandboxes all work today. The earlier Mello **poll/queue pipeline**
> (webhook → job → claim → write-back) also ships and is the **default** webhook path. A few
> components are still stubs — see [Implementation status](#implementation-status).

## Overview

mework connects task-management platforms (**Mello** kanban today; Jira / GitHub Issues by
design) to AI coding agents that run on **your** machines — not a cloud AI service. It has
**three components**:

1. **Server = Agent Hub** — a provider-agnostic gateway + registry: provider connections, an
   **agent catalog** of versioned pullable agents, a **message broker** that pushes work to
   per-runner/per-session **topics**, session metadata, and a **permission/grant** model that
   scopes what each dispatch may do. The hub **never runs an agent or sees your source**.
2. **Daemon = Runner** — enrolled **once** on a device, then unattended: it **subscribes over
   SSE**, receives dispatches, runs the agent in a sandbox, enforces the granted permissions,
   and reports results.
3. **Sandbox** — a **pluggable runtime** (`local` / `docker` / …) that runs **one agent** with
   the prompt fed over **stdin (never argv)**, so dispatched work executes without touching the
   host directly.

## Architecture

```
provider ─webhook→  Server (Agent Hub)            ── gateway + registry only ──
                      • provider connections + signature-verified webhooks
                      • agent catalog (versioned, pullable)
                      • message broker → topics (memory | postgres)
                      • session manager + permission/grant model
                      │  SSE push                   ▲ POST/GET (ack, result, pull)
                      ▼                             │
                    Daemon = Runner  (enrolled once, unattended)
                      • subscribe runner.<id>.dispatch over SSE
                      • on dispatch: pull agent / open sandbox, enforce grant
                      │ spawn (one agent per sandbox; prompt via stdin)
                      ▼
                    Sandbox (local | docker | …)
                      • isolated; runs ONE agent
                      • result/events ─→ hub ─→ provider write-back or session stream
```

**Two delivery flows coexist:**

- **Legacy Mello pipeline (default):** a `@mework …` comment webhook → durable Postgres job →
  the daemon claims it (`/api/v1/jobs/claim`) → runs the agent → the hub writes the result back
  to the provider over REST. Experimental per-resource **channel routing** is **off by default**
  (`CHANNEL_ROUTING_ENABLED`).
- **Interactive session / sandbox (agent-hub):** `mework sandbox start -w .` (or `session
  create`) registers a session → the hub dispatches an open-session message on
  `runner.<id>.dispatch` → the daemon opens a **long-lived sandbox** → chat turns flow over the
  bus (`session.<id>.input` in, `session.<id>.control` events out) and stream back to the CLI.

**Permissions.** Every dispatch carries a scoped, least-privilege **grant**. The hub
authorizes, the runner enforces, the sandbox contains. No grant for an operation → denied.

Full design, the SSE contract, the permission model, and bus topics:
**[docs/architecture.md](docs/architecture.md)**.

## Install

```bash
make build            # → bin/mework (CLI + daemon + `server start`) and bin/mework-server
# or
go install ./apps/...  # installs mework and mework-server
```

Requires **Go 1.26** and **PostgreSQL** for the server. Claude Code (`claude` in PATH) on the
runner machine to actually execute agents.

## Quick start

### A) Local, three commands

```bash
# env for the in-process hub (server fails fast without these; keys must be ≥16 chars)
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/mework
export SERVER_KEY=dev-server-key-0123456789 MEWORK_SECRET_KEY=dev-secret-key-0123456789

# 1. Run the hub (in-process; or run ./bin/mework-server, or docker compose)
mework server start --listen :8080

# 2. Log in, enroll this machine as a runner, start the daemon
mework login --token <mello-pat>
REG=$(curl -s -XPOST localhost:8080/api/v1/runners/registration-tokens \
        -H "Authorization: Bearer <mello-pat>" | jq -r .token)
mework runner enroll --url http://localhost:8080 --token "$REG"   # writes ~/.mework/identity.json
mework daemon start

# 3. Turn a workspace folder (with a mework.yml) into a running, chattable worker
SID=$(mework sandbox start -w . --json | jq -r .id)
mework session attach "$SID"               # stream events (one terminal)
mework session send  "$SID" "summarize this repo"   # send a turn (another terminal)
mework sandbox stop  "$SID"
```

### B) Mello kanban trigger

```bash
mework login --token <mello-pat>
mework provider connect --provider mello --token <mello-pat> --webhook-secret <secret>
mework runner enroll --url <hub> --token <registration-token>
mework profile create --name default --body ./system_prompt.txt --backend claude --harness claude-code
mework daemon start

# Then comment on any ticket:  @mework <profile> [workflow] <instructions>
#   e.g.  @mework default review fix the failing login test
# workflow ∈ plan|cook|test|review|ship|journal
```

## Commands

| Group | Commands |
|-------|----------|
| **Core** | `workspace list/pack/push/pull`, `board list/get`, `ticket list/get/create/move`, `comment list/add`, `search` |
| **Runner** | `server start [--listen]`, `runner enroll --url --token`, `daemon start/stop/status/restart/logs`, `runtime register/list/revoke` *(legacy)*, `agent list`, `profile create/list/update/delete` |
| **Sessions** | `session list/create/send/attach/close`, `sandbox start -w <dir> [--attach] [--json] [--idle] / list / stop / send` |
| **Additional** | `login`, `auth status/logout`, `config show/set`, `provider connect`, `version` |

`runner enroll` is the primary enrollment path; `runtime register` remains for backward
compatibility. Most list/get commands accept `--json`. Global flags: `--server-url`,
`--workspace-id`, `--profile`, `--debug`. Full reference: **[docs/cli-and-usage.md](docs/cli-and-usage.md)**.

## HTTP API (summary)

| Auth | Routes |
|------|--------|
| **Open** | `GET /healthz` · `GET /livez` · `GET /readyz` · `POST /webhooks/{provider}` |
| **Runtime (`rt_`)** | `/api/v1/jobs/{ack,claim,heartbeat,subscribe,messages/{id}/ack}` · `POST /api/v1/runners/sessions/{id}/{result,events}` · `GET /api/v1/agents/{name}/versions/{version}/pull` (grant) |
| **Registration token** | `POST /api/v1/runners/enroll` |
| **PAT** | `/api/v1/{runtimes,connections,profiles,agents,channels,runs/{id}/artifacts}` · `/api/v1/runners/registration-tokens` · `/api/v1/sessions` (+ `/{id}`, `/{id}/messages`, `/{id}/stream`) |

Details, topics, and data model: **[docs/api-reference.md](docs/api-reference.md)**.

## Implementation status

Production-ready: the Mello pipeline, runner enrollment, SSE dispatch, interactive
sessions/sandboxes, the `local` and `docker` sandbox engines, security invariants
(stdin-not-argv, AES-256-GCM credential sealing, HMAC `rt_token`, signature-verified webhooks),
and the memory/postgres message brokers.

Still stub / not production (tracked):

| Component | State |
|-----------|-------|
| Artifact store | dummy (`/runs/{id}/artifacts` returns "not yet wired") |
| NATS bus backend | not wired (memory/postgres are) |
| GitHub / Jira providers | stubs (Mello is real) |
| `mework-sandbox` binary | stub mode |
| cloudflare / custom engines | partial |
| channel routing | experimental, off by default |
| per-turn session streaming | coarse (one token/message/done per turn) |

## Spec-driven development with OpenSpec

This project uses **[OpenSpec](https://github.com/Fission-AI/OpenSpec)** — non-trivial changes
start as a spec/proposal, not code.

```
/opsx:explore   think through an idea (no code written)
/opsx:propose   create a change + artifacts (proposal → delta specs → design → tasks)
/opsx:spec      quality-gate the spec across 6 review axes
/opsx:apply     implement the tasks
/opsx:sync      merge delta specs into the main specs
/opsx:archive   finalize → openspec/changes/archive/
```

Canonical capabilities live as baseline specs in `openspec/specs/` (e.g. `provider-gateway`,
`webhook-pipeline`, `job-queue`, `rest-writeback`, `daemon-runtime`, `message-bus`,
`prebuilt-agent-sandbox`, `channel-routing`, `auth-and-secrets`, `cli`, `project-structure`).
Shipped changes are archived under `openspec/changes/archive/`. Change dirs are named
`cNNNN-<slug>` to encode apply order. Workflow details:
[docs/openspec-workflow.md](docs/openspec-workflow.md).

## Development

```bash
make build    # bin/mework and bin/mework-server (version ldflags)
make vet      # go vet across all modules
make test     # go test -p 1 across modules (DB tests skip without TEST_DATABASE_URL)
make test-db  # start a Docker Postgres for DB-backed tests
make snapshot # goreleaser cross-compile (requires goreleaser)
```

DB-backed tests skip unless `TEST_DATABASE_URL` is set (e.g.
`postgres://postgres:postgres@localhost:5432/mework_test`). The acceptance BDD suite under
`libs/tests/e2e` is gated behind the `e2e` build tag.

## Docs

Start at **[docs/README.md](docs/README.md)** — the documentation index. Highlights:

- [docs/product-overview.md](docs/product-overview.md) — product overview.
- [docs/architecture.md](docs/architecture.md) — architecture, flows, bus topics.
- [docs/api-reference.md](docs/api-reference.md) — endpoints, auth, topics, data model.
- [docs/cli-and-usage.md](docs/cli-and-usage.md) — CLI reference + end-to-end usage.
- [docs/auth-and-secrets.md](docs/auth-and-secrets.md) — auth, tokens, grants, env vars.
- [docs/runtime-and-sandbox.md](docs/runtime-and-sandbox.md) — runner loop + sandboxes.
- [docs/deployment-guide.md](docs/deployment-guide.md) — deploy the server.
- [examples/remote-claude/](examples/remote-claude/) — runnable example: workspace → sandbox.
- [CLAUDE.md](CLAUDE.md) — developer guide for AI agents working in this repo.
