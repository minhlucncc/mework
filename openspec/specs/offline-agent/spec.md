# Offline Agent

## Purpose

Defines the capability to run a self-contained, zero-infrastructure local agent
that operates without a hub server, Postgres, or any network-accessible service.
All state is in-memory or on the local filesystem.

## Requirements

### Requirement: User can start an offline-mode agent bound to a workspace directory

The system SHALL provide a `mework start --workspace <dir> --offline` command
that boots a self-contained local agent process. The agent SHALL resolve its
definition from `<dir>/mework.yml` using a file-system resolver, start the
sandbox (local engine), and listen for one-shot task submissions via the CLI.
The command SHALL fail with a clear error when `--workspace` is missing.

#### Scenario: Start offline agent with valid workspace
- **WHEN** user runs `mework start --workspace /tmp/my-workspace --offline`
- **THEN** the agent reads `/tmp/my-workspace/mework.yml` for the agent definition
- **THEN** the agent starts the local sandbox with the configured backend
- **THEN** the agent prints its status and waits for task submissions
- **THEN** the agent stays running until a `mework stop` or SIGINT/SIGTERM

#### Scenario: Start offline agent without workspace
- **WHEN** user runs `mework start --offline` without `--workspace`
- **THEN** the system prints an error: `--workspace is required in offline mode`
- **THEN** the system exits with a non-zero exit code

#### Scenario: Start offline agent when workspace has no mework.yml
- **WHEN** user runs `mework start --workspace /tmp/empty-dir --offline`
- **THEN** the system prints an error: `no mework.yml found in workspace`
- **THEN** the system exits with a non-zero exit code

### Requirement: User can run a one-shot task against the offline-mode agent

The system SHALL provide a `mework run <instruction>` command that sends a
one-shot task to the running offline-mode agent. The agent SHALL process the
task through its sandbox and stream the result to stdout. The instruction text
SHALL be passed to the agent over stdin (never argv — injection safety
invariant). The command SHALL exit with the agent's exit code when the task
completes.

#### Scenario: Run task when agent is running
- **WHEN** user runs `mework run "list files in the workspace"`
- **THEN** the instruction `list files in the workspace` is delivered to the agent via stdin
- **THEN** the agent processes the instruction through the configured backend
- **THEN** the agent's output is streamed to stdout in real-time
- **THEN** the command exits with code 0 on success

#### Scenario: Run task when no agent is running
- **WHEN** user runs `mework run "hello"` but no offline agent was started
- **THEN** the system prints an error: `no offline agent running — run 'mework start --offline' first`
- **THEN** the system exits with a non-zero exit code

#### Scenario: Run task with empty instruction
- **WHEN** user runs `mework run ""` or `mework run` with no args
- **THEN** the system prints an error: `instruction is required`
- **THEN** the system exits with a non-zero exit code

### Requirement: Agent resolves definition from workspace mework.yml

The offline-mode agent SHALL resolve its definition from `<workspace>/mework.yml`
using a file-system definition resolver (the existing `FileDefinitionResolver`).
The `mework.yml` SHALL specify `name`, `engine`, and `backend` fields. The
agent SHALL use the `engine` field to select the sandbox runtime and the
`backend` field as the subprocess command.

#### Scenario: Resolve valid mework.yml
- **WHEN** workspace contains `mework.yml` with `engine: local` and `backend: claude`
- **THEN** the agent starts a local sandbox that spawns `claude` as the backend subprocess

#### Scenario: Resolve mework.yml with unsupported engine
- **WHEN** workspace contains `mework.yml` with `engine: docker` (not supported in offline mode)
- **THEN** the system prints an error: `offline mode supports only 'local' engine`
- **THEN** the system exits with a non-zero exit code

### Requirement: Offline mode requires zero external infrastructure

The offline-mode agent SHALL NOT depend on Postgres, a hub server, a provider
adapter, or any network-accessible service. All state SHALL be in-memory or on
the local filesystem. The agent SHALL NOT attempt to connect to `MEWORK_HUB_URL`
or register as a runner. No PAT, rt_token, or enrollment is required.

#### Scenario: Start agent with no env vars configured
- **WHEN** user runs `mework start --workspace . --offline` with no `MEWORK_HUB_URL` or `DATABASE_URL`
- **THEN** the agent starts successfully without attempting any network connections

#### Scenario: Start agent with network unavailable
- **WHEN** user runs `mework start --workspace . --offline` with no network access
- **THEN** the agent starts and processes tasks without any network-related errors

### Requirement: Offline agent can host a Mezon bot listener

The offline-mode agent SHALL optionally start a Mezon bot listener as an additional input channel alongside the existing Unix socket listener. The Mezon bot SHALL be started only when Mezon credentials are configured in the workspace's `mework.yml` or via environment variables (`MEZON_APP_ID`, `MEZON_API_KEY`). The offline agent SHALL NOT fail if Mezon credentials are absent -- it simply does not start the bot.

#### Scenario: Start offline agent with Mezon credentials

- **WHEN** user runs `mework start --workspace /tmp/ws --offline` and `mework.yml` contains `mezon.app_id: "myapp"` and `mezon.api_key: "secret123"`
- **THEN** the offline agent starts both the Unix socket listener and the Mezon bot WebSocket listener
- **THEN** messages from both sources are accepted and dispatched to the sandbox

#### Scenario: Start offline agent without Mezon credentials

- **WHEN** user runs `mework start --workspace /tmp/ws --offline` with no Mezon credentials
- **THEN** the offline agent starts with only the Unix socket listener (existing behavior)
- **THEN** no Mezon connection is attempted

### Requirement: Mezon messages in offline mode use policy enforcement

Messages arriving from the Mezon bot in offline mode SHALL be processed through the same `policy.Policy` engine as Unix socket messages. The policy attributes SHALL include `"channel": "mezon:<channel_id>"` instead of `"channel": "local"`, enabling distinct policy rules for Mezon-sourced messages.

#### Scenario: Policy enforcement for Mezon message

- **WHEN** a Mezon message arrives at the offline agent with `channel_id = "ch_abc"`
- **THEN** the policy is evaluated with `"channel": "mezon:ch_abc"` in the attributes

#### Scenario: Policy blocks Mezon message

- **WHEN** a Mezon message is rejected by the policy engine
- **THEN** the Mezon bot sends an error reply to the originating channel (when configured)
- **THEN** the message is not dispatched to the sandbox

### Requirement: Offline Mezon bot sends replies to channel

When the offline agent processes a Mezon message, the agent's response SHALL be sent back to the originating Mezon channel via the bot's `SendMessage()` method. The bot client SHALL be started in a mode that supports both receiving and sending.

#### Scenario: Reply sent to originating channel

- **WHEN** the offline agent finishes processing a Mezon-sourced message
- **THEN** the agent's output is sent as a message to the same Mezon channel the request came from

### Requirement: Offline mode credentials from mework.yml

The workspace `mework.yml` SHALL support optional `mezon` configuration fields: `app_id`, `api_key`, and optionally `base_url`. These SHALL be read at agent startup and passed to the bot client. When present, `api_key` SHALL be held in memory only and never written to disk in plaintext (the user is responsible for securing `mework.yml`).

#### Scenario: Read Mezon config from mework.yml

- **WHEN** `mework.yml` contains `mezon: {app_id: "myapp", api_key: "secret123"}`
- **THEN** the offline agent starts the Mezon bot with those credentials

### Requirement: Offline Mezon bot does not depend on Postgres or hub

The offline mode Mezon bot SHALL operate without Postgres, a hub server, or any provider adapter registration. It SHALL use the bot client directly (not through the `provider.Registry` or channel router). Messages are dispatched directly to the local sandbox, and replies are sent directly via the bot client's `SendMessage()`.

#### Scenario: No database dependency

- **WHEN** the offline agent starts a Mezon bot with no `DATABASE_URL` set
- **THEN** the bot connects to Mezon and processes messages without any database access
