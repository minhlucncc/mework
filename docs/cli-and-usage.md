# CLI & Usage Guide

> Audience: developers using MeWork from a kanban board, and operators of a local
> runner. This is the end-to-end walkthrough plus the full command reference. Status
> badges: **`[Implemented]`** today; **`[Planned — cNNNN]`** under `openspec/changes/`.

## How it fits together

```
External Task System (e.g. Mello)
      │                               ▲
      │ webhook                       │ write-back (REST API)
      ▼                               │
┌──────────────────────────────────────────────┐
│                MeWork Server                 │
│  - Inbound adapter (signature verify, parse) │
│  - PostgreSQL job queue                      │
│  - Outbound adapter (durable outbox)         │
└──────────────────────────────────────────────┘
      ▲                               │
      │ POST /api/v1/jobs/claim       │ POST /api/v1/jobs/:id/ack
      │ (rt_token)                    │ (status + results)
      ▼                               │
┌──────────────────────────────────────────────┐
│                MeWork Daemon                 │
│  - Local AI CLI (claude/codex/opencode)      │
│  - Isolated workspace (~/.mework/work/)      │
└──────────────────────────────────────────────┘
```

You comment on a ticket; the server enqueues the work; your local daemon runs the AI
CLI against your code; the server posts the result back to the ticket. Source code and
provider credentials never leave your machine.

This is the **legacy Mello pipeline** (poll/claim, the default webhook path). mework also
ships the **interactive session/sandbox** flow — `runner enroll` (install-once) + the daemon
subscribing over SSE, then `sandbox start` / `session create` to open a long-lived sandbox you
chat with over the bus. Both coexist; see [architecture.md](architecture.md).

## Command tree `[Implemented]`

Root: `mework` (cobra). Persistent flags: `--server-url` (env `MELLO_BASE_URL`),
`--workspace-id` (env `MELLO_WORKSPACE_ID`), `--profile` (env `MEWORK_PROFILE`),
`--debug` (env `MEWORK_DEBUG`).

### Core — provider task management (Mello REST)

| Command | Subcommands |
|---------|-------------|
| `login` | prompts for / accepts `--token`; validates via Mello `/me`; saves to config |
| `auth` | `status`, `logout` |
| `config` | `show` (token masked), `set <key> <value>` |
| `workspace` (alias `ws`) | `list` |
| `board` | `list`, `get <board-id>` |
| `ticket` (alias `t`) | `list <board-id>`, `get <ticket-id>`, `create`, `move <ticket-id>` |
| `comment` | `list <ticket-id>`, `add <ticket-id>` |
| `search <query>` | full-text search |

### Runner / server management `[Implemented]`

| Command | Subcommands | Auth |
|---------|-------------|------|
| `server` | `start [--listen]` — run the hub in-process (reads `DATABASE_URL`, `SERVER_KEY`, `MEWORK_SECRET_KEY`, …) | env |
| `runner` | `enroll --url --token` — exchange a registration token for a durable runner identity (install-once) | registration token |
| `daemon` | `start [--foreground]`, `stop`, `status`, `restart`, `logs [-f]` | runner identity |
| `agent` | `list [--json]` — list catalog agents | PAT |
| `provider` | `connect` (default `mello`; `--token`, `--webhook-secret`) | PAT |
| `profile` | `create`, `list`, `update`, `delete` (`--name`, `--body` file, `--backend`, `--harness`) | PAT |
| `runtime` *(legacy)* | `register --code [--label]`, `list`, `revoke --id` | PAT |
| `version` | — | — |

`runner enroll` is the **primary** enrollment path (install-once → unattended daemon);
`runtime register` remains for backward compatibility. `config set` accepts only:
`base_url`, `workspace_id`, `server_url`, `rt_token`, `daemon.trigger_keyword`,
`daemon.done_column_id`. The PAT `token` is **not** settable via `config set` (use `login`).

### Sessions & sandboxes `[Implemented]`

Drive a workspace as a server-addressable, chattable worker. `sandbox` is the
workspace-oriented façade; `session` is the lower-level API.

| Command | Subcommands | Auth |
|---------|-------------|------|
| `sandbox` | `start -w <dir> [--attach] [--json] [--idle <dur>]`, `list [--json]`, `stop <id>`, `send <id> <msg>` | PAT |
| `session` | `list [--json]`, `create --agent <name> [--runner <id>] [--version <v>] [--json]`, `send <id> <msg>`, `attach <id> [--idle <dur>]`, `close <id>` | PAT |

`sandbox start -w .` reads `<dir>/mework.yml`, targets the local enrolled runner, and creates
a workspace-bound session — the daemon opens a long-lived sandbox bound to that directory.
`sandbox send/stop/list` are aliases over `session send/close/list`. `attach` streams
`token`/`message`/`done`/`error` events over SSE until a terminal event or the idle timeout.

## End-to-end setup

### 1. Log in with your Mello PAT
```bash
mework login --token mello_pat_xxxxxx
```
Omit `--token` to be prompted securely (keeps the token out of shell history).

### 2. Point the CLI at the server
```bash
mework config set server_url http://localhost:8080
```
Saved in `~/.mework/config.json`.

### 3. Connect a provider (for write-back)
So the server can post results back to your board:
```bash
mework provider connect --provider mello --token mello_pat_xxx
```
The server stores this token **sealed** with `MEWORK_SECRET_KEY` and uses it only for
outbound API calls on your behalf. Omit `--token` to be prompted.

### 4. Enroll this machine as a runner  `[Implemented]`
Enrollment exchanges a one-time **registration token** for a durable runner identity,
persisted to `~/.mework/identity.json` so the daemon runs unattended:
```bash
# Operator issues a registration token (PAT-authed):
REG=$(curl -s -XPOST "$HUB/api/v1/runners/registration-tokens" \
        -H "Authorization: Bearer <mello-pat>" | jq -r .token)
# This machine enrolls:
mework runner enroll --url "$HUB" --token "$REG"
```

<details><summary>Legacy: <code>runtime register</code> (backward-compat)</summary>

```bash
mework runtime register --code macbook-claude --label "MacBook Pro · Claude"
mework config set rt_token mework_rt_xxx     # save the one-time token
mework runtime list
mework runtime revoke --id <uuid>
```
</details>

### 5. Create an AI profile
A profile is the server-side system prompt + backend hint + harness:
```bash
mework profile create --name default \
  --body path/to/system_prompt.md --backend claude --harness claude-code
mework profile list
mework profile update --name default --body path/to/new_prompt.md
mework profile delete --name default
```
The profile name is what you reference as `<profile>` in the trigger. When a job is
enqueued the server snapshots the profile body into the job payload.

### 6. Start the daemon
```bash
mework daemon start              # background
mework daemon start --foreground # in terminal, prints logs
mework daemon status
mework daemon logs -f
mework daemon stop
```
The daemon needs `server_url` + `rt_token` in config and at least one AI CLI on `PATH`
(`claude`, `codex`, or `opencode`). See [runtime-and-sandbox.md](runtime-and-sandbox.md)
for the lifecycle and state files.

## Triggering an agent from a ticket

With the daemon running, comment on a Mello card:

```
@mework <profile> [workflow] <free instructions>
```

- `profile` — the first token; the profile/runtime to use.
- `workflow` — optional; recognized only when it is one of `plan`, `cook`, `test`,
  `review`, `ship`, `journal` (case-insensitive). Otherwise everything after the
  profile is treated as instructions.
- the rest is free-form instructions for the AI.

### Examples
Fix type errors with the default profile:
```
@mework default fix the type errors in libs/server/hub/health.go
```
Build a component with a specialized profile and the `review` workflow:
```
@mework frontend-fix review create a Button component with a hover animation
```

### What happens automatically
1. The card records your `@mework` comment.
2. The server verifies the signature, parses the trigger, and enqueues a job for the
   target runtime (deduped on the delivery id).
3. Your daemon claims the job and reads the snapshotted title/description + profile +
   instructions from the payload.
4. It runs the local AI CLI in an isolated workspace (`~/.mework/work/<job-id>/`), with
   the prompt fed over stdin.
5. It acks the result to the server.
6. The server writes the result back onto the card via the provider REST API, through a
   durable outbox (exactly-once — no duplicate comments on restart).

## Driving a workspace as a sandbox `[Implemented]`

Instead of (or alongside) the ticket trigger, turn a workspace folder into a long-lived,
chattable worker. The folder needs a `mework.yml` (engine + backend), e.g.:

```yaml
name: local-claude
version: 1.0.0
engine: local        # local | docker | cloudflare | custom
backend: claude       # command[0]; the turn arrives on stdin
```

With the hub running and this machine enrolled + the daemon started:

```bash
SID=$(mework sandbox start -w . --json | jq -r .id)   # server → dispatch → daemon opens the sandbox
mework sandbox list                                    # SID, agent, status
mework session attach "$SID"        # terminal A: stream token/message/done events
mework sandbox send  "$SID" "summarize this repo and list entry points"   # terminal B: a turn
mework sandbox stop  "$SID"
```

The turn travels CLI → server (`session.<id>.input`) → daemon → the long-lived sandbox over
**stdin (never argv)**; events stream back over `session.<id>.control`. See
[runtime-and-sandbox.md](runtime-and-sandbox.md) and
[examples/remote-claude/](../examples/remote-claude/) for a runnable walkthrough.

## Local vs server-side profiles

Two different things share the word "profile":

- **Local daemon profile** — `--profile dev` (or `MEWORK_PROFILE`) isolates local
  config, pid, logs, and workspaces under `~/.mework/profiles/dev/`. For running
  multiple independent daemons on one machine.
- **Server-side AI profile** — created via `mework profile create/update`, stored on
  the server, snapshotted into each job. This is what `<profile>` in the trigger
  refers to.

## Not yet implemented

- `mework update` (self-update) — deferred until there is a published GitHub release.
  The `.goreleaser.yml` + `Makefile` provide the build machinery; the download/verify/
  swap flow needs the real release repo first.
