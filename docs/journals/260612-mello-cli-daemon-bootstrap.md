# Journal — 2026-06-12: Bootstrapping the Mello CLI Daemon

## What was built
A Go CLI + agent-runtime daemon for Mello (kanban), module `mework`, modeled on
the Multica Go daemon. Nine phases, all green (build/test/vet). Daemon polls
Mello REST for a `/run` comment keyword, runs a local AI CLI (claude/codex/
opencode) against the ticket, writes results back via the hosted Mello MCP.

## Key decisions (and why)
- **Go, not Python.** Multica (the reference) is Go; Mello's SDK/MCP is Python.
  Chose Go to mirror Multica's structure, consuming Mello's Python MCP as a
  client. Tradeoff: reimplement a REST client in Go vs. a cross-language bridge.
- **Poll, not push.** Mello has no webhooks/server-push (verified in source).
  Multica's WebSocket wakeup + server-side task claim were dropped; replaced with
  a REST poll loop keyed on a comment keyword.
- **Trigger = `/run` comment keyword**, user-driven via the existing Mello MCP —
  not an autonomous assignee-hunter. Simpler, more controllable.
- **Write-back via hosted MCP** kept even after red-team found every write has a
  plain REST endpoint (redundant). User made the informed call to keep MCP;
  surfaced rather than silently reversed.

## Lessons / what bit us
- **Research subagents were pinned to an unavailable model** (`gemini-3.5-flash`)
  and failed; had to pass an explicit `model: sonnet` override. One red-team
  subagent fell into a ~140x web-search reasoning loop and never produced output.
  Takeaway: for read-answerable verification, doing it inline against source was
  faster and more reliable than delegating.
- **Red-team caught two real issues by reading source**: (1) the shipped FastMCP
  `main()` is stdio-only — a "hosted HTTP/SSE MCP" assumes Mello runs it
  differently; (2) MCP write-back is redundant given full REST write surface.
  Both surfaced to the user per audit rules instead of auto-applied.
- **Re-entrant mutex deadlock**: `State.Mark` → `save` → `MarshalJSON` all locked
  the same mutex; `go test` hung 30s then dumped goroutines. Fix: don't hold the
  lock across marshaling. Tests with `-timeout` made the hang diagnosable.
- **`ck plan create` silently no-op'd** (exit 0, no files). Fell back to authoring
  plan files directly — the skill explicitly allows this.

## Deliberate scope cuts (honest gaps)
- `mello update` self-update **not implemented** — needs a real published release
  repo; the module is still the placeholder `mework`. Build machinery
  (Makefile + goreleaser) is in place.
- **Checklist write-back tick** not auto-done — no generic "agent done" item
  convention on a Mello board. MCP client supports the tools when a convention
  exists.

## Security notes
- AI prompt fed via **stdin, never argv** — ticket content is attacker-
  controllable, so this avoids command injection.
- Self-retrigger guard: trigger scan skips comments authored by the daemon's own
  user id, so its start/done comments can't loop. Idempotency via per-ticket
  handled-comment-id set, marked before the agent runs.
- Token files written 0600; never settable via `config set`; masked in output.

## Open questions
1. Confirm the actual hosted Mello MCP endpoint URL (user says it exists).
2. Finalize the Go module path / GitHub repo owner before publishing (unblocks
   `mello update`).
3. Verify REST field names against a live API response (derived from SDK
   `from_dict`, not yet hit live).
