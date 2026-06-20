# Daemon Runtime Specification

## Purpose

Define the local agent daemon: a **stateless** long-poll worker that claims jobs
from `mework-server`, runs an AI coding CLI against the ticket in an isolated
workspace, and reports the result. It holds no provider credentials and persists
no job state of its own. Owned by `internal/daemon` and `internal/agentrun`.

## Requirements

### Requirement: Stateless poll worker

The system SHALL implement the daemon as a stateless loop that polls the server
to claim a job, acks it `running`, heartbeats periodically while it executes, and
acks `done`/`failed` with a result summary. The daemon MUST authenticate to job
routes with its runtime token (`rt_token`) and MUST NOT persist local job state.

#### Scenario: Claim, execute, ack

- **WHEN** a job is available and the daemon polls
- **THEN** the daemon claims it, acks `running`, executes it, and acks the terminal result

#### Scenario: Heartbeat while running

- **WHEN** a job execution is in progress
- **THEN** the daemon heartbeats periodically so the server lease does not expire

### Requirement: AI backend detection

The system SHALL detect an installed AI CLI backend (default discovery order
`claude` → `codex` → `opencode` via `PATH`) and execute the one selected by the
job's profile/backend.

#### Scenario: Fall back to the next backend

- **WHEN** the preferred backend is not found on `PATH`
- **THEN** the daemon selects the next available backend in discovery order

### Requirement: Safe, isolated execution

The system SHALL feed the prompt to the backend over **stdin, never as a shell
argument** (ticket content is attacker-controllable), execute in an isolated
per-job working directory, and bound each run with a timeout (default 30
minutes).

#### Scenario: Prompt is not placed on the command line

- **WHEN** the daemon runs a backend with ticket-derived prompt content
- **THEN** the prompt is written to the process stdin and never appears in argv

#### Scenario: Runaway run is bounded

- **WHEN** a backend run exceeds its timeout
- **THEN** the run is cancelled and the job is acked `failed`

### Requirement: Daemon lifecycle management

The system SHALL provide lifecycle controls — `daemon start` (background, or
`--foreground` in-process), `stop`, `status`, `restart`, and `logs` — with
per-profile pid, log, and work directories so multiple profiles can run isolated.

#### Scenario: Inspect a running daemon

- **WHEN** a user runs `mework daemon status`
- **THEN** the system reports whether the daemon is running and its health for the active profile
