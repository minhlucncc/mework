# Auth and Secrets Specification

## Purpose

Define the authentication and credential-protection model for `mework-server`:
two distinct token types guarding two route classes, AES-256-GCM sealing of
provider credentials at rest, and HMAC-based runtime-token lookup. Owned by
`internal/server/{auth,middleware,token,secret}`.

## Requirements

### Requirement: Two-token authentication

The system SHALL guard management routes (`/api/v1` runtimes, connections,
profiles) with a Mello personal access token (PAT) authenticator, and guard
daemon job routes (`/api/v1/jobs/*`) with a runtime token (`rt_token`)
authenticator. `/webhooks/{provider}` is exempt from both but signature-verified;
`/healthz` is open.

#### Scenario: PAT required for management routes

- **WHEN** a request hits a management route without a valid PAT
- **THEN** the system rejects it as unauthorized

#### Scenario: Runtime token required for job routes

- **WHEN** a daemon calls a job route without a valid `rt_token`
- **THEN** the system rejects it as unauthorized

### Requirement: Runtime token generation and lookup

The system SHALL generate runtime tokens with a recognizable prefix and
sufficient entropy (256-bit), return the plaintext token to the caller exactly
once at registration, and store only an HMAC-SHA256 lookup hash (keyed by
`SERVER_KEY`) so the raw token is never persisted.

#### Scenario: Token shown once

- **WHEN** a runtime is registered
- **THEN** the plaintext `rt_token` is returned once and only its HMAC lookup hash is stored

#### Scenario: Authenticate by lookup hash

- **WHEN** a daemon presents its `rt_token`
- **THEN** the server hashes it with `SERVER_KEY` and matches the stored lookup hash to identify the runtime

### Requirement: Credential sealing at rest

The system SHALL seal provider credentials with AES-256-GCM using
`MEWORK_SECRET_KEY` before storing them, and unseal them only server-side at the
moment they are needed for write-back.

#### Scenario: Stored credential is encrypted

- **WHEN** a provider connection credential is saved
- **THEN** it is stored as AES-256-GCM ciphertext, not plaintext

#### Scenario: Unseal only for write-back

- **WHEN** a write-back needs the provider credential
- **THEN** the server unseals it in memory for the call and does not expose it elsewhere

### Requirement: Required server secrets

The system SHALL require `DATABASE_URL`, `SERVER_KEY`, and `MEWORK_SECRET_KEY` at
startup and MUST fail fast if any is missing.

#### Scenario: Missing secret aborts startup

- **WHEN** the server starts without `MEWORK_SECRET_KEY`
- **THEN** it refuses to start rather than run without credential sealing
