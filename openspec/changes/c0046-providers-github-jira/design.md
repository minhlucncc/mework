## Context

The pipeline is provider-agnostic; only Mello implements `provider.Adapter` (verify, parse,
channel key, write-back) and is registered. GitHub/Jira are stubs. Adding them is implementing
the same interface — no pipeline changes.

## Goals / Non-Goals

**Goals:** working GitHub Issues + Jira adapters at parity with the Mello adapter's contract;
registered + connection-configurable; well-tested.

**Non-Goals:** Linear (future); GitHub App installation/OAuth flows beyond a PAT + webhook
secret; rich PR-review semantics (issue/PR **comments** are the write-back surface now).

## Decisions

- **Mirror the Mello adapter shape.** Each adapter implements verify (HMAC of the raw body
  with the connection's webhook secret; GitHub `X-Hub-Signature-256`, Jira webhook
  secret/JWT), `ParseTrigger` (the shared `@mework [profile] [workflow] [instructions]`
  grammar over the provider's comment payload), `ChannelKey` (`<provider>:<resource_id>`), and
  write-back (REST comment create). Constant-time HMAC compare + a replay window, same as
  Mello.
- **Provider-specific payload mapping only.** The differences are payload shapes and signature
  headers; map each provider's comment/issue event into the pipeline's neutral trigger +
  resource identifiers. Identify entities by `(provider_code, external_*_id)` — no schema
  change.
- **Register + blank-import.** Same pattern as Mello (`provider.Register` in `init`, blank-
  imported by `apps/mework-server`), so a `github`/`jira` connection is all that's needed to
  activate.

## Risks / Trade-offs

- **[Jira auth variants]** → support the common webhook-secret/JWT path; document which Jira
  deployment types are covered; extendable later.
- **[GitHub rate limits on write-back]** → rely on the existing durable write-back outbox +
  retry; add provider-appropriate backoff.
- **[Payload schema drift]** → pin to the documented webhook event shapes; tests assert the
  mapping; unknown event types are ignored (no enqueue).

## Migration Plan

Additive. New adapters + registration; activated per-connection. No schema migration (provider-
agnostic identifiers already in place).
