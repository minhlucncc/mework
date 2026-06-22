## ADDED Requirements

### Requirement: NATS JetStream broker backend

The system SHALL provide a NATS JetStream implementation of the pluggable broker contract,
selectable by configuration, at semantic parity with the other backends: topic-filtered
subscription (including single- and multi-segment wildcards), durable and **resumable**
delivery from a given event id, **explicit acknowledgement** with redelivery until acked, and
bounded per-subscriber backpressure. Selecting the backend SHALL require no change to
publishers or subscribers (they use the broker interface).

#### Scenario: Publish, subscribe, and resume on NATS

- **WHEN** the NATS backend is selected and a subscriber resumes from a prior event id
- **THEN** it receives the retained messages it had not yet acked and then the live stream

#### Scenario: Ack prevents redelivery on NATS

- **WHEN** a subscriber acknowledges a delivered message on the NATS backend
- **THEN** that message is not redelivered

#### Scenario: Backend is a configuration choice

- **WHEN** the bus driver is configured to NATS
- **THEN** publishers and subscribers operate unchanged through the broker interface
