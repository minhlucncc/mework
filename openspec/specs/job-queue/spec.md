# Job Queue Specification

## Purpose

Define the durable, Postgres-backed job lifecycle that connects the webhook
pipeline to the daemon: enqueue, long-poll claim, ack, heartbeat, a transactional
state machine, and a background sweeper that recovers leases. Owned by
`internal/server/jobs`.

## Requirements

### Requirement: Transactional state machine

The system SHALL enforce job status transitions inside a transaction with row
locking (`SELECT ... FOR UPDATE`). Allowed transitions are `queued → claimed|failed`,
`claimed → running|done|failed|queued`, and `running → done|failed|queued`.
Terminal states (`done`, `failed`) MUST be immutable. A transition to the current
status MUST be an idempotent no-op. Entering `running` MUST set `started_at`;
entering `done`/`failed` MUST set `finished_at`.

#### Scenario: Reject a transition out of a terminal state

- **WHEN** a job is `done` and a transition to `running` is attempted
- **THEN** the system returns an invalid-transition error and leaves the job unchanged

#### Scenario: Idempotent re-ack

- **WHEN** an ack sets a job to a status it already holds
- **THEN** the system treats it as a no-op and succeeds

### Requirement: Exactly-once claim

The system SHALL allow a runtime to claim at most one active job at a time and
SHALL hand a queued job to exactly one runtime, using row-level locking
(`FOR UPDATE SKIP LOCKED`) and a partial unique index enforcing one active job
per runtime.

#### Scenario: Concurrent claims do not double-assign

- **WHEN** two runtimes poll the queue simultaneously for the same queued job
- **THEN** exactly one claim succeeds and the other receives no job

#### Scenario: One active job per runtime

- **WHEN** a runtime already holds a claimed/running job and attempts to claim another
- **THEN** the system denies the second claim until the first reaches a terminal state

### Requirement: Heartbeat and lease

The system SHALL accept periodic heartbeats from the claiming runtime to extend a
job's lease while it runs, and SHALL expose ack endpoints to report `running`,
`done`, and `failed` outcomes with a result summary.

#### Scenario: Heartbeat extends the lease

- **WHEN** a runtime heartbeats a running job before its lease expires
- **THEN** the lease is extended and the sweeper does not reclaim the job

### Requirement: Lease sweeper

The system SHALL run a background sweeper that returns jobs whose lease has
expired (runtime crashed or stalled) back to `queued` so another runtime can
claim them, and that drives pending write-backs.

#### Scenario: Reclaim an abandoned job

- **WHEN** a claimed/running job's lease expires with no heartbeat
- **THEN** the sweeper transitions it back to `queued` for re-claim
