## ADDED Requirements

### Requirement: Tenant-scoped, asynchronous auto-provisioning

When no session exists for a channel, the system SHALL auto-provision one by selecting an
eligible worker **in the tenant that owns the channel's `(provider_code, resource_id)`** —
derived from the provider connection / watched container the event was verified against — and
MUST NOT restrict selection to a fixed default tenant. Auto-provisioning (worker selection and
its bounded retry) SHALL run **asynchronously**, off the inbound request path, so the request
that triggered it is not blocked on worker availability. Provisioning failures SHALL be logged
and reflected in channel status rather than blocking the caller.

#### Scenario: Worker selected in the event's tenant

- **WHEN** a webhook arrives for a channel owned by a non-default tenant and no session exists
- **THEN** auto-provisioning selects an eligible worker in that tenant and binds the channel

#### Scenario: Provisioning does not block the request

- **WHEN** a channel has no eligible worker and provisioning must retry
- **THEN** the triggering request returns promptly while selection retries in the background

#### Scenario: No eligible worker is logged, not blocking

- **WHEN** no eligible worker becomes available within the bounded retry
- **THEN** the failure is logged / reflected in channel status and no request goroutine is held
