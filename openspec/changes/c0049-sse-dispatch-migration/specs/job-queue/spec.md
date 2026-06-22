## ADDED Requirements

### Requirement: Push delivery with the job record as source of truth

Webhook-triggered jobs SHALL be **delivered by push** to the assigned runner over the dispatch
stream, while the durable job record remains the authoritative source for status,
deduplication (`provider_code` + `external_event_id`), and lease. The legacy poll/claim route
SHALL be retired: requests to the former claim endpoint return not-found. Terminal-state
immutability and at-least-once delivery (idempotent terminal acks) are preserved.

#### Scenario: Enqueue writes the durable job and pushes a dispatch

- **WHEN** a verified, deduplicated trigger enqueues a job
- **THEN** the durable job row is written and a dispatch is pushed to the assigned runner

#### Scenario: Legacy claim route is retired

- **WHEN** a client POSTs to the former `/api/v1/jobs/claim` endpoint
- **THEN** the server responds not-found (the route no longer exists)
