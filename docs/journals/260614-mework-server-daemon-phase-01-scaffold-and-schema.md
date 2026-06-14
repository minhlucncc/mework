# Mework Server - Phase 1: Scaffold and Schema
Date: 2026-06-14

## Context & Key Decisions
- **Postgres over SQLite**: A stateful central server (Go + Postgres) is selected for the Mework server backend. This decision is driven by the requirement of a 24/7 VPS that handles shared multi-account routing, concurrent daemon long-poll connection claims, and a persistent TTL-based job queue. This avoids a future SQLite-to-Postgres scaling migration.
- **Embedded SQL Migrations via Goose**: We chose `pressly/goose/v3` to manage schema migrations. Using a single migration file (`000001_init.sql`) with `-- +goose Up` and `-- +goose Down` annotation markers is cleaner and prevents version collisions that occur when Go embeds separate `.up.sql` and `.down.sql` files as duplicate versions.

## Key Changes
- **Core Tables & Schema Deltas**:
  - `accounts`: Central table mapped to users.
  - `provider_connections`: Maps accounts to target provider credentials and metadata (such as `webhook_secret` and MCP settings), uniquely keyed by `(account_id, provider_code)`.
  - `account_identities`: Maps target platform user IDs to the system's `accounts`, uniquely keyed by `(provider_code, external_user_id)`.
  - `watched_containers`: Maps target platform container IDs (e.g. board IDs) to the system's `accounts`, uniquely keyed by `(provider_code, external_container_id)`.
  - `runtimes`: Incorporates a `token_lookup` column storing an indexed `HMAC-SHA256` hash of the runtime token for O(1) verification on the claim hot-path. Includes a UNIQUE constraint on `(account_id, code)`.
  - `profiles`: Holds account-isolated markdown prompt instructions.
  - `jobs`: Stores task details, metadata, and execution states. Incorporates `attempts` and `last_error` columns for retry limit enforcement, and a snapshot of the task context (`task_title`, `task_description`, `profile_body_snapshot`) along with provider metadata (`external_task_id`, `external_event_id`, and `provider_code`).
- **Targeted Database Indexes**:
  - `idx_jobs_claim`: Composite index on `(runtime_id, status, created_at)` to support rapid queued job lookup.
  - `idx_jobs_one_active_per_runtime`: A partial unique index on `(runtime_id) WHERE status IN ('claimed', 'running')` that enforces the single-job-per-runtime invariant at the database layer.
- **Server Scaffold**:
  - Net/HTTP chi router configured with request ID, logging, and recovery middleware.
  - Health check endpoint `/healthz` executing a database connection check (DB ping) with a 2-second timeout context.
  - Graceful shutdown sequence intercepting `SIGINT`/`SIGTERM` to allow connections to drain before closing the database pool.

## Test Validation
- **Unit and Integration Tests**:
  - `config_test.go`: Asserts environment loader validation and defaults.
  - `health_test.go`: Verifies health handler return codes (200 OK vs 503 Service Unavailable) against nil, active, and closed database pools.
  - `migrate_test.go`: An integration test running against a local test database (`mework_test` in PostgreSQL container), executing a full rollback, migration up, table and field verification via `information_schema`, index presence assertion, and final teardown.
- All tests pass successfully.

## Next Steps
- Implement Phase 2: Account Mapping & Runtime Registry (minting `rt_token`, authenticating via Mello `/me`, routing token lookups, and ensuring IDOR checks).
