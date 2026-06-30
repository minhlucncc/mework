# Daemon Runtime Specification

## Purpose

Define the local agent daemon: a **stateless** long-poll worker that claims jobs
from `mework-server`, runs an AI coding CLI against the ticket in an isolated
workspace, and reports the result. It holds no provider credentials and persists
no job state of its own. Owned by `internal/daemon` and `internal/agentrun`.
## Requirements
### Requirement: Stateless poll worker

The daemon SHALL operate as an **enrolled SSE runner**, not a poll worker. After
one-time enrollment it MUST subscribe to its topics over SSE (per `message-bus`),
receive dispatches by push, and drive a **pull → run → report** loop, persisting
only its durable runner identity and in-flight bookkeeping. It MUST NOT poll a
claim endpoint on a fixed interval.

#### Scenario: No interval polling

- **WHEN** an enrolled runner is online and idle
- **THEN** it holds an open SSE subscription and issues no periodic claim/poll requests

#### Scenario: Driven by dispatch

- **WHEN** the hub dispatches work to the runner
- **THEN** the runner pulls the agent, runs it, and reports the result, acknowledging the dispatch on terminal handling

### Requirement: AI backend detection

The system SHALL detect an installed AI CLI backend (default discovery order
`claude` → `codex` → `opencode` via `PATH`) and execute the one selected by the
job's profile/backend.

#### Scenario: Fall back to the next backend

- **WHEN** the preferred backend is not found on `PATH`
- **THEN** the daemon selects the next available backend in discovery order

### Requirement: Safe, isolated execution

The system SHALL feed the prompt (and, for interactive sessions, **each turn**) to the
backend over **stdin, never as a shell argument** (ticket/turn content is
attacker-controllable), execute in an isolated per-job/per-session working directory,
and bound each run by a timeout. A **one-shot** run is bounded by a per-run timeout
(default 30 minutes). A **long-lived interactive** sandbox is instead bounded by its
**session lifecycle** — explicit close or idle reaping — rather than a single per-run
timeout, while individual turns MAY still carry a per-turn bound.

#### Scenario: Prompt is not placed on the command line

- **WHEN** the daemon runs a backend with ticket-derived prompt content
- **THEN** the prompt is written to the process stdin and never appears in argv

#### Scenario: Turn content is not placed on the command line

- **WHEN** the daemon delivers an interactive turn to a long-lived sandbox
- **THEN** the turn content is written to the process stdin and never appears in argv

#### Scenario: Runaway run is bounded

- **WHEN** a one-shot backend run exceeds its timeout
- **THEN** the run is cancelled and the job is acked `failed`

#### Scenario: Long-lived sandbox is bounded by its session

- **WHEN** an interactive session is closed or reaped for idleness
- **THEN** the daemon destroys the long-lived sandbox bounding its lifetime

### Requirement: Daemon lifecycle management

The system SHALL provide lifecycle controls — `daemon start` (background, or
`--foreground` in-process), `stop`, `status`, `restart`, and `logs` — with
per-profile pid, log, and work directories so multiple profiles can run isolated.

#### Scenario: Inspect a running daemon

- **WHEN** a user runs `mework daemon status`
- **THEN** the system reports whether the daemon is running and its health for the active profile

### Requirement: Spec-based enrollment

The daemon SHALL declare the agent specs it can execute during enrollment. Specs SHALL be sent as an array in the enrollment request body. The daemon SHALL determine its supported specs from the AI CLIs it has installed at startup time (e.g., if `claude` is found in PATH, the daemon includes `"claude-code"` in its specs).

#### Scenario: Enroll with detected specs

- **WHEN** the daemon starts and detects `claude` and `opencode` in PATH
- **THEN** it enrolls with `specs: ["claude-code", "opencode"]`

### Requirement: Channel subscription on sandbox provision

When the daemon receives a channel dispatch, the sandbox SHALL subscribe to the channel's topic on the message bus using the filter `channel.<provider>.<resource_id>.*`.

#### Scenario: Sandbox subscribes to its assigned channel

- **WHEN** a sandbox is provisioned for channel `"mello:TICKET-99"` on a runner
- **THEN** the sandbox opens a bus subscription with filter `channel.mello.TICKET-99.*`

### Requirement: Heartbeat with current specs

The daemon SHALL include its current specs in every heartbeat sent to the server, allowing the server to detect when a runner's capabilities change.

#### Scenario: Heartbeat updates specs

- **WHEN** the daemon heartbeats with `{"specs": ["claude-code"]}`
- **THEN** the server updates `runtimes.specs` for that runner

### Requirement: Interactive sandbox session execution

The daemon SHALL be able to drive a **long-lived sandbox per session** in addition to
one-shot dispatch. It MUST start the sandbox once for the session, deliver each chat
turn to the running agent over stdin, support **cancel/interrupt** of an in-flight
turn without destroying the sandbox, and destroy the sandbox when the session closes
or is reaped. The **one-agent-per-sandbox** invariant MUST hold for the session.

#### Scenario: Sandbox persists across turns

- **WHEN** the daemon receives a second turn for an open session
- **THEN** it routes the turn to the same long-lived sandbox started for the first turn

#### Scenario: Cancel interrupts the turn but keeps the sandbox

- **WHEN** a cancel arrives for an in-flight turn
- **THEN** the daemon interrupts the turn and leaves the sandbox running for the next turn

### Requirement: Daemon streams session events

While driving an interactive session, the daemon SHALL **stream events** for each turn
(at least `token`, `message`, and one terminal `done`/`error`) to the session's topic
on the message bus, so subscribers can observe progress live.

#### Scenario: Events published per turn

- **WHEN** the daemon runs an interactive turn
- **THEN** it publishes `token`/`message` events and exactly one terminal `done` or `error` for that turn to the session topic

### Requirement: Open-session dispatch drives the interactive session

The daemon SHALL treat a dispatch identified by a non-empty session id as an
**open-session dispatch** and drive a **long-lived interactive session** for that id rather
than running a one-shot agent. It MUST verify the dispatch grant (requiring spawn
permission), authorize against the **owner and tenant** carried on the dispatch, resolve the
agent definition, and start the sandbox **exactly once** for the session. The daemon SHALL
then **consume chat turns from the session's input topic** and route each turn to the
running session **serially**, and SHALL **deliver the session's per-turn events to the
server** for relay to subscribers.
A **duplicate** open-session dispatch for an already-open session id MUST NOT start a second
sandbox. Closing or cancelling the session MUST follow the interactive-session lifecycle
(cancel interrupts an in-flight turn; close destroys the sandbox).

#### Scenario: Open-session dispatch starts one long-lived sandbox

- **WHEN** the daemon receives a dispatch carrying a session id, owner, tenant, and a
  pull+spawn grant
- **THEN** it authorizes the caller, resolves the definition, and opens the session's
  sandbox once

#### Scenario: Turns from the input topic route to the session

- **WHEN** chat turns arrive on the session's input topic for an open session
- **THEN** the daemon delivers each turn to the same long-lived sandbox, one after another

#### Scenario: Duplicate dispatch is idempotent

- **WHEN** the daemon receives a second open-session dispatch for a session it already has
  open (e.g. on stream resume/redelivery)
- **THEN** it does not start a second sandbox for that session

#### Scenario: Per-turn events reach the server

- **WHEN** the daemon runs an interactive turn for a server-routed session
- **THEN** it delivers the turn's `token`/`message` events and one terminal `done`/`error`
  to the server so subscribers can observe them

### Requirement: Workspace-bound open-session dispatch

When an open-session dispatch carries a **workspace path**, the daemon SHALL resolve the
agent definition from that workspace's `mework.yml` (a local file resolver) instead of the
server catalog, and SHALL bind the long-lived sandbox to that directory so the agent runs
with the workspace as its working directory. The path is interpreted on the daemon host. When
the dispatch carries no workspace path, the daemon SHALL resolve the definition from the
server catalog as before. All other open-session behavior (one sandbox per session, serial
turns from the input topic, per-turn events to the server) is unchanged.

#### Scenario: Dispatch with a workspace binds the directory

- **WHEN** the daemon receives an open-session dispatch carrying a workspace path
- **THEN** it resolves the definition from that workspace's `mework.yml` and starts the
  sandbox bound to that directory

#### Scenario: Dispatch without a workspace uses the catalog

- **WHEN** the daemon receives an open-session dispatch with no workspace path
- **THEN** it resolves the definition from the server catalog, unchanged

#### Scenario: Missing workspace definition fails the session

- **WHEN** the dispatch's workspace path has no readable `mework.yml` on the daemon host
- **THEN** the daemon reports the session failed rather than starting an unbound sandbox

### Requirement: Daemon can host a Mezon bot proxy

The enrolled daemon SHALL optionally host a Mezon bot listener as a proxy, connecting to Mezon's WebSocket gateway and dispatching received messages to the channel router on the server via the SSE bus. This allows the daemon to serve as the bot endpoint rather than requiring the server to maintain the WebSocket connection.

#### Scenario: Daemon starts Mezon bot

- **WHEN** the daemon enrolls with `mezon_bot: true` in its enrollment specs
- **THEN** the daemon starts a Mezon bot client that connects to Mezon and receives messages

#### Scenario: Daemon routes Mezon messages to server

- **WHEN** the daemon's Mezon bot receives a channel message
- **THEN** the daemon forwards the message to the server via the SSE bus for channel routing

### Requirement: Daemon spec includes mezon-bot capability

The daemon SHALL include `"mezon-bot"` in its enrollment specs when configured to host a Mezon bot. The server SHALL use this spec to select a daemon capable of running the Mezon bot when auto-provisioning channels for Mezon.

#### Scenario: Enrolled daemon advertises mezon-bot

- **WHEN** the daemon enrolls with Mezon bot configuration
- **THEN** it includes `"mezon-bot"` in its `specs` array on enrollment and heartbeat

