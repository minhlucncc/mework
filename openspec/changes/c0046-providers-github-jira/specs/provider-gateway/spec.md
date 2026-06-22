## ADDED Requirements

### Requirement: GitHub and Jira provider adapters

The system SHALL provide working **GitHub Issues** and **Jira** provider adapters implementing
the provider adapter contract — webhook **signature verification**, trigger parsing of the
`@mework` grammar over the provider's comment events, channel-key derivation, and REST
**write-back** (creating a comment on the issue/PR) — and SHALL register them so a provider
connection activates each. Adapting a provider MUST NOT require a schema migration (entities
are identified by `(provider_code, external_*_id)`).

#### Scenario: GitHub comment triggers a job and writes back

- **WHEN** a signature-valid GitHub issue/PR comment matching the `@mework` grammar arrives
- **THEN** the pipeline enqueues a job and the result is written back as a GitHub comment

#### Scenario: Jira comment triggers a job and writes back

- **WHEN** a signature-valid Jira issue comment matching the `@mework` grammar arrives
- **THEN** the pipeline enqueues a job and the result is written back as a Jira comment

#### Scenario: Invalid signature is rejected

- **WHEN** a GitHub or Jira webhook arrives with an invalid signature
- **THEN** the adapter rejects it and no job is enqueued

#### Scenario: Adding the provider needs no migration

- **WHEN** a GitHub or Jira connection is configured
- **THEN** the provider activates without any database schema change
