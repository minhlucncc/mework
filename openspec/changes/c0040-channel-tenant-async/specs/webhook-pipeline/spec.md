## ADDED Requirements

### Requirement: Webhook intake does not block on provisioning

Webhook intake SHALL acknowledge a verified, deduplicated event promptly and MUST NOT block on
channel auto-provisioning. Worker selection and its retries SHALL happen after the
acknowledgement, off the request path, so inbound webhook latency is independent of worker
availability.

#### Scenario: Prompt acknowledgement regardless of worker availability

- **WHEN** a verified webhook triggers auto-provisioning for a channel with no ready worker
- **THEN** the webhook is acknowledged promptly and provisioning proceeds in the background
