# API Reference

> Audience: developers integrating with or extending `mework-server`. Covers HTTP
> endpoints, authentication per route, the data model, and the planned SSE bus +
> catalog API. Status badges: **`[Implemented]`** exists today; **`[Planned — cNNNN]`**
> is specified under `openspec/changes/`.
>
> Source of truth: `internal/server/router.go` (routes), `internal/store/migrations/`
> (schema), `internal/server/webhook/parse.go` (trigger grammar).

## Authentication schemes

| Scheme | Credential | Guards | Mechanism |
|--------|-----------|--------|-----------|
| **open** | none | `/healthz` | — |
| **signature** | per-connection webhook secret | `/webhooks/{provider}` | HMAC-SHA256 verified inside the handler |
| **rt_token** `[Implemented]` | runtime bearer token (`mework_rt_…`) | `/api/v1/jobs/*` | HMAC-SHA256 lookup hash keyed by `SERVER_KEY` |
| **PAT** `[Implemented]` | Mello personal access token | `/api/v1` management routes | bearer → Mello `/me`, cached 60s |
| **runner identity** `[Planned — c0004]` | durable runner credential | SSE subscribe / ack / pull | obtained by exchanging a registration token at enroll |

See [auth-and-secrets.md](auth-and-secrets.md) for token formats, hashing, and the
grant model.

## HTTP endpoints — current `[Implemented]`

Global middleware (chi): `RequestID`, `RealIP`, `Logger`, `Recoverer`.

### Health & webhooks

| Method | Path | Auth | Handler / behavior |
|--------|------|------|--------------------|
| GET | `/healthz` | open | DB ping → `200 {"status":"ok"}` or `503` |
| POST | `/webhooks/{provider}` | signature | Verify, parse trigger, enqueue. `202` on enqueue; `200` on any silent-ignore path (so the provider stops retrying); `401` only on missing/failed signature |

### Job routes (rt_token)

| Method | Path | Behavior |
|--------|------|----------|
| POST | `/api/v1/jobs/claim` | Claim the oldest `queued` job for this runtime. `FOR UPDATE SKIP LOCKED` under a per-runtime advisory lock; sets `status=claimed`, `claim_lease_until=NOW()+30s`, `attempts++`. `204 No Content` when nothing to claim |
| POST | `/api/v1/jobs/{id}/ack` | Ownership-checked (`403` if not owner). Status `running\|done\|failed` via the state machine (`409` on invalid transition). On `done`/`failed`: sets `writeback_status=pending` and fires async REST write-back. `204` |
| POST | `/api/v1/jobs/{id}/heartbeat` | Extends `claim_lease_until=NOW()+90s`. `204` |

### Management routes (PAT)

| Method | Path | Handler |
|--------|------|---------|
| POST | `/api/v1/runtimes` | `registry.CreateRuntime` — response includes the one-time `Token` |
| GET | `/api/v1/runtimes` | `registry.ListRuntimes` |
| DELETE | `/api/v1/runtimes/{id}` | `registry.DeleteRuntime` |
| POST | `/api/v1/connections` | `connection.CreateConnection` (seals the provider token) |
| GET | `/api/v1/connections` | `connection.ListConnections` |
| GET | `/api/v1/connections/{provider_code}` | `connection.GetConnection` |
| DELETE | `/api/v1/connections/{provider_code}` | `connection.DeleteConnection` |
| POST | `/api/v1/profiles` | `profile.CreateProfile` |
| GET | `/api/v1/profiles` | `profile.ListProfiles` |
| GET | `/api/v1/profiles/{name}` | `profile.GetProfile` |
| PUT | `/api/v1/profiles/{name}` | `profile.UpdateProfile` |
| DELETE | `/api/v1/profiles/{name}` | `profile.DeleteProfile` |

## HTTP/SSE endpoints — target `[Planned]`

The redesign **removes the long-poll claim** and replaces it with SSE subscribe +
out-of-band POST acks. It adds the catalog and dispatch routes.

### Message bus `[Planned — c0002]`

| Method | Path | Behavior |
|--------|------|----------|
| GET | `…/subscribe` | `Content-Type: text/event-stream`. Honors requested topics + the `Last-Event-ID` header for resume; emits SSE events with **monotonic ids** and periodic heartbeat comments. A subscriber may only subscribe to topics it is entitled to |
| POST | `…/ack` | Acknowledge a delivered message by id (SSE is server→client only, so ack is out-of-band) |

Broker interface (server-internal, pluggable backend — Postgres `LISTEN/NOTIFY`
default, in-memory for tests, NATS/Redis swappable without changing the SSE contract):
`Publish(topic, msg)` · `Subscribe(topics, fromEventID) → stream` · `Ack(msgID)`.
Topics: `runner.<id>.dispatch`, `session.<id>.control`.

### Agent catalog `[Planned — c0003]`

| Method | Path | Behavior |
|--------|------|----------|
| POST | `/api/v1/agents/{name}/versions` | Publish an **immutable** version (rejects overwriting an existing version with different content) |
| GET | `/api/v1/agents` | List agents |
| GET | `/api/v1/agents/{name}` | Resolve an agent (including `@latest` / named channels → concrete version) |
| GET | `/api/v1/agents/{name}/versions/{version}/pull` | Authorized pull (against puller identity + grant); returns artifact-or-reference + `form` |
| POST | `/api/v1/agents/{name}/dispatch` | **Dispatch = publish**: resolves the version, builds the grant, and publishes a small dispatch message (agent ref + grant, referencing the exact version) to the target runner/session topic. Does not push artifact bytes — the runner pulls lazily |

`form` is type-agnostic: `definition` (manifest: prompt + workflow + declared needs)
or `image` (packaged/container image reference). Existing `profiles` map onto
`definition`-form agents.

## Trigger grammar `[Implemented]`

Parsed in `internal/server/webhook/parse.go` from a ticket comment body:

```
@mework <profile> [workflow] <free instructions>
```

- `@mework` must be at the start of the body, or preceded by a space/newline. No match
  → not a trigger.
- **Word 1** = `profile` (the profile/runtime code).
- **Word 2** = `workflow` **only if** it normalizes to a recognized keyword
  (case-insensitive, trimmed via `NormalizeWorkflow`); otherwise word 2 onward is
  `instructions` and `workflow` is empty.
- Remaining text = `instructions` (capped at 64KB).

Recognized workflow keywords: **`plan`, `cook`, `test`, `review`, `ship`, `journal`**.

## Webhook processing pipeline `[Implemented]`

`POST /webhooks/{provider}` (`internal/server/webhook/handler.go`):

1. `ExtractContainerID` from the raw body → lookup `watched_containers` → `account_id`.
2. Load `provider_connections.webhook_secret`.
3. Verify `X-Mello-Signature` / `X-Mello-Timestamp` / `X-Mello-Delivery-Id`
   (`HMAC-SHA256(secret, timestamp + "." + body)`, ±5-min replay window).
4. `ParseEvent` → `ParseTrigger` on the comment body.
5. Idempotency by delivery id against `jobs.external_event_id`.
6. Actor allowlist check against `account_identities`; self-retrigger guard.
7. Resolve runtime by `profileName == runtimes.code`; cap instructions at 64KB.
8. Snapshot the profile body; decrypt the connection token; fetch ticket
   title/description.
9. `jobs.Enqueue` → `202 Accepted`.

## Data model `[Implemented]`

Single goose migration (`internal/store/migrations/000001_init.sql`). Entities are
identified by `(provider_code, external_*_id)` so a new provider needs no migration.

| Table | Key columns | Notes |
|-------|-------------|-------|
| `accounts` | `id`, `name`, `created_at` | One per Mello user |
| `provider_connections` | `account_id`, `provider_code`, `webhook_secret`, `mcp_auth_enc`, `config` | Unique `(account_id, provider_code)`; `mcp_auth_enc` holds the AES-sealed provider token |
| `account_identities` | `account_id`, `provider_code`, `external_user_id` | Actor allowlist; unique `(provider_code, external_user_id)` |
| `watched_containers` | `account_id`, `provider_code`, `external_container_id` | Board → account routing; unique `(provider_code, external_container_id)` |
| `runtimes` | `id`, `account_id`, `code`, `label`, `token_lookup`, `last_seen_at`, `status` | `token_lookup` is the HMAC hash (unique); unique `(account_id, code)` |
| `profiles` | `id`, `account_id`, `name`, `body`, `backend_hint`, `harness`, `workflow_config` | Unique `(account_id, name)` |
| `jobs` | `id`, `account_id`, `runtime_id`, `external_task_id`, `external_event_id`, `provider_code`, `external_actor_id`, `status`, `writeback_status`, `task_title`, `task_description`, `profile_body_snapshot`, `workflow`, `instructions`, `claim_lease_until`, `ttl_expires_at`, `attempts`, `result_summary`, `started_at`, `finished_at` | Unique `(provider_code, external_event_id)` for idempotency |

Key indexes: `idx_jobs_claim (runtime_id, status, created_at)`; partial unique
`idx_jobs_one_active_per_runtime (runtime_id) WHERE status IN ('claimed','running')`;
partial `idx_jobs_writeback WHERE writeback_status='pending'`.

**Target tables `[Planned — c0003]`:** `agents` (name) and `agent_versions`
(immutable version, `form`, payload/reference, checksum). Republishing a version with
different content is rejected. `c0002` reframes `jobs` as the durable backing store
behind the bus (keeping the transactional state machine: `running`→`started_at`,
terminal→`finished_at`, terminal states immutable), and removes the long-poll claim and
heartbeat/lease requirements.

## Job state machine `[Implemented]`

Transactional with row locks (`internal/server/jobs/state.go`); terminal states
immutable; same-status transition is a no-op.

```
queued   → claimed | failed
claimed  → running | done | failed | queued
running  → done | failed | queued
```

A background **lease sweeper** returns expired-lease jobs to `queued` and drives
pending write-backs.

## Write-back `[Implemented]`

On terminal ack the server sets `writeback_status=pending` and posts the result over
the provider's REST API (Mello `CreateComment`), unsealing the connection credential
only at write time. A **durable outbox** (`pending → processing → done`/`failed`,
sweeper-retried) guarantees exactly-once delivery — no duplicate comment on restart.
The local side never holds provider credentials. See
[runtime-and-sandbox.md](runtime-and-sandbox.md) and
[auth-and-secrets.md](auth-and-secrets.md).

## Client coverage

`internal/meworkclient/` maps 1:1 onto the API: `Claim`/`Ack`/`Heartbeat` (rt_token);
`CreateRuntime`/`ListRuntimes`/`DeleteRuntime`, `Create/Get/List/DeleteConnection`,
`Create/Get/List/Update/DeleteProfile` (PAT). `CreateRuntimeResponse` carries the
one-time `Token`.
