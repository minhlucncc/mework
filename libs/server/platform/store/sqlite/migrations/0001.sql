-- Initial SQLite schema for mework. Mirrors the Postgres schema in
-- libs/server/platform/store/migrations/*.sql with two substitutions:
--   * UUID → TEXT (text-encoded UUIDs; generated in Go with uuid.NewString)
--   * JSONB → TEXT holding JSON-encoded payloads (boundary: encoding/json)
--
-- FK constraints on the tenant-scoped tables (runtimes, profiles,
-- provider_connections, …) are dropped in the offline driver because
-- single-tenant offline runs do not benefit from cascading deletes and
-- the test fixtures insert rows out of dependency order. The Postgres
-- counterpart keeps them.

CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed the default account so callers / fixtures can reference a known
-- account without an explicit insert. The Postgres counterpart creates
-- this in 000002_tenancy.sql; the offline driver ships the row as part
-- of v1 to keep the test fixtures honest.
INSERT OR IGNORE INTO accounts (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');

CREATE TABLE IF NOT EXISTS provider_connections (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    provider_code TEXT NOT NULL,
    webhook_secret TEXT,
    mcp_url TEXT,
    mcp_auth_enc TEXT,
    config TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, provider_code)
);

CREATE TABLE IF NOT EXISTS account_identities (
    account_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    provider_code TEXT NOT NULL,
    external_user_id TEXT NOT NULL,
    PRIMARY KEY (account_id, provider_code),
    UNIQUE (provider_code, external_user_id)
);

CREATE TABLE IF NOT EXISTS watched_containers (
    account_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    provider_code TEXT NOT NULL,
    external_container_id TEXT NOT NULL,
    PRIMARY KEY (account_id, provider_code, external_container_id),
    UNIQUE (provider_code, external_container_id)
);

CREATE TABLE IF NOT EXISTS runtimes (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    code TEXT NOT NULL,
    label TEXT NOT NULL,
    token_lookup TEXT NOT NULL UNIQUE,
    last_seen_at TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'offline',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, code)
);

CREATE TABLE IF NOT EXISTS profiles (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    name TEXT NOT NULL,
    body TEXT NOT NULL,
    backend_hint TEXT,
    harness TEXT,
    workflow_config TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (account_id, name)
);

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_versions (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    form TEXT NOT NULL,
    payload BLOB,
    reference TEXT NOT NULL DEFAULT '',
    checksum TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (agent_id, version)
);

CREATE TABLE IF NOT EXISTS agent_pointers (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel TEXT NOT NULL DEFAULT 'latest',
    version_id TEXT NOT NULL REFERENCES agent_versions(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, channel)
);

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
    runtime_id TEXT NOT NULL DEFAULT '',
    external_task_id TEXT NOT NULL DEFAULT '',
    external_event_id TEXT NOT NULL,
    provider_code TEXT NOT NULL,
    external_actor_id TEXT,
    writeback_status TEXT NOT NULL DEFAULT 'pending',
    writeback_attempts INTEGER NOT NULL DEFAULT 0,
    writeback_last_error TEXT,
    task_title TEXT NOT NULL DEFAULT '',
    task_description TEXT NOT NULL DEFAULT '',
    profile_body_snapshot TEXT,
    workflow TEXT,
    instructions TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued',
    claim_lease_until TIMESTAMP,
    ttl_expires_at TIMESTAMP NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    result_summary TEXT,
    payload TEXT,
    runner_id TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    claimed_at TIMESTAMP,
    finished_at TIMESTAMP,
    UNIQUE (provider_code, external_event_id)
);

CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs (runtime_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_writeback ON jobs (writeback_status)
    WHERE writeback_status = 'pending';
CREATE INDEX IF NOT EXISTS idx_jobs_account_id ON jobs (account_id);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    runtime_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at TIMESTAMP,
    metadata TEXT
);

CREATE TABLE IF NOT EXISTS audit_log (
    id TEXT PRIMARY KEY,
    tenant_id TEXT,
    actor_id TEXT NOT NULL,
    actor_type TEXT NOT NULL DEFAULT 'user',
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    metadata TEXT NOT NULL DEFAULT '{}',
    recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_log_recorded ON audit_log (recorded_at DESC);

CREATE TABLE IF NOT EXISTS runner_identity (
    id TEXT PRIMARY KEY,
    rt_token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT,
    runtime_id TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    last_seen_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS registration_tokens (
    id TEXT PRIMARY KEY,
    token_lookup TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    account_id TEXT,
    expires_at TIMESTAMP,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
