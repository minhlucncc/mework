## ADDED Requirements

### Requirement: One-shot work delivered by SSE push

The daemon SHALL receive one-shot (webhook-triggered) work by **push** on its dispatch stream
(`runner.<id>.dispatch`), the same stream used for open-session dispatches, rather than by
polling a claim endpoint. On a one-shot dispatch the daemon SHALL pull the agent artifact, run
it, report the terminal result, and acknowledge the dispatch. The durable job record remains
the source of truth for status, deduplication, and lease; push is the delivery mechanism, and
a dispatch retained on the bus is delivered after a momentary disconnect.

#### Scenario: One-shot job arrives by push and is acked

- **WHEN** a webhook-triggered job is assigned to a runner
- **THEN** the runner receives a dispatch on its dispatch stream, runs it, reports the result,
  and acknowledges it — without polling a claim endpoint

#### Scenario: Dispatch is delivered after a brief disconnect

- **WHEN** the assigned runner is briefly disconnected when the dispatch is published
- **THEN** on reconnect it receives the retained dispatch and processes it
