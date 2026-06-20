# Webhook Pipeline Specification

## Purpose

Define the inbound path that turns a provider event into an enqueued job:
`POST /webhooks/{provider}` receives an event, the matching adapter verifies its
signature and parses it, the trigger grammar is matched, and a canonical job is
enqueued exactly once. Owned by `internal/server/webhook`.

## Requirements

### Requirement: Webhook ingestion endpoint

The system SHALL expose `POST /webhooks/{provider}` that is unauthenticated by
PAT/runtime token but MUST verify the provider's request signature inside the
handler before acting on the payload.

#### Scenario: Reject an unsigned or mis-signed payload

- **WHEN** a webhook arrives whose signature does not verify for the provider
- **THEN** the system MUST reject it and MUST NOT enqueue a job

#### Scenario: Accept a valid signed payload

- **WHEN** a webhook arrives with a valid signature for a registered provider
- **THEN** the handler parses it via the provider adapter and proceeds to trigger matching

### Requirement: Trigger grammar

The system SHALL fire a job only for comments that match the trigger grammar
`@mework [profile] [workflow] [free instructions]`, where `profile` is the first
token, `workflow` is the second token when it is one of the recognized workflows
(`plan`, `cook`, `test`, `review`, `ship`, `journal`), and the remainder is free
instructions. `@mework` MUST be matched only at a word boundary (start of body,
or preceded by a space or newline). When the second token is a recognized
workflow, the parsed `workflow` value MUST be normalized to its canonical
lowercase form regardless of the casing or surrounding whitespace used in the
comment.

#### Scenario: Profile and workflow present

- **WHEN** a comment body is `@mework dev review fix the login bug`
- **THEN** the system parses profile `dev`, workflow `review`, and instructions `fix the login bug`

#### Scenario: Profile only

- **WHEN** a comment body is `@mework dev fix it`
- **THEN** the system parses profile `dev`, empty workflow, and instructions `fix it`

#### Scenario: Workflow keyword normalized to canonical case

- **WHEN** a comment body is `@mework dev Review fix the login bug`
- **THEN** the system parses workflow `review` (lowercased), not `Review`

#### Scenario: Not a trigger

- **WHEN** a comment body merely contains `@mework` inside another token (e.g. an email `test@mework.com`)
- **THEN** the system does NOT treat it as a trigger

### Requirement: Self-retrigger guard

The system SHALL NOT enqueue a job for a comment authored by the daemon's own
provider user, preventing feedback loops where a write-back re-triggers itself.

#### Scenario: Skip the daemon's own comment

- **WHEN** the triggering comment was authored by the same provider user the runtime writes back as
- **THEN** the system skips it and enqueues nothing

### Requirement: Idempotent enqueue

The system SHALL de-duplicate events using a unique constraint on
`(provider_code, external_event_id)`, so redelivered webhooks enqueue at most one
job.

#### Scenario: Duplicate webhook delivery

- **WHEN** the same provider event is delivered more than once
- **THEN** only one job exists for that `(provider_code, external_event_id)`
