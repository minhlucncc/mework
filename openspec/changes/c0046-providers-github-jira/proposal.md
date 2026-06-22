## Why

mework is designed to be provider-agnostic ("Mello today; Jira / Linear / GitHub Issues by
design"), but only the **Mello** adapter is implemented and registered. The GitHub and Jira
adapters are `init()`-print stubs (`libs/server/provider/github/github.go`,
`provider/jira/jira.go`) and are never `provider.Register`-ed (M2). To make the
provider-gateway claim real, GitHub Issues and Jira need working adapters behind the existing
`provider.Adapter` interface.

## What Changes

- **Implement the GitHub Issues adapter** behind `provider.Adapter`: webhook **signature
  verification** (`X-Hub-Signature-256` HMAC), `ParseTrigger` matching the `@mework …`
  grammar in issue/PR comments, the channel-key derivation, and REST **write-back** (create
  an issue/PR comment).
- **Implement the Jira adapter** behind `provider.Adapter`: webhook verification (Jira webhook
  secret / JWT as applicable), `ParseTrigger` over issue comments, channel key, and REST
  write-back (add a comment to the issue).
- **Register both** so they are selectable via provider connections; blank-import in the
  server main alongside Mello. No pipeline changes — the existing webhook → enqueue →
  write-back flow is provider-agnostic and works once the adapter is registered.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `provider-gateway`: registers working **GitHub Issues** and **Jira** adapters (signature
  verification, trigger parsing, channel key, REST write-back) behind the provider interface,
  alongside Mello.

## Impact

- **Server:** `libs/server/provider/github/github.go`, `libs/server/provider/jira/jira.go`
  (real adapters), registration + `apps/mework-server` blank-imports. Reuses the
  `provider.Adapter` contract, the webhook pipeline, and the durable write-back outbox
  unchanged.
- **Tests:** per-adapter signature-verification (valid/invalid/replay), `ParseTrigger` grammar,
  channel-key, and write-back (against an `httptest` GitHub/Jira API), mirroring the Mello
  adapter tests.
- **Config:** provider connections for `github` / `jira` (token + webhook secret) via the
  existing connection CRUD. No schema migration (identify by `(provider_code, external_*_id)`).
